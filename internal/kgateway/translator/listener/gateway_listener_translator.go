package listener

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ports"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	route "github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/httproute"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/routeutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/sslutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

var logger = logging.New("translator/listener")

type ListenerTranslatorConfig struct {
	ListenerBindIpv6 bool
}

// TranslateListeners translates the set of gloo listeners required to produce a full output proxy (either form one Gateway or multiple merged Gateways)
func TranslateListeners(
	kctx krt.HandlerContext,
	ctx context.Context,
	queries query.GatewayQueries,
	gateway *ir.Gateway,
	routesForGw *query.RoutesForGwResult,
	reporter reports.Reporter,
	settings ListenerTranslatorConfig,
) []ir.ListenerIR {
	validatedListeners := validateGateway(gateway, reporter)

	mergedListeners := mergeGWListeners(queries, gateway.Namespace, validatedListeners, *gateway, routesForGw, reporter, settings)
	translatedListeners := mergedListeners.translateListeners(kctx, ctx, queries, reporter)
	return translatedListeners
}

func mergeGWListeners(
	queries query.GatewayQueries,
	gatewayNamespace string,
	listeners []ir.Listener,
	parentGw ir.Gateway,
	routesForGw *query.RoutesForGwResult,
	reporter reports.Reporter,
	settings ListenerTranslatorConfig,
) *MergedListeners {
	ml := &MergedListeners{
		parentGw:         parentGw,
		GatewayNamespace: gatewayNamespace,
		Queries:          queries,
		settings:         settings,
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
	GatewayNamespace string
	parentGw         ir.Gateway
	Listeners        []*MergedListener
	Queries          query.GatewayQueries
	settings         ListenerTranslatorConfig
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
	}

	fc := &httpFilterChain{
		parents: []httpFilterChainParent{parent},
	}
	listenerName := GenerateListenerName(listener)
	finalPort := gwv1.PortNumber(ports.TranslatePort(uint16(listener.Port)))

	for _, lis := range ml.Listeners {
		if lis.port == finalPort {
			if lis.httpFilterChain != nil {
				lis.httpFilterChain.parents = append(lis.httpFilterChain.parents, parent)
			} else {
				lis.httpFilterChain = fc
			}
			return
		}
	}

	// create a new filter chain for the listener
	ml.Listeners = append(ml.Listeners, &MergedListener{
		name:             listenerName,
		gatewayNamespace: ml.GatewayNamespace,
		port:             finalPort,
		httpFilterChain:  fc,
		listenerReporter: reporter,
		listener:         listener,
		gateway:          ml.parentGw,
		settings:         ml.settings,
	})
}

