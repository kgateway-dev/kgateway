package listener

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/api/conditions"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/listenerpolicy"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	route "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/httproute"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/routeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/sslutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

var logger = logging.New("translator/listener")

const (
	TcpTlsListenerNoBackendsMessage = "TCP/TLS listener has no valid backends or routes"
	ResourceNotFoundMessageTemplate = "%s %s/%s not found."
)

type ListenerTranslatorConfig struct {
	ListenerBindIpv6                     bool
	EnableExperimentalGatewayAPIFeatures bool
}

// TranslateListeners translates the set of ListenerIRs required to produce a full output proxy (either from one Gateway or multiple merged Gateways)
func TranslateListeners(
	kctx krt.HandlerContext,
	ctx context.Context,
	queries query.GatewayQueries,
	gateway *ir.Gateway,
	routesForGw *query.RoutesForGwResult,
	reporter reports.Reporter,
	settings ListenerTranslatorConfig,
) []ir.ListenerIR {
	defer metrics.CollectTranslationMetrics(metrics.TranslatorMetricLabels{
		Name:       gateway.Name,
		Namespace:  gateway.Namespace,
		Translator: "TranslateListeners",
	})(nil)

	validatedListeners := validateGateway(gateway, reporter)
	mergedListeners := mergeGWListeners(queries, validatedListeners, *gateway, routesForGw, reporter, settings)
	translatedListeners := mergedListeners.translateListeners(kctx, ctx, queries, reporter)

	return translatedListeners
}

func mergeGWListeners(
	queries query.GatewayQueries,
	listeners []ir.Listener,
	parentGw ir.Gateway,
	routesForGw *query.RoutesForGwResult,
	reporter reports.Reporter,
	settings ListenerTranslatorConfig,
) *MergedListeners {
	ml := &MergedListeners{
		parentGw:          parentGw,
		Queries:           queries,
		settings:          settings,
		frontendTLSConfig: parentGw.FrontendTLSConfig,
	}
	for _, listener := range listeners {
		result := routesForGw.GetListenerResult(listener.Parent, string(listener.Name))
		parentReporter := listener.GetParentReporter(reporter)
		listenerReporter := parentReporter.ListenerName(string(listener.Name))
		var routes []*query.RouteInfo
		if result != nil {
			routes = result.Routes
		}
		ml.AppendListener(listener, routes, listenerReporter)
	}
	return ml
}

type MergedListeners struct {
	parentGw          ir.Gateway
	Listeners         []*MergedListener
	Queries           query.GatewayQueries
	settings          ListenerTranslatorConfig
	frontendTLSConfig *ir.FrontendTLSConfigIR
}

func (ml *MergedListeners) AppendListener(
	listener ir.Listener,
	routes []*query.RouteInfo,
	reporter reports.ListenerReporter,
) error {
	switch listener.Protocol {
	case gwv1.HTTPProtocolType:
		ml.appendHttpListener(listener, routes, reporter)
	case gwv1.HTTPSProtocolType:
		ml.appendHttpsListener(listener, routes, reporter)
	// TODO default handling
	case gwv1.TCPProtocolType:
		ml.AppendTcpListener(listener, routes, reporter)
	case gwv1.TLSProtocolType:
		ml.AppendTlsListener(listener, routes, reporter)
	default:
		return fmt.Errorf("unsupported protocol: %v", listener.Protocol)
	}

	return nil
}

func getListenerPortNumber(listener ir.Listener) gwv1.PortNumber {
	return gwv1.PortNumber(listener.Port)
}

func (ml *MergedListeners) appendHttpListener(
	listener ir.Listener,
	routesWithHosts []*query.RouteInfo,
	reporter reports.ListenerReporter,
) {
	parent := httpFilterChainParent{
		gatewayListenerName: query.GenerateRouteKey(listener.Parent, string(listener.Name)),
		gatewayListener:     listener,
		routesWithHosts:     routesWithHosts,
		attachedPolicies:    listener.AttachedPolicies,
		listenerReporter:    reporter,
	}
	fc := &httpFilterChain{
		parents: []httpFilterChainParent{parent},
	}

	finalPort := getListenerPortNumber(listener)
	for _, lis := range ml.Listeners {
		if lis.port != finalPort {
			continue
		}
		if lis.httpFilterChain != nil {
			lis.httpFilterChain.parents = append(lis.httpFilterChain.parents, parent)
		} else {
			lis.httpFilterChain = fc
		}
		return
	}

	// create a new filter chain for the listener
	ml.Listeners = append(ml.Listeners, &MergedListener{
		name:            GenerateListenerName(listener),
		port:            finalPort,
		httpFilterChain: fc,
		listener:        listener,
		gateway:         ml.parentGw,
		settings:        ml.settings,
	})
}

func (ml *MergedListeners) appendHttpsListener(
	listener ir.Listener,
	routesWithHosts []*query.RouteInfo,
	reporter reports.ListenerReporter,
) {
	tls := listener.TLS
	if tls == nil {
		tls = &gwv1.ListenerTLSConfig{}
	}

	mfc := httpsFilterChain{
		gatewayListenerName: query.GenerateRouteKey(listener.Parent, string(listener.Name)),
		listener:            listener,
		listenerReporter:    reporter,
		sniDomain:           listener.Hostname,
		tls:                 tls,
		routesWithHosts:     routesWithHosts,
		attachedPolicies:    listener.AttachedPolicies,
	}

	// Perform the port transformation away from privileged ports only once to use
	// during both lookup and when appending the listener.
	finalPort := getListenerPortNumber(listener)
	for _, lis := range ml.Listeners {
		if lis.port == finalPort {
			lis.httpsFilterChains = append(lis.httpsFilterChains, mfc)
			return
		}
	}

	ml.Listeners = append(ml.Listeners, &MergedListener{
		name:              GenerateListenerName(listener),
		port:              finalPort,
		httpsFilterChains: []httpsFilterChain{mfc},
		listener:          listener,
		gateway:           ml.parentGw,
		settings:          ml.settings,
	})
}

func (ml *MergedListeners) AppendTcpListener(
	listener ir.Listener,
	routeInfos []*query.RouteInfo,
	reporter reports.ListenerReporter,
) {
	parent := tcpFilterChainParent{
		gatewayListenerName: query.GenerateRouteKey(listener.Parent, string(listener.Name)),
		listener:            listener,
		listenerReporter:    reporter,
		routesWithHosts:     routeInfos,
	}
	fc := tcpFilterChain{
		parents:          parent,
		listenerReporter: reporter,
	}

	finalPort := getListenerPortNumber(listener)
	for _, lis := range ml.Listeners {
		if lis.port == finalPort {
			lis.TcpFilterChains = append(lis.TcpFilterChains, fc)
			return
		}
	}

	// create a new filter chain for the listener
	ml.Listeners = append(ml.Listeners, &MergedListener{
		name:            GenerateListenerName(listener),
		port:            finalPort,
		TcpFilterChains: []tcpFilterChain{fc},
		listener:        listener,
		gateway:         ml.parentGw,
		settings:        ml.settings,
	})
}

