package listener

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/api/annotations"
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

	validatedListeners := validateGateway(gateway, reporter, settings)
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
		if result == nil || result.Error != nil {
			// TODO report
			// TODO, if Error is not nil, this is a user-config error on selectors
			// continue
		}
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
	parent := tcpFilterChainParent{
		gatewayListenerName: query.GenerateRouteKey(listener.Parent, string(listener.Name)),
		listener:            listener,
		listenerReporter:    reporter,
		routesWithHosts:     routeInfos,
	}
	tls := listener.TLS
	if tls == nil {
		tls = &gwv1.ListenerTLSConfig{}
	}
	fc := tcpFilterChain{
		parents:          parent,
		tls:              tls,
		sniDomain:        listener.Hostname,
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
		settings:        ml.settings,
	})
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

	// Translate HTTPS filter chains
	for _, mfc := range ml.httpsFilterChains {
		httpsFilterChain, err := mfc.translateHttpsFilterChain(
			kctx,
			ctx,
			queries,
			reporter,
			ml.gateway.FrontendTLSConfig,
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
		if tcpListener := tfc.translateTcpFilterChain(kctx, ctx, queries, ml.name, reporter, ml.gateway.FrontendTLSConfig); tcpListener != nil {
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
	sniDomain        *gwv1.Hostname
	listenerReporter reports.ListenerReporter
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
) *ir.TcpIR {
	parent := tc.parents
	if len(parent.routesWithHosts) == 0 {
		return nil
	}

	if len(parent.routesWithHosts) > 1 {
		// Only one route per listener is supported
		// TODO: report error on the listener
		//	reporter.Gateway(gw).SetCondition(reports.RouteCondition{
		//		Type:   gwv1.RouteConditionPartiallyInvalid,
		//		Status: metav1.ConditionTrue,
		//		Reason: gwv1.RouteReasonUnsupportedValue,
		//	})
	}
	r := slices.MinFunc(parent.routesWithHosts, func(a, b *query.RouteInfo) int {
		return a.Object.GetSourceObject().GetCreationTimestamp().Compare(b.Object.GetSourceObject().GetCreationTimestamp().Time)
	})

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

		tlsConfig, err := translateTLSConfig(kctx, ctx, tc.parents.listener, tc.tls, queries, resolvedValidation)
		if err != nil {
			// An error and a non-nil tlsCsonfig means that the listener is partially valid,
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
				TLS:             tlsConfig,
			},
			BackendRefs: backends,
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

		var matcher ir.FilterChainMatch
		if tc.sniDomain != nil {
			matcher.SniDomains = []string{string(*tc.sniDomain)}
		}

		return &ir.TcpIR{
			FilterChainCommon: ir.FilterChainCommon{
				FilterChainName: tcpHostName,
				Matcher:         matcher,
			},
			BackendRefs: backends,
		}
	default:
		return nil
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

func (httpFilterChain *httpFilterChain) translateHttpFilterChain(
	ctx context.Context,
	parentName string,
	reporter reports.Reporter,
) ir.HttpFilterChainIR {
	routesByHost := map[string]routeutils.SortableRoutes{}
	for _, parent := range httpFilterChain.parents {
		buildRoutesPerHost(
			ctx,
			routesByHost,
			parent.routesWithHosts,
			reporter,
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
		sort.Stable(vhostRoutes)

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
	sort.Slice(virtualHosts, func(i, j int) bool {
		return virtualHosts[i].Name < virtualHosts[j].Name
	})

	// TODO: Make a similar change for other filter chains ???
	return ir.HttpFilterChainIR{
		FilterChainCommon: ir.FilterChainCommon{
			FilterChainName: parentName,
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
		sort.Stable(vhostRoutes)
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
	)
	if err != nil {
		// An error and a non-nil tlsConfig means that the listener is partially valid,
		// and we should continue to translate the listener after writing the error to status
		reportTLSConfigError(err, hfc.listenerReporter, tlsConfig != nil)
		if tlsConfig == nil {
			return nil, err
		}
	}
	sort.Slice(virtualHosts, func(i, j int) bool {
		return virtualHosts[i].Name < virtualHosts[j].Name
	})

	return &ir.HttpFilterChainIR{
		FilterChainCommon: ir.FilterChainCommon{
			FilterChainName: hfc.gatewayListenerName,
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

		hostnames := routeWithHosts.Hostnames()
		if len(hostnames) == 0 {
			hostnames = []string{"*"}
		}
		for _, host := range hostnames {
			routesByHost[host] = append(routesByHost[host], routeutils.ToSortable(routeWithHosts.Object.GetSourceObject(), routes)...)
		}
	}
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
	for _, certRef := range tls.CertificateRefs {
		parentGVK := listener.Parent.GetObjectKind().GroupVersionKind()
		if parentGVK.Empty() {
			switch listener.Parent.(type) {
			case *gwv1.Gateway:
				parentGVK = wellknown.GatewayGVK
			case *gwxv1a1.XListenerSet:
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
			return nil, err
		}

		// The resulting sslconfig will still have to go through a real translation where we run through this again.
		// This means that while its nice to still fail early here we dont need to scrub the actual contents of the secret.
		if _, err := sslutils.ValidateTlsSecretData(secret.Name, secret.Namespace, secret.Data); err != nil {
			return nil, err
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
		generated, caErr = applyClientCertificateValidation(kctx, ctx, queries, listener, resolvedValidation, tlsConfig)
		if !generated {
			// If client certs are not required (AllowInsecureFallback), log the error but don't fail the listener
			// The listener will still work for connections without client certs
			if !resolvedValidation.RequireClientCertificate {
				logger.Warn("failed to fetch CA certificate for client validation, skipping validation",
					"listener", listener.Name,
					"port", listener.Port,
					"error", caErr,
					"mode", "AllowInsecureFallback")
				// Don't set ClientCertificateValidation - listener will work without client cert validation
				return tlsConfig, nil
			}
			// If client certs are required (AllowValidOnly), fail the listener
			logger.Warn("failed to fetch CA certificate for client validation, failing listener",
				"listener", listener.Name,
				"port", listener.Port,
				"error", caErr,
				"mode", "AllowValidOnly")
			return nil, caErr
		}
	}

	return tlsConfig, caErr
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

// applyClientCertificateValidation applies the resolved client certificate validation configuration
// to the TLS config by fetching CA certificates and setting validation parameters.
// Returns a boolean indicating if any client certificate validation was applied successfully and an error if any errors were encountered.
func applyClientCertificateValidation(
	kctx krt.HandlerContext,
	ctx context.Context,
	queries query.GatewayQueries,
	listener ir.Listener,
	validationConfig *ir.ClientCertificateValidationIR,
	tlsConfig *ir.TLSConfig,
) (bool, error) {
	if validationConfig == nil {
		return true, nil
	}

	// Fetch CA certificates from ConfigMaps or Secrets
	var caCertificates [][]byte
	parentGVK := listener.Parent.GetObjectKind().GroupVersionKind()
	if parentGVK.Empty() {
		switch listener.Parent.(type) {
		case *gwv1.Gateway:
			parentGVK = wellknown.GatewayGVK
		case *gwxv1a1.XListenerSet:
			parentGVK = wellknown.XListenerSetGVK
		default:
			return false, fmt.Errorf("unsupported parent type: %T", listener.Parent)
		}
	}

	var certErr error
	for _, caCertRef := range validationConfig.CACertificateRefs {
		caCertData, err := buildCaCertificateReference(
			kctx,
			ctx,
			queries,
			caCertRef,
			parentGVK,
			listener.Parent.GetNamespace(),
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
	}

	return true, certErr
}

// reportTLSConfigError reports TLS configuration errors by setting appropriate listener conditions.
func reportTLSConfigError(err error, listenerReporter reports.ListenerReporter, partiallyValid bool) {
	reason := gwv1.ListenerReasonInvalidCertificateRef
	message := "Invalid certificate ref(s)."
	acceptedReason := gwv1.ListenerReasonInvalid

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
		message = err.Error()
	case errors.Is(err, sslutils.ErrInvalidCACertificateRef):
		reason = sslutils.ListenerReasonInvalidCACertificateRef
		acceptedReason = sslutils.ListenerReasonNoValidCACertificate
		message = err.Error()
	case errors.Is(err, sslutils.ErrInvalidCACertificateKind):
		reason = sslutils.ListenerReasonInvalidCACertificateKind
		acceptedReason = sslutils.ListenerReasonNoValidCACertificate
		message = err.Error()
	}

	var notFoundErr *krtcollections.NotFoundError
	if errors.As(err, &notFoundErr) {
		resourceType := notFoundErr.NotFoundObj.Kind
		if resourceType == "" {
			resourceType = "Resource"
		}
		message = fmt.Sprintf(ResourceNotFoundMessageTemplate, resourceType, notFoundErr.NotFoundObj.Namespace, notFoundErr.NotFoundObj.Name)
	}

	listenerReporter.SetCondition(reports.ListenerCondition{
		Type:    gwv1.ListenerConditionResolvedRefs,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})

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