func (ml *MergedListeners) appendHttpsListener(
	listener ir.Listener,
	routesWithHosts []*query.RouteInfo,
	reporter reports.ListenerReporter,
) {
	// create a new filter chain for the listener
	// protocol:            listener.Protocol,

	mfc := httpsFilterChain{
		gatewayListenerName: query.GenerateRouteKey(listener.Parent, string(listener.Name)),
		sniDomain:           listener.Hostname,
		tls:                 listener.TLS,
		routesWithHosts:     routesWithHosts,
		attachedPolicies:    listener.AttachedPolicies,
	}

	// Perform the port transformation away from privileged ports only once to use
	// during both lookup and when appending the listener.
	finalPort := gwv1.PortNumber(ports.TranslatePort(uint16(listener.Port)))

	listenerName := GenerateListenerName(listener)
	for _, lis := range ml.Listeners {
		if lis.port == finalPort {
			lis.httpsFilterChains = append(lis.httpsFilterChains, mfc)
			return
		}
	}
	ml.Listeners = append(ml.Listeners, &MergedListener{
		name:              listenerName,
		gatewayNamespace:  ml.GatewayNamespace,
		port:              finalPort,
		httpsFilterChains: []httpsFilterChain{mfc},
		listenerReporter:  reporter,
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
	var validRouteInfos []*query.RouteInfo

	for _, routeInfo := range routeInfos {
		tRoute, ok := routeInfo.Object.(*ir.TcpRouteIR)
		if !ok {
			continue
		}

		if len(tRoute.ParentRefs) == 0 {
			logger.Warn("no parent references found for TCPRoute", "resource_ref", tRoute.ResourceName())
			continue
		}

		validRouteInfos = append(validRouteInfos, routeInfo)
	}

	// If no valid routes are found, do not create a listener
	if len(validRouteInfos) == 0 {
		logger.Error("no valid routes found for listener", "listener", listener.Name)
		return
	}

	parent := tcpFilterChainParent{
		gatewayListenerName: query.GenerateRouteKey(listener.Parent, string(listener.Name)),
		routesWithHosts:     validRouteInfos,
	}

	fc := tcpFilterChain{
		parents: parent,
	}
	listenerName := GenerateListenerName(listener)
	finalPort := gwv1.PortNumber(ports.TranslatePort(uint16(listener.Port)))

	for _, lis := range ml.Listeners {
		if lis.port == finalPort {
			lis.TcpFilterChains = append(lis.TcpFilterChains, fc)
			return
		}
	}

	// create a new filter chain for the listener
	ml.Listeners = append(ml.Listeners, &MergedListener{
		name:             listenerName,
		gatewayNamespace: ml.GatewayNamespace,
		port:             finalPort,
		TcpFilterChains:  []tcpFilterChain{fc},
		listenerReporter: reporter,
		listener:         listener,
		gateway:          ml.parentGw,
		settings:         ml.settings,
	})
}

func (ml *MergedListeners) AppendTlsListener(
	listener ir.Listener,
	routeInfos []*query.RouteInfo,
	reporter reports.ListenerReporter,
) {
	var validRouteInfos []*query.RouteInfo

	for _, routeInfo := range routeInfos {
		tRoute, ok := routeInfo.Object.(*ir.TlsRouteIR)
		if !ok {
			continue
		}

		if len(tRoute.ParentRefs) == 0 {
			logger.Warn("no parent references found for TLSRoute", "resource_ref", tRoute.ResourceName())
			continue
		}

		validRouteInfos = append(validRouteInfos, routeInfo)
	}

	// If no valid routes are found, do not create a listener
	if len(validRouteInfos) == 0 {
		logger.Error("no valid routes found for listener", "listener", listener.Name)
		return
	}

	parent := tcpFilterChainParent{
		gatewayListenerName: query.GenerateRouteKey(listener.Parent, string(listener.Name)),
		routesWithHosts:     validRouteInfos,
	}

	fc := tcpFilterChain{
		parents:   parent,
		tls:       listener.TLS,
		sniDomain: listener.Hostname,
	}

	listenerName := GenerateListenerName(listener)
	finalPort := gwv1.PortNumber(ports.TranslatePort(uint16(listener.Port)))

	for _, lis := range ml.Listeners {
		if lis.port == finalPort {
			lis.TcpFilterChains = append(lis.TcpFilterChains, fc)
			return
		}
	}

	// create a new filter chain for the listener
	ml.Listeners = append(ml.Listeners, &MergedListener{
		name:             listenerName,
		gatewayNamespace: ml.GatewayNamespace,
		port:             finalPort,
		TcpFilterChains:  []tcpFilterChain{fc},
		listenerReporter: reporter,
		listener:         listener,
		settings:         ml.settings,
	})
}

func (ml *MergedListeners) translateListeners(
	kctx krt.HandlerContext,
	ctx context.Context,
	queries query.GatewayQueries,
	reporter reports.Reporter,
) []ir.ListenerIR {
	var listeners []ir.ListenerIR
	for _, mergedListener := range ml.Listeners {
		listener := mergedListener.TranslateListener(kctx, ctx, queries, reporter)

		// run listener plugins
		//		panic("TODO: handle listener policy attachment")
		// for _, listenerPlugin := range pluginRegistry.GetListenerPlugins() {
		// err := listenerPlugin.ApplyListenerPlugin(ctx, &plugins.ListenerContext{
		// 	Gateway:    &ml.parentGw,
		// 	GwListener: &mergedListener.listener,
		// }, listener)
		// if err != nil {
		// 	contextutils.LoggerFrom(ctx).Errorf("error in ListenerPlugin: %v", err)
		// }
		// }

		listeners = append(listeners, listener)
	}
	return listeners
}

type MergedListener struct {
	name              string
	gatewayNamespace  string
	port              gwv1.PortNumber
	httpFilterChain   *httpFilterChain
	httpsFilterChains []httpsFilterChain
	TcpFilterChains   []tcpFilterChain
	listenerReporter  reports.ListenerReporter
	listener          ir.Listener
	gateway           ir.Gateway
	settings          ListenerTranslatorConfig

	// TODO(policy via http listener options)
}

func (ml *MergedListener) TranslateListener(
	kctx krt.HandlerContext,
	ctx context.Context,
	queries query.GatewayQueries,
	reporter reports.Reporter,
) ir.ListenerIR {
	var (
		httpFilterChains    []ir.HttpFilterChainIR
		matchedTcpListeners []ir.TcpIR
	)

	// Translate HTTP filter chains
	if ml.httpFilterChain != nil {
		httpFilterChain := ml.httpFilterChain.translateHttpFilterChain(
			ctx,
			ml.name,
			ml.listener,
			ml.gateway,
			reporter,
		)
		httpFilterChains = append(httpFilterChains, httpFilterChain)
		/* TODO: not sure why this logic is here, vhosts can duplicate across filter chains. and name should be unique
		for vhostRef, vhost := range vhostsForFilterchain {
			if _, ok := mergedVhosts[vhostRef]; ok {
				// Handle potential error if duplicate vhosts are found
				contextutils.LoggerFrom(ctx).Errorf(
					"Duplicate virtual host found: %s", vhostRef,
				)
				continue
			}
			mergedVhosts[vhostRef] = vhost
		}
		*/
	}

	// Translate HTTPS filter chains
	for _, mfc := range ml.httpsFilterChains {
		httpsFilterChain := mfc.translateHttpsFilterChain(
			kctx,
			ctx,
			mfc.gatewayListenerName,
			ml.gatewayNamespace,
			ml.listener,
			queries,
			reporter,
			ml.listenerReporter,
		)
		if httpsFilterChain == nil {
			// Log and skip invalid HTTPS filter chains
			logger.Error("failed to translate HTTPS filter chain for listener", "listener", ml.name)
			continue
		}

		httpFilterChains = append(httpFilterChains, *httpsFilterChain)
		/* TODO: not sure why this logic is here, vhosts can duplicate across filter chains. and name should be unique

		for vhostRef, vhost := range vhostsForFilterchain {
			if _, ok := mergedVhosts[vhostRef]; ok {
				logger.Error("Duplicate virtual host found", "vhostRef", vhostRef)
				continue
			}
			mergedVhosts[vhostRef] = vhost
		}
		*/
	}

	// Translate TCP listeners (if any exist)
	for _, tfc := range ml.TcpFilterChains {
		if tcpListener := tfc.translateTcpFilterChain(ml.listener, ml.name, reporter); tcpListener != nil {
			matchedTcpListeners = append(matchedTcpListeners, *tcpListener)
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
		BindPort:          uint32(ml.port),
		AttachedPolicies:  ir.AttachedPolicies{}, // TODO: find policies attached to listener and attach them <- this might not be possilbe due to listener merging. also a gw listener ~= envoy filter chain; and i don't believe we need policies there
		HttpFilterChain:   httpFilterChains,
		TcpFilterChain:    matchedTcpListeners,
		PolicyAncestorRef: ml.listener.PolicyAncestorRef,
	}
}

// tcpFilterChain each one represents a Gateway listener that has been merged into a single Gloo Listener
// (with distinct filter chains). In the case where no Gateway listener merging takes place, every listener
// will use a Gloo AggregatedListener with one TCP filter chain.
type tcpFilterChain struct {
	parents   tcpFilterChainParent
	tls       *gwv1.GatewayTLSConfig
	sniDomain *gwv1.Hostname
}

type tcpFilterChainParent struct {
	gatewayListenerName string
	routesWithHosts     []*query.RouteInfo
}

func (tc *tcpFilterChain) translateTcpFilterChain(listener ir.Listener, parentName string, reporter reports.Reporter) *ir.TcpIR {
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
		// Collect ParentRefReporters for the TCPRoute
		parentRefReporters := make([]reports.ParentRefReporter, 0, len(tRoute.ParentRefs))

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

		for _, parentRef := range tRoute.ParentRefs {
			parentRefReporter := reporter.Route(tRoute.SourceObject).ParentRef(&parentRef)
			parentRefReporter.SetCondition(condition)
			parentRefReporters = append(parentRefReporters, parentRefReporter)
		}

		if condition.Status != metav1.ConditionTrue {
			return nil
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

		return &ir.TcpIR{
			FilterChainCommon: ir.FilterChainCommon{
				FilterChainName: tcpHostName,
			},
			BackendRefs: backends,
		}
	case *ir.TlsRouteIR:
		tRoute := r.Object.(*ir.TlsRouteIR)

		parentRefReporters := make([]reports.ParentRefReporter, 0, len(tRoute.ParentRefs))

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

		for _, parentRef := range tRoute.ParentRefs {
			parentRefReporter := reporter.Route(tRoute.SourceObject).ParentRef(&parentRef)
			parentRefReporter.SetCondition(condition)
			parentRefReporters = append(parentRefReporters, parentRefReporter)
		}

		if condition.Status != metav1.ConditionTrue {
			return nil
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

// httpFilterChain each one represents a GW Listener that has been merged into a single Gloo Listener (with distinct filter chains).
// In the case where no GW Listener merging takes place, every listener will use a Gloo AggregatedListeener with 1 HTTP filter chain.
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
}

func (httpFilterChain *httpFilterChain) translateHttpFilterChain(
	ctx context.Context,
	parentName string,
	listener ir.Listener,
	gw ir.Gateway,
	reporter reports.Reporter,
) ir.HttpFilterChainIR {
	routesByHost := map[string]routeutils.SortableRoutes{}
	for _, parent := range httpFilterChain.parents {
		buildRoutesPerHost(
			ctx,
			routesByHost,
			parent.routesWithHosts,
			listener,
			reporter,
		)
	}

	var (
		virtualHostNames = map[string]bool{}
		virtualHosts     = []*ir.VirtualHost{}
	)
	for host, vhostRoutes := range routesByHost {
		// find the parent this host belongs to, and use its policies
		var attachedPolicies ir.AttachedPolicies
		maxHostnameLen := -1
		for _, p := range httpFilterChain.parents {
			if isHostContained(host, p.gatewayListener.Hostname) {
				hostnameLen := 0
				if p.gatewayListener.Hostname != nil {
					hostnameLen = len(string(*p.gatewayListener.Hostname))
				}
				if hostnameLen > maxHostnameLen {
					attachedPolicies = p.attachedPolicies
					maxHostnameLen = hostnameLen
				}
			}
		}

		sort.Stable(vhostRoutes)
		vhostName := makeVhostName(ctx, parentName, host)
		if !virtualHostNames[vhostName] {
			virtualHostNames[vhostName] = true
			virtualHost := &ir.VirtualHost{
				Name:             vhostName,
				Hostname:         host,
				Rules:            vhostRoutes.ToRoutes(),
				AttachedPolicies: attachedPolicies,
			}
			virtualHosts = append(virtualHosts, virtualHost)
		}
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
	sniDomain           *gwv1.Hostname
	tls                 *gwv1.GatewayTLSConfig
	routesWithHosts     []*query.RouteInfo
	attachedPolicies    ir.AttachedPolicies
}

func (httpsFilterChain *httpsFilterChain) translateHttpsFilterChain(
	kctx krt.HandlerContext,
	ctx context.Context,
	parentName string,
	gatewayNamespace string,
	listener ir.Listener,
	queries query.GatewayQueries,
	reporter reports.Reporter,
	listenerReporter reports.ListenerReporter,
) *ir.HttpFilterChainIR {
	// process routes first, so any route related errors are reported on the httproute.
	routesByHost := map[string]routeutils.SortableRoutes{}
	buildRoutesPerHost(
		ctx,
		routesByHost,
		httpsFilterChain.routesWithHosts,
		listener,
		reporter,
	)

	var (
		virtualHostNames = map[string]bool{}
		virtualHosts     = []*ir.VirtualHost{}
	)
	for host, vhostRoutes := range routesByHost {
		sort.Stable(vhostRoutes)
		vhostName := makeVhostName(ctx, parentName, host)
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

	if httpsFilterChain.sniDomain != nil {
		matcher.SniDomains = []string{string(*httpsFilterChain.sniDomain)}
	}

	sslConfig, err := translateSslConfig(
		kctx,
		ctx,
		gatewayNamespace,
		httpsFilterChain.tls,
		queries,
	)
	if err != nil {
		reason := gwv1.ListenerReasonRefNotPermitted
		if !errors.Is(err, krtcollections.ErrMissingReferenceGrant) {
			reason = gwv1.ListenerReasonInvalidCertificateRef
		}
		listenerReporter.SetCondition(reports.ListenerCondition{
			Type:   gwv1.ListenerConditionResolvedRefs,
			Status: metav1.ConditionFalse,
			Reason: reason,
		})
		// listener with no ssl is invalid. We return nil so set programmed to false
		listenerReporter.SetCondition(reports.ListenerCondition{
			Type:   gwv1.ListenerConditionProgrammed,
			Status: metav1.ConditionFalse,
			Reason: gwv1.ListenerReasonInvalid,
		})
		return nil
	}
	sort.Slice(virtualHosts, func(i, j int) bool {
		return virtualHosts[i].Name < virtualHosts[j].Name
	})
	return &ir.HttpFilterChainIR{
		FilterChainCommon: ir.FilterChainCommon{
			FilterChainName: parentName,
			Matcher:         matcher,
			TLS:             sslConfig,
		},
		AttachedPolicies: httpsFilterChain.attachedPolicies,
		Vhosts:           virtualHosts,
	}
}

func buildRoutesPerHost(
	ctx context.Context,
	routesByHost map[string]routeutils.SortableRoutes,
	routes []*query.RouteInfo,
	gwListener ir.Listener,
	reporter reports.Reporter,
) {
	// func() { panic("TODO: handle policy attachment") }()
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

func translateSslConfig(
	kctx krt.HandlerContext,
	ctx context.Context,
	parentNamespace string,
	tls *gwv1.GatewayTLSConfig,
	queries query.GatewayQueries,
) (*ir.TlsBundle, error) {
	if tls == nil {
		return nil, nil
	}

	if tls.Mode == nil ||
		*tls.Mode != gwv1.TLSModeTerminate {
		return nil, nil
	}

	for _, certRef := range tls.CertificateRefs {
		// validate via query
		secret, err := queries.GetSecretForRef(kctx, ctx, schema.GroupKind{
			Group: gwv1.GroupName,
			Kind:  "Gateway",
		},
			parentNamespace,
			certRef)
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

		return &ir.TlsBundle{
			PrivateKey: privateKey,
			CertChain:  certChain,
			CA:         rootCa,
		}, nil
		// TODO support multiple certs
	}

	return nil, nil
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
	// Add a ~ to make sure the name won't collide with user provided names in other listeners
	return fmt.Sprintf("listener~%d", listener.Port)
}