func (ml *MergedListeners) AppendTlsListener(
	listener ir.Listener,
	routeInfos []*query.RouteInfo,
	reporter reports.ListenerReporter,
) {
	tls := listener.TLS
	if tls == nil {
		tls = &gwv1.ListenerTLSConfig{}
	}

	var filterChains []tcpFilterChain
	appendFilterChain := func(routeSet []*query.RouteInfo, sniDomains []string) {
		filterChains = append(filterChains, tcpFilterChain{
			parents: tcpFilterChainParent{
				gatewayListenerName: query.GenerateRouteKey(listener.Parent, string(listener.Name)),
				listener:            listener,
				listenerReporter:    reporter,
				routesWithHosts:     routeSet,
			},
			tls:              tls,
			sniDomains:       sniDomains,
			listenerReporter: reporter,
		})
	}

	if len(routeInfos) == 0 {
		appendFilterChain(nil, listenerSNIDomains(listener))
	} else {
		var conflicted []conflictedRoute
		for i, owned := range resolveTLSRouteSNIDomains(listener, routeInfos) {
			if len(owned.lost) > 0 {
				conflicted = append(conflicted, conflictedRoute{
					routeInfo: routeInfos[i],
					message:   sniConflictMessage(owned, routeInfos),
					partial:   owned.ok,
				})
			}
			// A duplicate shares the filter chain of its first entry.
			if owned.ok && !owned.duplicate {
				appendFilterChain([]*query.RouteInfo{routeInfos[i]}, owned.sniDomains)
			}
		}
		// Every lost SNI domain has a winning route, so there is at least one
		// filter chain to report the conflicts from.
		if len(conflicted) > 0 && len(filterChains) > 0 {
			filterChains[0].conflictedRoutes = conflicted
		}
	}

	finalPort := getListenerPortNumber(listener)
	for _, lis := range ml.Listeners {
		if lis.port == finalPort {
			lis.TcpFilterChains = append(lis.TcpFilterChains, filterChains...)
			return
		}
	}

	// create a new filter chain for the listener
	ml.Listeners = append(ml.Listeners, &MergedListener{
		name:            GenerateListenerName(listener),
		port:            finalPort,
		TcpFilterChains: filterChains,
		listener:        listener,
		settings:        ml.settings,
	})
}

func listenerSNIDomains(listener ir.Listener) []string {
	if listener.Hostname == nil {
		return nil
	}
	return []string{string(*listener.Hostname)}
}

func tlsRouteSNIDomains(listener ir.Listener, routeInfo *query.RouteInfo) []string {
	if routeInfo != nil {
		if _, ok := routeInfo.Object.(*ir.TlsRouteIR); ok {
			if hostnames := routeInfo.Hostnames(); len(hostnames) > 0 {
				return slices.Clone(hostnames)
			}
		}
	}

	return listenerSNIDomains(listener)
}

type ownedSNIDomains struct {
	// sniDomains are the SNI domains the route owns; nil matches any SNI.
	sniDomains []string
	// ok is false when every SNI domain of the route is owned by another route.
	ok bool
	// lost are the SNI domains of the route that are owned by other routes.
	lost []lostSNIDomain
	// duplicate is true when an earlier entry is the same route attached to the
	// listener through another parentRef. Only the earlier entry is resolved;
	// a duplicate copies its result, so every parentRef reports a conflict.
	duplicate bool
}

type lostSNIDomain struct {
	// domain is the lost SNI domain; "" matches any SNI.
	domain string
	// winner is the index in routeInfos of the route that owns the domain.
	winner int
}

// sniConflictMessage describes the SNI domains a route lost and the routes
// that own them.
func sniConflictMessage(owned ownedSNIDomains, routeInfos []*query.RouteInfo) string {
	lost := make([]string, 0, len(owned.lost))
	for _, l := range owned.lost {
		domain := l.domain
		if domain == "" {
			domain = "any SNI"
		}
		w := routeInfos[l.winner].Object
		lost = append(lost, fmt.Sprintf("%s (owned by %s %s/%s)", domain, w.GetGroupKind().Kind, w.GetNamespace(), w.GetName()))
	}
	if owned.ok {
		return fmt.Sprintf("Programmed for SNI domains [%s]; not programmed for SNI domains owned by other routes: %s",
			strings.Join(owned.sniDomains, ", "), strings.Join(lost, ", "))
	}
	return "Not programmed, all SNI domains are owned by other routes: " + strings.Join(lost, ", ")
}

// resolveTLSRouteSNIDomains assigns each SNI domain of a TLS listener to a
// single route, since Envoy rejects filter chains with identical matches. The
// result is indexed like routeInfos.
//
// Routes whose hostnames intersect with the listener hostname to the same SNI
// domain are ordered by the specificity of their own hostnames, then by
// creation timestamp (oldest first), then by namespace/name.
func resolveTLSRouteSNIDomains(listener ir.Listener, routeInfos []*query.RouteInfo) []ownedSNIDomains {
	type candidate struct {
		idx           int
		routeHostname string
	}
	precedes := func(a, b candidate) bool {
		if c := routeutils.CompareHostnameSpecificity(a.routeHostname, b.routeHostname); c != 0 {
			return c < 0
		}
		aObj, bObj := routeInfos[a.idx].Object.GetSourceObject(), routeInfos[b.idx].Object.GetSourceObject()
		if c := aObj.GetCreationTimestamp().Compare(bObj.GetCreationTimestamp().Time); c != 0 {
			return c < 0
		}
		return types.NamespacedName{Namespace: aObj.GetNamespace(), Name: aObj.GetName()}.String() <
			types.NamespacedName{Namespace: bObj.GetNamespace(), Name: bObj.GetName()}.String()
	}

	// A route without SNI domains matches any SNI; track it under "".
	domainsOrCatchAll := func(domains []string) []string {
		if len(domains) == 0 {
			return []string{""}
		}
		return domains
	}

	// The same route can reach the listener through more than one parentRef
	// (for example one by sectionName and one by port). It must not conflict
	// with itself, so only its first entry is resolved.
	first := map[routeKey]int{}
	duplicateOf := make([]int, len(routeInfos))
	duplicate := make([]bool, len(routeInfos))
	for i, routeInfo := range routeInfos {
		key := keyOfRoute(routeInfo)
		if j, ok := first[key]; ok {
			duplicate[i], duplicateOf[i] = true, j
			continue
		}
		first[key] = i
	}

	domains := make([][]string, len(routeInfos))
	winners := map[string]candidate{}
	for i, routeInfo := range routeInfos {
		if duplicate[i] {
			continue
		}
		domains[i] = tlsRouteSNIDomains(listener, routeInfo)
		if !producesTcpFilterChain(routeInfo) {
			// Rejected later; it must not take SNI domains from valid routes.
			continue
		}
		routeHostnames := specHostnames(routeInfo)
		for _, domain := range domainsOrCatchAll(domains[i]) {
			c := candidate{idx: i, routeHostname: routeutils.MostSpecificRouteHostname(routeHostnames, domain)}
			if w, ok := winners[domain]; !ok || precedes(c, w) {
				winners[domain] = c
			}
		}
	}

	out := make([]ownedSNIDomains, len(routeInfos))
	for i, routeInfo := range routeInfos {
		switch {
		case duplicate[i]:
			continue
		case !producesTcpFilterChain(routeInfo):
			out[i] = ownedSNIDomains{sniDomains: domains[i], ok: true}
			continue
		}
		for _, domain := range domainsOrCatchAll(domains[i]) {
			if w := winners[domain].idx; w != i {
				out[i].lost = append(out[i].lost, lostSNIDomain{domain: domain, winner: w})
			} else if domain != "" {
				out[i].sniDomains = append(out[i].sniDomains, domain)
			}
		}
		out[i].ok = len(out[i].lost) == 0 || len(out[i].sniDomains) > 0
	}
	// Duplicates come after their first entry, which is resolved by now.
	for i := range routeInfos {
		if duplicate[i] {
			out[i] = out[duplicateOf[i]]
			out[i].duplicate = true
		}
	}
	return out
}

// routeKey identifies a route regardless of the parentRef it is attached through.
type routeKey struct {
	gk        schema.GroupKind
	namespace string
	name      string
}

func keyOfRoute(routeInfo *query.RouteInfo) routeKey {
	return routeKey{gk: routeInfo.Object.GetGroupKind(), namespace: routeInfo.Object.GetNamespace(), name: routeInfo.Object.GetName()}
}

