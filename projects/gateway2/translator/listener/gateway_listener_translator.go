package listener

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/contextutils"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/ports"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	route "github.com/solo-io/gloo/projects/gateway2/translator/httproute"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/translator/routeutils"
	"github.com/solo-io/gloo/projects/gateway2/translator/sslutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	corev1 "k8s.io/api/core/v1"
)

// TranslateListeners translates the set of gloo listeners required to produce a full output proxy (either form one Gateway or multiple merged Gateways)
func TranslateListeners(
	kctx krt.HandlerContext,
	ctx context.Context,
	queries query.GatewayQueries,
	pluginRegistry registry.PluginRegistry,
	gateway *ir.Gateway,
	routesForGw *query.RoutesForGwResult,
	reporter reports.Reporter,
) []ir.ListenerIR {
	validatedListeners := validateListeners(gateway, reporter.Gateway(gateway.Obj))

	mergedListeners := mergeGWListeners(queries, gateway.Namespace, validatedListeners, *gateway, routesForGw, reporter.Gateway(gateway.Obj))
	translatedListeners := mergedListeners.translateListeners(kctx, ctx, pluginRegistry, queries, reporter)
	return translatedListeners
}

func mergeGWListeners(
	queries query.GatewayQueries,
	gatewayNamespace string,
	listeners []ir.Listener,
	parentGw ir.Gateway,
	routesForGw *query.RoutesForGwResult,
	reporter reports.GatewayReporter,
) *MergedListeners {
	ml := &MergedListeners{
		parentGw:         parentGw,
		GatewayNamespace: gatewayNamespace,
		Queries:          queries,
	}
	for _, listener := range listeners {
		result, ok := routesForGw.ListenerResults[string(listener.Name)]
		if !ok || result.Error != nil {
			// TODO report
			// TODO, if Error is not nil, this is a user-config error on selectors
			// continue
		}
		listenerReporter := reporter.ListenerName(string(listener.Name))
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
	default:
		return eris.Errorf("unsupported protocol: %v", listener.Protocol)
	}

	return nil
}

func (ml *MergedListeners) appendHttpListener(
	listener ir.Listener,
	routesWithHosts []*query.RouteInfo,
	reporter reports.ListenerReporter,
) {
	parent := httpFilterChainParent{
		gatewayListenerName: string(listener.Name),
		routesWithHosts:     routesWithHosts,
	}

	fc := &httpFilterChain{
		parents: []httpFilterChainParent{parent},
	}
	listenerName := string(listener.Name)
	finalPort := gwv1.PortNumber(ports.TranslatePort(uint16(listener.Port)))

	for _, lis := range ml.Listeners {
		if lis.port == finalPort {
			// concatenate the names on the parent output listener/filterchain
			// TODO is this valid listener name?
			lis.name += "~" + listenerName
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
		gatewayListenerName: string(listener.Name),
		sniDomain:           listener.Hostname,
		tls:                 listener.TLS,
		routesWithHosts:     routesWithHosts,
		queries:             ml.Queries,
	}

	// Perform the port transformation away from privileged ports only once to use
	// during both lookup and when appending the listener.
	finalPort := gwv1.PortNumber(ports.TranslatePort(uint16(listener.Port)))

	listenerName := string(listener.Name)
	for _, lis := range ml.Listeners {
		if lis.port == finalPort {
			// concatenate the names on the parent output listener
			// TODO is this valid listener name?
			lis.name += "~" + listenerName
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
			contextutils.LoggerFrom(context.Background()).Warnf(
				"No parent references found for TCPRoute %s", tRoute.Name,
			)
			continue
		}

		validRouteInfos = append(validRouteInfos, routeInfo)
	}

	// If no valid routes are found, do not create a listener
	if len(validRouteInfos) == 0 {
		contextutils.LoggerFrom(context.Background()).Errorf(
			"No valid routes found for listener %s", listener.Name,
		)
		return
	}

	parent := tcpFilterChainParent{
		gatewayListenerName: string(listener.Name),
		routesWithHosts:     validRouteInfos,
	}

	fc := tcpFilterChain{
		parents: parent,
	}
	listenerName := string(listener.Name)
	finalPort := gwv1.PortNumber(ports.TranslatePort(uint16(listener.Port)))

	for _, lis := range ml.Listeners {
		if lis.port == finalPort {
			// concatenate the names on the parent output listener
			lis.name += "~" + listenerName
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
	})
}

func getWeight(backendRef gwv1.BackendRef) *wrapperspb.UInt32Value {
	if backendRef.Weight != nil {
		return &wrapperspb.UInt32Value{Value: uint32(*backendRef.Weight)}
	}
	// Default weight is 1
	return &wrapperspb.UInt32Value{Value: 1}
}

func (ml *MergedListeners) translateListeners(
	kctx krt.HandlerContext,
	ctx context.Context,
	pluginRegistry registry.PluginRegistry,
	queries query.GatewayQueries,
	reporter reports.Reporter,
) []ir.ListenerIR {
	var listeners []ir.ListenerIR
	for _, mergedListener := range ml.Listeners {
		listener := mergedListener.TranslateListener(kctx, ctx, pluginRegistry, queries, reporter)

		// run listener plugins
		panic("TODO: handle listener policy attachment")
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

	// TODO(policy via http listener options)
}

func (ml *MergedListener) TranslateListener(
	kctx krt.HandlerContext,
	ctx context.Context,
	pluginRegistry registry.PluginRegistry,
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
			pluginRegistry,
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
			pluginRegistry,
			mfc.gatewayListenerName,
			ml.gatewayNamespace,
			ml.listener,
			queries,
			reporter,
			ml.listenerReporter,
		)
		if httpsFilterChain == nil {
			// Log and skip invalid HTTPS filter chains
			contextutils.LoggerFrom(ctx).Errorf("Failed to translate HTTPS filter chain for listener: %s", ml.name)
			continue
		}

		httpFilterChains = append(httpFilterChains, *httpsFilterChain)
		/* TODO: not sure why this logic is here, vhosts can duplicate across filter chains. and name should be unique

		for vhostRef, vhost := range vhostsForFilterchain {
			if _, ok := mergedVhosts[vhostRef]; ok {
				contextutils.LoggerFrom(ctx).Errorf("Duplicate virtual host found: %s", vhostRef)
				continue
			}
			mergedVhosts[vhostRef] = vhost
		}
		*/
	}

	// Translate TCP listeners (if any exist)
	for _, tfc := range ml.TcpFilterChains {
		if tcpListener := tfc.translateTcpFilterChain(ml.listener, reporter); tcpListener != nil {
			matchedTcpListeners = append(matchedTcpListeners, *tcpListener)
		}
	}

	// Create and return the listener with all filter chains and TCP listeners
	panic("TODO: handle listener policy attachment")
	return ir.ListenerIR{
		Name:             ml.name,
		BindAddress:      "::",
		BindPort:         uint32(ml.port),
		AttachedPolicies: ir.AttachedPolicies{}, // TODO: find policies attached to listener and attach them
		HttpFilterChain:  httpFilterChains,
		TcpFilterChain:   matchedTcpListeners,
	}
}

// tcpFilterChain each one represents a Gateway listener that has been merged into a single Gloo Listener
// (with distinct filter chains). In the case where no Gateway listener merging takes place, every listener
// will use a Gloo AggregatedListener with one TCP filter chain.
type tcpFilterChain struct {
	parents tcpFilterChainParent
}

type tcpFilterChainParent struct {
	gatewayListenerName string
	routesWithHosts     []*query.RouteInfo
}

func (tc *tcpFilterChain) translateTcpFilterChain(listener ir.Listener, reporter reports.Reporter) *ir.TcpIR {
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

	tRoute, ok := r.Object.(*ir.TcpRouteIR)
	if !ok {
		return nil
	}

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
	tcpHostName := fmt.Sprintf("%s.%s-rule-%d", tRoute.Namespace, tRoute.Name, 0)
	var backends []ir.Backend
	for _, backend := range tRoute.Backends {
		// validate that we don't have an error:
		if backend.Err != nil || backend.Upstream == nil {
			err := backend.Err
			if err == nil {
				err = errors.New("not found")
			}
			for _, parentRefReporter := range parentRefReporters {
				query.ProcessBackendError(err, parentRefReporter)
			}
		} else {
			backends = append(backends, backend)
		}

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
}

// httpFilterChain each one represents a GW Listener that has been merged into a single Gloo Listener (with distinct filter chains).
// In the case where no GW Listener merging takes place, every listener will use a Gloo AggregatedListeener with 1 HTTP filter chain.
type httpFilterChain struct {
	parents []httpFilterChainParent
}

type httpFilterChainParent struct {
	gatewayListenerName string
	routesWithHosts     []*query.RouteInfo
}

func (httpFilterChain *httpFilterChain) translateHttpFilterChain(
	ctx context.Context,
	parentName string,
	listener ir.Listener,
	pluginRegistry registry.PluginRegistry,
	reporter reports.Reporter,
) ir.HttpFilterChainIR {
	routesByHost := map[string]routeutils.SortableRoutes{}
	for _, parent := range httpFilterChain.parents {
		buildRoutesPerHost(
			ctx,
			routesByHost,
			parent.routesWithHosts,
			listener,
			pluginRegistry,
			reporter,
		)
	}

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
				// TODO: figurout attached polices
			}
			panic("TODO: handle vhost policy attachment")

			virtualHosts = append(virtualHosts, virtualHost)
		}
	}

	return ir.HttpFilterChainIR{
		FilterChainCommon: ir.FilterChainCommon{
			FilterChainName: string(listener.Name),
		},
		Vhosts: virtualHosts,
	}
}

type httpsFilterChain struct {
	gatewayListenerName string
	sniDomain           *gwv1.Hostname
	tls                 *gwv1.GatewayTLSConfig
	routesWithHosts     []*query.RouteInfo
	queries             query.GatewayQueries
}

func (httpsFilterChain *httpsFilterChain) translateHttpsFilterChain(
	kctx krt.HandlerContext,
	ctx context.Context,
	pluginRegistry registry.PluginRegistry,
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
		pluginRegistry,
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
			panic("TODO: handle vhost policy attachment")
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
		if !errors.Is(err, query.ErrMissingReferenceGrant) {
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

	return &ir.HttpFilterChainIR{
		FilterChainCommon: ir.FilterChainCommon{
			FilterChainName: string(listener.Name),
			Matcher:         matcher,
			TLS:             sslConfig,
		},
		Vhosts: virtualHosts,
	}
}

func buildRoutesPerHost(
	ctx context.Context,
	routesByHost map[string]routeutils.SortableRoutes,
	routes []*query.RouteInfo,
	gwListener ir.Listener,
	pluginRegistry registry.PluginRegistry,
	reporter reports.Reporter,
) {
	panic("TODO: handle policy attachment")
	for _, routeWithHosts := range routes {
		parentRefReporter := reporter.Route(routeWithHosts.Object.GetSourceObject()).ParentRef(&routeWithHosts.ParentRef)
		routes := route.TranslateGatewayHTTPRouteRules(
			ctx,
			pluginRegistry,
			gwListener.Listener,
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

	// TODO support passthrough mode
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