// producesTcpFilterChain reports whether translateTcpFilterChain can produce a
// filter chain for the route.
func producesTcpFilterChain(routeInfo *query.RouteInfo) bool {
	switch r := routeInfo.Object.(type) {
	case *ir.TcpRouteIR:
		return len(r.SourceObject.Spec.Rules) == 1 && len(r.Backends) > 0
	case *ir.TlsRouteIR:
		return len(r.SourceObject.Spec.Rules) == 1 && len(r.Backends) > 0
	default:
		return false
	}
}

func (ml *MergedListeners) translateListeners(
	kctx krt.HandlerContext,
	ctx context.Context,
	queries query.GatewayQueries,
	reporter reports.Reporter,
) []ir.ListenerIR {
	listeners := make([]ir.ListenerIR, 0, len(ml.Listeners))
	for _, mergedListener := range ml.Listeners {
		listener := mergedListener.TranslateListener(kctx, ctx, queries, reporter)
		listeners = append(listeners, listener)
	}
	return listeners
}

type MergedListener struct {
	name              string
	port              gwv1.PortNumber
	httpFilterChain   *httpFilterChain
	httpsFilterChains []httpsFilterChain
	TcpFilterChains   []tcpFilterChain
	listener          ir.Listener
	gateway           ir.Gateway
	settings          ListenerTranslatorConfig
}

func (ml *MergedListener) TranslateListener(
	kctx krt.HandlerContext,
	ctx context.Context,
	queries query.GatewayQueries,
	reporter reports.Reporter,
) ir.ListenerIR {
	// Translate HTTP filter chains
	var httpFilterChains []ir.HttpFilterChainIR
	if ml.httpFilterChain != nil {
		httpFilterChain := ml.httpFilterChain.translateHttpFilterChain(
			ctx,
			ml.name,
			reporter,
		)
		httpFilterChains = append(httpFilterChains, httpFilterChain)
	}

	// Translate HTTPS filter chains. Index the slice so each chain's Listener points
	// into it rather than at a per-iteration copy.
	for i := range ml.httpsFilterChains {
		httpsFilterChain, err := ml.httpsFilterChains[i].translateHttpsFilterChain(
			kctx,
			ctx,
			queries,
			reporter,
			ml.gateway.FrontendTLSConfig,
			ml.gateway.Namespace,
		)
		if err != nil {
			// Log and skip invalid HTTPS filter chains
			logger.Error("failed to translate HTTPS filter chain for listener", "listener", ml.name, "error", err)
			continue
		}

		httpFilterChains = append(httpFilterChains, *httpsFilterChain)
	}

	// Translate TCP listeners (if any exist)
	var matchedTcpListeners []ir.TcpIR
	for _, tfc := range ml.TcpFilterChains {
		if tcpListener := tfc.translateTcpFilterChain(kctx, ctx, queries, ml.name, reporter, ml.gateway.FrontendTLSConfig, ml.gateway.Namespace); tcpListener != nil {
			matchedTcpListeners = append(matchedTcpListeners, *tcpListener)
		}
	}

	// Only report errors if ALL TCP filter chains failed (port is not programmed)
	if len(ml.TcpFilterChains) > 0 && len(matchedTcpListeners) == 0 {
		// All TCP filter chains failed - the port is not programmed
		listenerCondition := reports.ListenerCondition{
			Type:    gwv1.ListenerConditionProgrammed,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.ListenerReasonInvalid,
			Message: TcpTlsListenerNoBackendsMessage,
		}
		// Report to all TCP filter chains since the entire port failed
		for _, tfc := range ml.TcpFilterChains {
			tfc.listenerReporter.SetCondition(listenerCondition)
		}
	}

	// Get bind address based on ListenerBindIpv6 setting
	bindAddress := "0.0.0.0"
	if ml.settings.ListenerBindIpv6 {
		bindAddress = "::"
	}

	// Create and return the listener with all filter chains and TCP listeners
	return ir.ListenerIR{
		Name:              ml.name,
		BindAddress:       bindAddress,
		BindPort:          uint32(ml.port),       //nolint:gosec // G115: Gateway listener port is int32, always positive, safe to convert to uint32
		AttachedPolicies:  ir.AttachedPolicies{}, // TODO: find policies attached to listener and attach them <- this might not be possible due to listener merging. also a gw listener ~= envoy filter chain; and i don't believe we need policies there
		HttpFilterChain:   httpFilterChains,
		TcpFilterChain:    matchedTcpListeners,
		PolicyAncestorRef: ml.listener.PolicyAncestorRef,
	}
}

// tcpFilterChain each one represents a Gateway listener that has been merged into a single kgateway Listener
// (with distinct filter chains). In the case where no Gateway listener merging takes place, every listener
// will use a kgateway AggregatedListener with one TCP filter chain.
type tcpFilterChain struct {
	parents          tcpFilterChainParent
	tls              *gwv1.ListenerTLSConfig
	sniDomains       []string
	listenerReporter reports.ListenerReporter
	// conflictedRoutes are routes on the same Gateway listener that lost some
	// or all of their SNI domains to other routes.
	conflictedRoutes []conflictedRoute
}

type conflictedRoute struct {
	routeInfo *query.RouteInfo
	message   string
	// partial is true when the route still owns some SNI domains, so it is
	// translated, and its backends validated, in its own filter chain.
	partial bool
}

type tcpFilterChainParent struct {
	gatewayListenerName string
	listener            ir.Listener
	listenerReporter    reports.ListenerReporter
	routesWithHosts     []*query.RouteInfo
}

func (tc *tcpFilterChain) translateTcpFilterChain(
	kctx krt.HandlerContext, ctx context.Context,
	queries query.GatewayQueries,
	parentName string,
	reporter reports.Reporter,
	frontendTLSConfig *ir.FrontendTLSConfigIR,
	gatewayNamespace string,
) *ir.TcpIR {
	for _, c := range tc.conflictedRoutes {
		rejectConflictingRoute(c.routeInfo, c.message, reporter)
		if !c.partial {
			// The route is never translated, so validate its backends here to
			// report ResolvedRefs.
			reportBackendErrors(c.routeInfo, reporter)
		}
	}

	parent := tc.parents
	if len(parent.routesWithHosts) == 0 {
		return nil
	}

	// A TCP listener cannot distinguish between multiple attached routes (there
	// is no SNI to match on), so only one route can be honored. The oldest route
	// by CreationTimestamp wins; any others are rejected with Accepted=False.
	//
	// Note: after #13694, AppendTlsListener creates one filter chain per
	// TLSRoute, so this path is only reached with len > 1 for TCPRoutes.
	r := slices.MinFunc(parent.routesWithHosts, func(a, b *query.RouteInfo) int {
		return a.Object.GetSourceObject().GetCreationTimestamp().Compare(b.Object.GetSourceObject().GetCreationTimestamp().Time)
	})
	if len(parent.routesWithHosts) > 1 {
		winner := keyOfRoute(r)
		for _, other := range parent.routesWithHosts {
			// The winning route can also be attached through another parentRef;
			// it must not conflict with itself.
			if keyOfRoute(other) == winner {
				continue
			}
			rejectConflictingRoute(other, fmt.Sprintf("Not programmed, the listener can only serve one route and the older %s %s/%s is programmed",
				r.Object.GetGroupKind().Kind, r.Object.GetNamespace(), r.Object.GetName()), reporter)
			reportBackendErrors(other, reporter)
		}
	}

	switch r.Object.(type) {
	case *ir.TcpRouteIR:
		tRoute := r.Object.(*ir.TcpRouteIR)

		var condition reports.RouteCondition
		if len(tRoute.SourceObject.Spec.Rules) == 1 {
			condition = reports.RouteCondition{
				Type:   gwv1.RouteConditionAccepted,
				Status: metav1.ConditionTrue,
				Reason: gwv1.RouteReasonAccepted,
			}
		} else {
			condition = reports.RouteCondition{
				Type:   gwv1.RouteConditionAccepted,
				Status: metav1.ConditionFalse,
				Reason: gwv1.RouteReasonUnsupportedValue,
			}
		}
		if condition.Status != metav1.ConditionTrue {
			return nil
		}

		// Collect ParentRefReporters for the TCPRoute
		parentRefReporters := make([]reports.ParentRefReporter, 0, len(tRoute.ParentRefs))
		for _, parentRef := range tRoute.ParentRefs {
			parentRefReporter := reporter.Route(tRoute.SourceObject).ParentRef(&parentRef)
			parentRefReporter.SetCondition(condition)
			parentRefReporters = append(parentRefReporters, parentRefReporter)
		}

		// Ensure unique names by appending the rule index to the TCPRoute name
		tcpHostName := fmt.Sprintf("%s-%s.%s-rule-%d", parentName, tRoute.Namespace, tRoute.Name, 0)
		var backends []ir.BackendRefIR
		for _, backend := range tRoute.Backends {
			// validate that we don't have an error:
			if backend.Err != nil || backend.BackendObject == nil {
				err := backend.Err
				if err == nil {
					err = errors.New("not found")
				}
				for _, parentRefReporter := range parentRefReporters {
					query.ProcessBackendError(err, parentRefReporter)
				}
			}
			// add backend even if we have errors, as according to spec, with multiple destinations,
			// they should fail based of the weights.
			backends = append(backends, backend)
		}
		// Avoid creating a TcpListener if there are no TcpHosts
		if len(backends) == 0 {
			return nil
		}

		resolvedValidation, err := resolveFrontendTLSConfig(tc.parents.listener.Port, frontendTLSConfig)
		if err != nil {
			reportTLSConfigError(err, tc.listenerReporter, resolvedValidation != nil)
			// An error and a non-nil validation means that the listener is partially valid,
			// and we should continue to translate the listener after writing the error to status
			if resolvedValidation == nil {
				return nil
			}
		}

		tlsConfig, err := translateTLSConfig(kctx, ctx, tc.parents.listener, tc.tls, queries, resolvedValidation, gatewayNamespace)
		if err != nil {
			// An error and a non-nil tlsConfig means that the listener is partially valid,
			// and we should continue to translate the listener after writing the error to status
			reportTLSConfigError(err, tc.listenerReporter, tlsConfig != nil)
			if tlsConfig == nil {
				return nil
			}
		}

		if tlsConfig != nil && len(tlsConfig.AlpnProtocols) == 0 {
			tlsConfig.AlpnProtocols = []string{string(annotations.AllowEmptyAlpnProtocols)}
		}

		return &ir.TcpIR{
			FilterChainCommon: ir.FilterChainCommon{
				FilterChainName: tcpHostName,
				StatPrefix:      buildStatPrefix("tcp", tc.parents.listener.Port, tRoute.Namespace, tRoute.Name),
				TLS:             tlsConfig,
			},
			BackendRefs:      backends,
			AttachedPolicies: tc.parents.listener.AttachedPolicies,
		}
	case *ir.TlsRouteIR:
		tRoute := r.Object.(*ir.TlsRouteIR)

		var condition reports.RouteCondition
		if len(tRoute.SourceObject.Spec.Rules) == 1 {
			condition = reports.RouteCondition{
				Type:   gwv1.RouteConditionAccepted,
				Status: metav1.ConditionTrue,
				Reason: gwv1.RouteReasonAccepted,
			}
		} else {
			condition = reports.RouteCondition{
				Type:   gwv1.RouteConditionAccepted,
				Status: metav1.ConditionFalse,
				Reason: gwv1.RouteReasonUnsupportedValue,
			}
		}
		if condition.Status != metav1.ConditionTrue {
			return nil
		}

		parentRefReporters := make([]reports.ParentRefReporter, 0, len(tRoute.ParentRefs))
		for _, parentRef := range tRoute.ParentRefs {
			parentRefReporter := reporter.Route(tRoute.SourceObject).ParentRef(&parentRef)
			parentRefReporter.SetCondition(condition)
			parentRefReporters = append(parentRefReporters, parentRefReporter)
		}

		// Ensure unique names by appending the rule index to the TLSRoute name
		tcpHostName := fmt.Sprintf("%s-%s.%s-rule-%d", parentName, tRoute.Namespace, tRoute.Name, 0)
		var backends []ir.BackendRefIR
		for _, backend := range tRoute.Backends {
			// validate that we don't have an error:
			if backend.Err != nil || backend.BackendObject == nil {
				err := backend.Err
				if err == nil {
					err = errors.New("not found")
				}
				for _, parentRefReporter := range parentRefReporters {
					query.ProcessBackendError(err, parentRefReporter)
				}
			}
			// add backend even if we have errors, as according to spec, with multiple destinations,
			// they should fail based of the weights.
			backends = append(backends, backend)
		}
		// Avoid creating a TcpListener if there are no TcpHosts
		if len(backends) == 0 {
			return nil
		}

		// Resolve and translate TLS config for TLS termination (same as TCPRoute)
		resolvedValidation, err := resolveFrontendTLSConfig(tc.parents.listener.Port, frontendTLSConfig)
		if err != nil {
			// An error and a non-nil validation means that the listener is partially valid,
			// and we should continue to translate the listener after writing the error to status
			reportTLSConfigError(err, tc.listenerReporter, resolvedValidation != nil)
			if resolvedValidation == nil {
				return nil
			}
		}

		tlsConfig, err := translateTLSConfig(kctx, ctx, tc.parents.listener, tc.tls, queries, resolvedValidation, gatewayNamespace)
		if err != nil {
			// An error and a non-nil tlsConfig means that the listener is partially valid,
			// and we should continue to translate the listener after writing the error to status
			reportTLSConfigError(err, tc.listenerReporter, tlsConfig != nil)
			if tlsConfig == nil {
				return nil
			}
		}

		if tlsConfig != nil && len(tlsConfig.AlpnProtocols) == 0 {
			tlsConfig.AlpnProtocols = []string{string(annotations.AllowEmptyAlpnProtocols)}
		}

		var matcher ir.FilterChainMatch
		matcher.SniDomains = slices.Clone(tc.sniDomains)

		return &ir.TcpIR{
			FilterChainCommon: ir.FilterChainCommon{
				FilterChainName: tcpHostName,
				StatPrefix:      buildStatPrefix("tls", tc.parents.listener.Port, tRoute.Namespace, tRoute.Name),
				Matcher:         matcher,
				TLS:             tlsConfig,
			},
			BackendRefs:      backends,
			AttachedPolicies: tc.parents.listener.AttachedPolicies,
		}
	default:
		return nil
	}
}

// rejectConflictingRoute sets Programmed=False on the specific listener
// attachment of a route that lost to another route attached to the same
// listener.
func rejectConflictingRoute(ri *query.RouteInfo, message string, reporter reports.Reporter) {
	condition := reports.RouteCondition{
		Type:    conditions.KgatewayConditionProgrammed,
		Status:  metav1.ConditionFalse,
		Reason:  gwv1.RouteConditionReason("Conflicted"),
		Message: message,
	}
	switch o := ri.Object.(type) {
	case *ir.TcpRouteIR:
		reporter.Route(o.SourceObject).ParentRef(&ri.ParentRef).SetCondition(condition)
	case *ir.TlsRouteIR:
		reporter.Route(o.SourceObject).ParentRef(&ri.ParentRef).SetCondition(condition)
	}
}

// reportBackendErrors reports the backend errors of a TCP or TLS route on the
// specific listener attachment, for a route that is not translated.
func reportBackendErrors(ri *query.RouteInfo, reporter reports.Reporter) {
	var parentRefReporter reports.ParentRefReporter
	var backends []ir.BackendRefIR
	switch o := ri.Object.(type) {
	case *ir.TcpRouteIR:
		parentRefReporter = reporter.Route(o.SourceObject).ParentRef(&ri.ParentRef)
		backends = o.Backends
	case *ir.TlsRouteIR:
		parentRefReporter = reporter.Route(o.SourceObject).ParentRef(&ri.ParentRef)
		backends = o.Backends
	default:
		return
	}
	for _, backend := range backends {
		if backend.Err != nil || backend.BackendObject == nil {
			err := backend.Err
			if err == nil {
				err = errors.New("not found")
			}
			query.ProcessBackendError(err, parentRefReporter)
		}
	}
}

// httpFilterChain each one represents a GW Listener that has been merged into a single Listener (with distinct filter chains).
// In the case where no GW Listener merging takes place, every listener will use a MergedListener with 1 HTTP filter chain.
type httpFilterChain struct {
	parents []httpFilterChainParent
}

func isHostContained(host string, maybeLhost *gwv1.Hostname) bool {
	if maybeLhost == nil {
		return true
	}
	listenerHostname := string(*maybeLhost)
	if strings.HasPrefix(listenerHostname, "*.") {
		if strings.HasSuffix(host, listenerHostname[1:]) {
			return true
		}
	}
	return host == listenerHostname
}

type httpFilterChainParent struct {
	gatewayListenerName string
	gatewayListener     ir.Listener
	routesWithHosts     []*query.RouteInfo
	attachedPolicies    ir.AttachedPolicies
	listenerReporter    reports.ListenerReporter
}

func filterHTTPListenerIsolationHostnames(parent httpFilterChainParent, parents []httpFilterChainParent, hostnames []string) []string {
	filteredHostnames := make([]string, 0, len(hostnames))
	for _, hostname := range hostnames {
		if hostnameOwnedByMoreSpecificHTTPListener(parent, parents, hostname) {
			continue
		}
		filteredHostnames = append(filteredHostnames, hostname)
	}
	return filteredHostnames
}

func hostnameOwnedByMoreSpecificHTTPListener(parent httpFilterChainParent, parents []httpFilterChainParent, routeHostname string) bool {
	for _, candidate := range parents {
		if candidate.gatewayListenerName == parent.gatewayListenerName {
			continue
		}
		if !listenerHostnameCoversRouteHostname(candidate.gatewayListener.Hostname, routeHostname) {
			continue
		}
		if listenerHostnamePrecedes(parent.gatewayListener.Hostname, candidate.gatewayListener.Hostname) {
			return true
		}
	}
	return false
}

func listenerHostnameCoversRouteHostname(listenerHostname *gwv1.Hostname, routeHostname string) bool {
	if listenerHostname == nil || *listenerHostname == "" {
		return true
	}
	if routeHostname == "*" {
		return false
	}

	intersection, ok := query.IntersectHostnames(string(*listenerHostname), routeHostname)
	return ok && intersection == routeHostname
}

func listenerHostnamePrecedes(current, candidate *gwv1.Hostname) bool {
	currentRank, currentSpecificity := listenerHostnamePrecedence(current)
	candidateRank, candidateSpecificity := listenerHostnamePrecedence(candidate)
	if currentRank != candidateRank {
		return candidateRank > currentRank
	}
	return candidateSpecificity > currentSpecificity
}

func listenerHostnamePrecedence(hostname *gwv1.Hostname) (int, int) {
	if hostname == nil || *hostname == "" {
		return 0, 0
	}

	hostnameString := string(*hostname)
	if after, ok := strings.CutPrefix(hostnameString, "*."); ok {
		return 1, strings.Count(after, ".")
	}

	return 2, 0
}

func (httpFilterChain *httpFilterChain) translateHttpFilterChain(
	ctx context.Context,
	parentName string,
	reporter reports.Reporter,
) ir.HttpFilterChainIR {
	routesByHost := map[string]routeutils.SortableRoutes{}
	for _, parent := range httpFilterChain.parents {
		buildRoutesPerHostWithHostnamesFilter(
			ctx,
			routesByHost,
			parent.routesWithHosts,
			reporter,
			func(hostnames []string) []string {
				return filterHTTPListenerIsolationHostnames(parent, httpFilterChain.parents, hostnames)
			},
		)
	}

	var (
		virtualHostNames = map[string]bool{}
		virtualHosts     = []*ir.VirtualHost{}
	)
	for host, vhostRoutes := range routesByHost {
		// find the parent this host belongs to, and use its policies
		var (
			attachedPolicies ir.AttachedPolicies
			listenerRef      ir.Listener
		)
		maxHostnameLen := -1
		for _, p := range httpFilterChain.parents {
			// does this listener's hostname match our host?
			if !isHostContained(host, p.gatewayListener.Hostname) {
				continue
			}
			// calculate the length of the hostname; most specific match wins
			var hostnameLen int
			if p.gatewayListener.Hostname != nil {
				hostnameLen = len(string(*p.gatewayListener.Hostname))
			}
			if hostnameLen > maxHostnameLen {
				attachedPolicies = p.attachedPolicies
				listenerRef = p.gatewayListener
				maxHostnameLen = hostnameLen
			}
		}

		// ensure we sort the routes before creating the vhost
		slices.SortStableFunc(vhostRoutes, func(a, b *routeutils.SortableRoute) int {
			return a.CompareTo(b)
		})

		// ensure we don't create duplicate vhosts
		vhostName := makeVhostName(ctx, parentName, host)
		if virtualHostNames[vhostName] {
			continue
		}
		virtualHostNames[vhostName] = true

		virtualHosts = append(virtualHosts, &ir.VirtualHost{
			Name:             vhostName,
			Hostname:         host,
			Rules:            vhostRoutes.ToRoutes(),
			AttachedPolicies: attachedPolicies,
			ParentRef:        listenerRef,
		})
	}
	// sort vhosts, to make sure the resource is stable
	slices.SortFunc(virtualHosts, func(a, b *ir.VirtualHost) int {
		return cmp.Compare(a.Name, b.Name)
	})

	// TODO: Make a similar change for other filter chains ???
	return ir.HttpFilterChainIR{
		FilterChainCommon: ir.FilterChainCommon{
			FilterChainName: parentName,
			StatPrefix:      buildStatPrefix("http", httpFilterChain.parents[0].gatewayListener.Port),
		},
		// Http plain text filter chains do not have attached policies.
		// Because a single chain is shared across multiple gateway-api listeners, we don't have a clean way
		// of applying listener level policies.
		// For route policies this is not an issue, as they will be applied on the vhost.
		// This is a problem for example if section name on HttpListener policy.
		// it won't attach in that case..
		// i'm pretty sure this is what we want, as we can't attach HCM policies to only some of the vhosts.
		Vhosts: virtualHosts,
	}
}

type httpsFilterChain struct {
	gatewayListenerName string
	// Although the parent gateway is the same for all listeners,
	// they can belong to different listenersets in different namespaces
	listener         ir.Listener
	listenerReporter reports.ListenerReporter
	sniDomain        *gwv1.Hostname
	tls              *gwv1.ListenerTLSConfig
	routesWithHosts  []*query.RouteInfo
	attachedPolicies ir.AttachedPolicies
}

func (hfc *httpsFilterChain) translateHttpsFilterChain(
	kctx krt.HandlerContext,
	ctx context.Context,
	queries query.GatewayQueries,
	reporter reports.Reporter,
	frontendTLSConfig *ir.FrontendTLSConfigIR,
	gatewayNamespace string,
) (*ir.HttpFilterChainIR, error) {
	// process routes first, so any route related errors are reported on the httproute.
	routesByHost := map[string]routeutils.SortableRoutes{}
	buildRoutesPerHost(
		ctx,
		routesByHost,
		hfc.routesWithHosts,
		reporter,
	)

	var (
		virtualHostNames = map[string]bool{}
		virtualHosts     = []*ir.VirtualHost{}
	)
	for host, vhostRoutes := range routesByHost {
		slices.SortStableFunc(vhostRoutes, func(a, b *routeutils.SortableRoute) int {
			return a.CompareTo(b)
		})
		vhostName := makeVhostName(ctx, hfc.gatewayListenerName, host)
		if !virtualHostNames[vhostName] {
			virtualHostNames[vhostName] = true
			virtualHost := &ir.VirtualHost{
				Name:     vhostName,
				Hostname: host,
				Rules:    vhostRoutes.ToRoutes(),
			}
			virtualHosts = append(virtualHosts, virtualHost)
		}
	}

	var matcher ir.FilterChainMatch
	if hfc.sniDomain != nil {
		matcher.SniDomains = []string{string(*hfc.sniDomain)}
	}

	resolvedValidation, err := resolveFrontendTLSConfig(hfc.listener.Port, frontendTLSConfig)
	if err != nil {
		reportTLSConfigError(err, hfc.listenerReporter, resolvedValidation != nil)
		// An error and a non-nil validation means that the listener is partially valid,
		// and we should continue to translate the listener after writing the error to status
		if resolvedValidation == nil {
			return nil, err
		}
	}

	tlsConfig, err := translateTLSConfig(
		kctx,
		ctx,
		hfc.listener,
		hfc.tls,
		queries,
		resolvedValidation,
		gatewayNamespace,
	)
	if err != nil {
		// An error and a non-nil tlsConfig means that the listener is partially valid,
		// and we should continue to translate the listener after writing the error to status
		reportTLSConfigError(err, hfc.listenerReporter, tlsConfig != nil)
		if tlsConfig == nil {
			return nil, err
		}
	}
	slices.SortFunc(virtualHosts, func(a, b *ir.VirtualHost) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return &ir.HttpFilterChainIR{
		FilterChainCommon: ir.FilterChainCommon{
			FilterChainName: hfc.gatewayListenerName,
			Listener:        &hfc.listener,
			StatPrefix:      buildStatPrefix("https", hfc.listener.Port, listenerStatSegments(hfc.listener)...),
			Matcher:         matcher,
			TLS:             tlsConfig,
		},
		AttachedPolicies: hfc.attachedPolicies,
		Vhosts:           virtualHosts,
	}, nil
}

func buildRoutesPerHost(
	ctx context.Context,
	routesByHost map[string]routeutils.SortableRoutes,
	routes []*query.RouteInfo,
	reporter reports.Reporter,
) {
	buildRoutesPerHostWithHostnamesFilter(ctx, routesByHost, routes, reporter, nil)
}

type routeHostnamesFilter func([]string) []string

func buildRoutesPerHostWithHostnamesFilter(
	ctx context.Context,
	routesByHost map[string]routeutils.SortableRoutes,
	routes []*query.RouteInfo,
	reporter reports.Reporter,
	filter routeHostnamesFilter,
) {
	for _, routeWithHosts := range routes {
		parentRefReporter := reporter.Route(routeWithHosts.Object.GetSourceObject()).ParentRef(&routeWithHosts.ParentRef)
		routes := route.TranslateGatewayHTTPRouteRules(
			ctx,
			routeWithHosts,
			parentRefReporter,
			reporter,
		)
		if len(routes) == 0 {
			// TODO report
			continue
		}

		hostnames := routeHostnamesOrCatchAll(routeWithHosts)
		if filter != nil {
			hostnames = filter(hostnames)
		}
		if len(hostnames) == 0 {
			continue
		}
		routeHostnames := specHostnames(routeWithHosts)
		for _, host := range hostnames {
			routeHostname := routeutils.MostSpecificRouteHostname(routeHostnames, host)
			routesByHost[host] = append(routesByHost[host], routeutils.ToSortable(routeWithHosts.Object.GetSourceObject(), routeHostname, routes)...)
		}
	}
}

// specHostnames returns the hostnames set on the route itself, before they are
// intersected with the listener hostname.
func specHostnames(routeInfo *query.RouteInfo) []string {
	switch r := routeInfo.Object.(type) {
	case *ir.HttpRouteIR:
		return r.GetHostnames()
	case *ir.TlsRouteIR:
		return r.GetHostnames()
	default:
		return nil
	}
}

func routeHostnamesOrCatchAll(routeWithHosts *query.RouteInfo) []string {
	hostnames := routeWithHosts.Hostnames()
	if len(hostnames) == 0 {
		return []string{"*"}
	}
	return hostnames
}

// resolveFrontendTLSConfig resolves the FrontendTLSConfig for a specific port.
// Per-port configuration takes precedence over default configuration.
// Returns nil if no FrontendTLSConfig is present or no validation is configured.
// Returns an error if the FrontendTLSConfig contains an error from processing.
// NOTE: Because a listener can be partially valid when there are multiple certificate references, this function can return both a
// ClientCertificateValidationIR and an error.
// The validationIR represents the validation configuration that could be successfully translated, and the error can be used to write status about the config that couldn't.
func resolveFrontendTLSConfig(port gwv1.PortNumber, frontendTLSConfig *ir.FrontendTLSConfigIR) (*ir.ClientCertificateValidationIR, error) {
	if frontendTLSConfig == nil {
		return nil, nil
	}

	// Check and return in order:
	// 1. Per-port errors
	// 2. Per-port validation
	// 3. Default errors/validation
	if frontendTLSConfig.PortErrors[port] != nil {
		return frontendTLSConfig.PerPortValidation[port], frontendTLSConfig.PortErrors[port]
	}

	if perPortConfig, ok := frontendTLSConfig.PerPortValidation[port]; ok {
		return perPortConfig, nil
	}

	return frontendTLSConfig.DefaultValidation, frontendTLSConfig.DefaultError
}

// NOTE: Because a listener can be partially valid when there are multiple certificate references, this function can return both a ClientCertificateValidationIR
// and an error. The IR respresents the TLS config that could be successfully translated, and the error can be used to write status about the config that couldn't.
func translateTLSConfig(
	kctx krt.HandlerContext,
	ctx context.Context,
	listener ir.Listener,
	tls *gwv1.ListenerTLSConfig,
	queries query.GatewayQueries,
	resolvedValidation *ir.ClientCertificateValidationIR,
	gatewayNamespace string,
) (*ir.TLSConfig, error) {
	if tls == nil {
		return nil, nil
	}
	if tls.Mode == nil ||
		*tls.Mode != gwv1.TLSModeTerminate {
		return nil, nil
	}

	var alpnProtocols []string
	if tls.Options[annotations.AlpnProtocols] != "" {
		alpnProtocols = strings.Split(string(tls.Options[annotations.AlpnProtocols]), ",")
	}

	var certificates []ir.TLSCertificate
	var certErr error
	for _, certRef := range tls.CertificateRefs {
		parentGVK := listener.Parent.GetObjectKind().GroupVersionKind()
		if parentGVK.Empty() {
			switch listener.Parent.(type) {
			case *gwv1.Gateway:
				parentGVK = wellknown.GatewayGVK
			case *gwv1.ListenerSet:
				parentGVK = wellknown.XListenerSetGVK
			}
		}

		// validate secret reference exists
		secret, err := queries.GetSecretForRef(
			kctx,
			ctx,
			parentGVK.GroupKind(),
			listener.Parent.GetNamespace(),
			certRef,
		)
		if err != nil {
			certErr = errors.Join(certErr, err)
			continue
		}

		// Only the validation result is used here; the IR carries the Secret's data as is.
		if err := secret.ValidateTLS(); err != nil {
			certErr = errors.Join(certErr, err)
			continue
		}

		certChain := secret.Data[corev1.TLSCertKey]
		privateKey := secret.Data[corev1.TLSPrivateKeyKey]
		rootCa := secret.Data[corev1.ServiceAccountRootCAKey]

		certificates = append(certificates, ir.TLSCertificate{
			PrivateKey: privateKey,
			CertChain:  certChain,
			CA:         rootCa,
		})
	}
	if len(certificates) == 0 && certErr != nil {
		return nil, certErr
	}

	tlsConfig := &ir.TLSConfig{
		AlpnProtocols: alpnProtocols,
		Certificates:  certificates,
	}
	if err := sslutils.ApplyTLSExtensionOptions(tls.Options, tlsConfig); err != nil {
		return nil, err
	}

	// Validate that VerifySubjectAltNames requires a trusted CA to be configured
	hasTrustedCA := resolvedValidation != nil && len(resolvedValidation.CACertificateRefs) > 0
	if len(tlsConfig.VerifySubjectAltNames) > 0 && !hasTrustedCA {
		return nil, sslutils.ErrVerifySubjectAltNamesRequiresCA
	}

	// Apply client certificate validation if present
	// Skip if CA cert refs are empty (no validation possible)
	var caErr error
	var generated bool
	if hasTrustedCA {
		// For AllowInsecureFallback mode, if CA cert fetching fails, skip validation rather than failing the listener
		// This allows the listener to work without client certs even if the CA cert ConfigMap is missing
		generated, caErr = applyClientCertificateValidation(kctx, ctx, queries, resolvedValidation, tlsConfig, wellknown.GatewayGVK, gatewayNamespace)
		if !generated {
			if resolvedValidation.RequireClientCertificate {
				// If client certs are required (AllowValidOnly), fail the listener
				logger.Error("failed to fetch CA certificate for client validation, failing listener",
					"listener", listener.Name,
					"port", listener.Port,
					"error", caErr,
					"mode", "AllowValidOnly")
				return nil, errors.Join(certErr, caErr)
			}
			// If client certs are not required (AllowInsecureFallback), log the error but don't fail the listener
			// The listener will still work for connections without client certs
			// Skip setting Gateway-level ClientCertificateValidation - per-listener validation will be attempted if exists
			logger.Warn("failed to fetch CA certificate for client validation, skipping validation",
				"listener", listener.Name,
				"port", listener.Port,
				"error", caErr,
				"mode", "AllowInsecureFallback")
		}
	}

	// Check if ListenerPolicy has clientCertificateValidation override
	if listenerPolCertVal := getCertValidationFromAttached(listener); listenerPolCertVal != nil {
		// Apply ListenerPolicy override, which takes precedence over Gateway-level config
		listenerParentGVK := listener.Parent.GetObjectKind().GroupVersionKind()
		if listenerParentGVK.Empty() {
			switch listener.Parent.(type) {
			case *gwv1.Gateway:
				listenerParentGVK = wellknown.GatewayGVK
			case *gwv1.ListenerSet:
				listenerParentGVK = wellknown.XListenerSetGVK
			default:
				return nil, fmt.Errorf("unsupported parent type for ListenerPolicy clientCertificateValidation: %T", listener.Parent)
			}
		}
		generated, caErr = applyClientCertificateValidation(kctx, ctx, queries, listenerPolCertVal, tlsConfig, listenerParentGVK, listener.Parent.GetNamespace())
		if !generated {
			if !listenerPolCertVal.RequireClientCertificate {
				logger.Warn("failed to fetch CA certificate for ListenerPolicy client validation override, skipping validation",
					"listener", listener.Name,
					"port", listener.Port,
					"error", caErr,
					"mode", "optional")
				// Keep Gateway-level validation if ListenerPolicy fetch fails
				return tlsConfig, nil
			}
			logger.Error("failed to fetch CA certificate for ListenerPolicy client validation override, failing listener",
				"listener", listener.Name,
				"port", listener.Port,
				"error", caErr,
				"mode", "required")
			return nil, errors.Join(certErr, caErr)
		}
	}

	return tlsConfig, errors.Join(certErr, caErr)
}

// buildCaCertificateReference fetches and extracts a CA certificate from either a ConfigMap or Secret
// referenced by the given ObjectReference. Returns the CA certificate data as a string.
func buildCaCertificateReference(
	kctx krt.HandlerContext,
	ctx context.Context,
	queries query.GatewayQueries,
	caCertRef gwv1.ObjectReference,
	parentGVK schema.GroupVersionKind,
	parentNamespace string,
) (string, error) {
	switch {
	case string(caCertRef.Group) == wellknown.ConfigMapGVK.Group && string(caCertRef.Kind) == wellknown.ConfigMapGVK.Kind:
		// Fetch ConfigMap
		configMap, err := queries.GetConfigMapForRef(
			kctx,
			ctx,
			parentGVK.GroupKind(),
			parentNamespace,
			caCertRef,
		)
		if err != nil {
			// If its a missing reference grant error, return the CA Certificate specific error
			if errors.Is(err, krtcollections.ErrMissingReferenceGrant) {
				return "", fmt.Errorf("failed to fetch CA certificate ConfigMap %s/%s: %w", caCertRef.Name, parentNamespace, sslutils.ErrMissingCaCertificateRefGrant)
			}
			// If its not a missing reference grant error, return the invalid certificate ref error
			return "", fmt.Errorf("failed to fetch CA certificate ConfigMap %s/%s: %w", caCertRef.Name, parentNamespace, sslutils.ErrInvalidCACertificateRef)
		}

		// Extract CA certificate from ConfigMap
		caCertData, err := sslutils.GetCACertFromConfigMap(configMap)
		if err != nil {
			return "", fmt.Errorf("failed to extract CA certificate from ConfigMap %s/%s: %w", configMap.Namespace, configMap.Name, err)
		}
		return caCertData, nil

	case string(caCertRef.Group) == wellknown.SecretGVK.Group && string(caCertRef.Kind) == wellknown.SecretGVK.Kind:
		// Convert ObjectReference to SecretObjectReference
		secretObjRef := gwv1.SecretObjectReference{
			Name:      caCertRef.Name,
			Namespace: caCertRef.Namespace,
		}
		if caCertRef.Group != "" {
			group := gwv1.Group(caCertRef.Group)
			secretObjRef.Group = &group
		}
		if caCertRef.Kind != "" {
			kind := gwv1.Kind(caCertRef.Kind)
			secretObjRef.Kind = &kind
		}

		secret, err := queries.GetSecretForRef(
			kctx,
			ctx,
			parentGVK.GroupKind(),
			parentNamespace,
			secretObjRef,
		)
		if err != nil {
			return "", fmt.Errorf("failed to fetch CA certificate Secret %s/%s: %w", caCertRef.Name, parentNamespace, err)
		}

		// Extract CA certificate from Secret
		caCertData, err := sslutils.GetCACertFromSecret(secret)
		if err != nil {
			return "", fmt.Errorf("failed to extract CA certificate from Secret %s/%s: %w", secret.Namespace, secret.Name, err)
		}
		return caCertData, nil

	// Should never happen as we validate the reference type in validateCAReferenceType
	default:
		return "", fmt.Errorf("unsupported CA certificate reference type: %s/%s", caCertRef.Group, caCertRef.Kind)
	}
}

// getCertValidationFromAttached aggregates ClientCertificateValidation from all attached ListenerPolicies.
// Returns nil if no ListenerPolicy is found or if none have clientCertificateValidation configured.
func getCertValidationFromAttached(listener ir.Listener) *ir.ClientCertificateValidationIR {
	if listener.AttachedPolicies.Policies == nil {
		return nil
	}

	// Extract ListenerPolicyIR from attached policies
	polAttachments := listener.AttachedPolicies.Policies[wellknown.ListenerPolicyGVK.GroupKind()]
	if len(polAttachments) == 0 {
		return nil
	}

	// Aggregate ClientCertificateValidation from all attached ListenerPolicies.
	// Collect unique CA cert refs and mark required if any policy requires it.
	var aggregatedCerts []gwv1.ObjectReference
	certSet := make(map[string]bool)
	requireClientCert := false

	for _, polAtt := range polAttachments {
		listenerPol, ok := polAtt.PolicyIr.(*listenerpolicy.ListenerPolicyIR)
		if !ok || listenerPol == nil {
			continue
		}
		certVal := listenerPol.GetClientCertificateValidation()
		if certVal == nil {
			continue
		}

		for _, certRef := range certVal.CACertificateRefs {
			key, err := json.Marshal(certRef)
			if err != nil {
				continue
			}

			if !certSet[string(key)] {
				certSet[string(key)] = true
				aggregatedCerts = append(aggregatedCerts, certRef)
			}
		}

		requireClientCert = requireClientCert || certVal.RequireClientCertificate
	}

	// Return nil if no certs were found
	if len(aggregatedCerts) == 0 {
		return nil
	}

	return &ir.ClientCertificateValidationIR{
		CACertificateRefs:        aggregatedCerts,
		RequireClientCertificate: requireClientCert,
	}
}

// applyClientCertificateValidation applies the resolved client certificate validation configuration
// to the TLS config by fetching CA certificates and setting validation parameters.
// Returns a boolean indicating if any client certificate validation was applied successfully and an error if any errors were encountered.
func applyClientCertificateValidation(
	kctx krt.HandlerContext,
	ctx context.Context,
	queries query.GatewayQueries,
	validationConfig *ir.ClientCertificateValidationIR,
	tlsConfig *ir.TLSConfig,
	parentGVK schema.GroupVersionKind,
	parentNamespace string,
) (bool, error) {
	if validationConfig == nil {
		return true, nil
	}

	// Fetch CA certificates from ConfigMaps or Secrets
	var caCertificates [][]byte

	var certErr error
	for _, caCertRef := range validationConfig.CACertificateRefs {
		caCertData, err := buildCaCertificateReference(
			kctx,
			ctx,
			queries,
			caCertRef,
			parentGVK,
			parentNamespace,
		)
		if err != nil {
			certErr = errors.Join(certErr, err)
			continue
		}

		caCertificates = append(caCertificates, []byte(caCertData))
	}

	// Only set ClientCertificateValidation if we successfully fetched at least one CA cert
	if len(caCertificates) == 0 {
		return false, certErr
	}

	// Set client certificate validation in TLS config
	tlsConfig.ClientCertificateValidation = &ir.ClientCertificateValidation{
		CACertificates:           caCertificates,
		RequireClientCertificate: validationConfig.RequireClientCertificate,
		AllowInsecureFallback:    validationConfig.AllowInsecureFallback,
	}

	return true, certErr
}

// reportTLSConfigError reports TLS configuration errors by setting appropriate listener conditions.
func reportTLSConfigError(err error, listenerReporter reports.ListenerReporter, partiallyValid bool) {
	reason := gwv1.ListenerReasonInvalidCertificateRef
	message := "Invalid certificate ref(s)."
	acceptedReason := gwv1.ListenerReasonInvalid
	resolvedRefsOK := false

	switch {
	case errors.Is(err, krtcollections.ErrMissingReferenceGrant):
		reason = gwv1.ListenerReasonRefNotPermitted
		message = "Reference not permitted by ReferenceGrant."
	case errors.Is(err, sslutils.ErrMissingCaCertificateRefGrant):
		reason = gwv1.ListenerReasonRefNotPermitted
		acceptedReason = sslutils.ListenerReasonNoValidCACertificate
		message = err.Error()
	case errors.Is(err, sslutils.ErrInvalidTlsSecret):
		message = err.Error()
	case errors.Is(err, sslutils.ErrVerifySubjectAltNamesRequiresCA):
		// verify-subject-alt-names requires CA is a TLS configuration issue,
		// not a certificate reference problem — the secret refs resolved fine.
		resolvedRefsOK = true
		message = err.Error()
	case errors.Is(err, sslutils.ErrInvalidCACertificateRef):
		reason = sslutils.ListenerReasonInvalidCACertificateRef
		acceptedReason = sslutils.ListenerReasonNoValidCACertificate
		message = err.Error()
	case errors.Is(err, sslutils.ErrInvalidCACertificateKind):
		reason = sslutils.ListenerReasonInvalidCACertificateKind
		acceptedReason = sslutils.ListenerReasonNoValidCACertificate
		message = err.Error()
	case errors.Is(err, sslutils.ErrUnknownTLSExtensionOption):
		// Unknown TLS extension options are not a certificate reference problem;
		// the cert refs resolved fine. Per the Gateway API spec, this is an
		// Invalid condition on Programmed/Accepted, not a ResolvedRefs issue.
		resolvedRefsOK = true
		message = err.Error()
	}

	if notFoundErr, ok := errors.AsType[*krtcollections.NotFoundError](err); ok {
		resourceType := notFoundErr.NotFoundObj.Kind
		if resourceType == "" {
			resourceType = "Resource"
		}
		message = fmt.Sprintf(ResourceNotFoundMessageTemplate, resourceType, notFoundErr.NotFoundObj.Namespace, notFoundErr.NotFoundObj.Name)
	}

	// When resolvedRefsOK is true, do not set a ResolvedRefs condition at all
	// — listenerConditionsWithDefaults sets ResolvedRefs=True as the default
	// for any condition type that was not explicitly set. We only need to
	// override it when references genuinely failed to resolve.
	if !resolvedRefsOK {
		listenerReporter.SetCondition(reports.ListenerCondition{
			Type:    gwv1.ListenerConditionResolvedRefs,
			Status:  metav1.ConditionFalse,
			Reason:  reason,
			Message: message,
		})
	}

	// Accepted and Programmed conditions are set to true if the listener is partially valid
	acceptedProgrammedStatus := metav1.ConditionFalse
	if partiallyValid {
		acceptedProgrammedStatus = metav1.ConditionTrue
	}

	listenerReporter.SetCondition(reports.ListenerCondition{
		Type:    gwv1.ListenerConditionProgrammed,
		Status:  acceptedProgrammedStatus,
		Reason:  gwv1.ListenerReasonInvalid,
		Message: message,
	})

	listenerReporter.SetCondition(reports.ListenerCondition{
		Type:    gwv1.ListenerConditionAccepted,
		Status:  acceptedProgrammedStatus,
		Reason:  acceptedReason,
		Message: message,
	})
}

// makeVhostName computes the name of a virtual host based on the parent name and domain.
func makeVhostName(
	ctx context.Context,
	parentName string,
	domain string,
) string {
	return utils.SanitizeForEnvoy(ctx, parentName+"~"+domain, "vHost")
}

func GenerateListenerName(listener ir.Listener) string {
	return GenerateListenerNameFromPort(listener.Port)
}

func GenerateListenerNameFromPort(port gwv1.PortNumber) string {
	// Add a ~ to make sure the name won't collide with user provided names in other listeners
	return fmt.Sprintf("listener~%d", port)
}

// Stat prefixes surface in user-facing metrics (e.g. the
// envoy_http_conn_manager_prefix and envoy_tcp_prefix tags), so they follow
// their own stable format instead of reusing filter chain names:
//
//	http_<port>                                        plain HTTP (all listeners on the port share one chain)
//	https_<port>_<section>                             HTTPS Gateway listener
//	https_<port>_<lsNamespace>_<lsName>_<section>      HTTPS ListenerSet listener
//	tcp_<port>_<routeNamespace>_<routeName>            TCPRoute
//	tls_<port>_<routeNamespace>_<routeName>            TLSRoute
//
// Segments are joined with "_", which Kubernetes names never contain, and the
// IR translator replaces "." with "_" so each prefix is a single stat segment.
func buildStatPrefix(protocol string, port gwv1.PortNumber, segments ...string) string {
	return strings.Join(append([]string{protocol, strconv.Itoa(int(port))}, segments...), "_")
}

// listenerStatSegments identifies a Gateway API listener within its Gateway:
// the section name alone for a Gateway, prefixed by namespace and name for a
// ListenerSet.
func listenerStatSegments(listener ir.Listener) []string {
	if _, ok := listener.Parent.(*gwv1.Gateway); ok || listener.Parent == nil {
		return []string{string(listener.Name)}
	}
	return []string{listener.Parent.GetNamespace(), listener.Parent.GetName(), string(listener.Name)}
}
