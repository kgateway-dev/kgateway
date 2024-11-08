package listener

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"google.golang.org/protobuf/types/known/wrapperspb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/solo-io/gloo/projects/gateway2/ports"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	route "github.com/solo-io/gloo/projects/gateway2/translator/httproute"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/translator/routeutils"
	"github.com/solo-io/gloo/projects/gateway2/translator/sslutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

// TranslateListeners translates the set of gloo listeners required to produce a full output proxy (either form one Gateway or multiple merged Gateways)
func TranslateListeners(
	ctx context.Context,
	queries query.GatewayQueries,
	pluginRegistry registry.PluginRegistry,
	gateway *gwv1.Gateway,
	routesForGw *query.RoutesForGwResult,
	reporter reports.Reporter,
) []*v1.Listener {
	validatedListeners := validateListeners(gateway, reporter.Gateway(gateway))

	mergedListeners := mergeGWListeners(queries, gateway.Namespace, validatedListeners, *gateway, routesForGw, reporter.Gateway(gateway))
	translatedListeners := mergedListeners.translateListeners(ctx, pluginRegistry, queries, reporter)
	return translatedListeners
}

func mergeGWListeners(
	queries query.GatewayQueries,
	gatewayNamespace string,
	listeners []gwv1.Listener,
	parentGw gwv1.Gateway,
	routesForGw *query.RoutesForGwResult,
	reporter reports.GatewayReporter,
) *MergedListeners {
	ml := &MergedListeners{
		parentGw:         parentGw,
		gatewayNamespace: gatewayNamespace,
		queries:          queries,
	}
	for _, listener := range listeners {
		result, ok := routesForGw.ListenerResults[string(listener.Name)]
		if !ok || result.Error != nil {
			// TODO report
			// TODO, if Error is not nil, this is a user-config error on selectors
			// continue
		}
		listenerReporter := reporter.Listener(&listener)
		var routes []*query.RouteInfo
		if result != nil {
			routes = result.Routes
		}
		ml.AppendListener(listener, routes, listenerReporter)
	}
	return ml
}

type MergedListeners struct {
	gatewayNamespace string
	parentGw         gwv1.Gateway
	Listeners        []*MergedListener
	queries          query.GatewayQueries
}

func (ml *MergedListeners) AppendListener(
	listener gwv1.Listener,
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
	listener gwv1.Listener,
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
		gatewayNamespace: ml.gatewayNamespace,
		port:             finalPort,
		httpFilterChain:  fc,
		listenerReporter: reporter,
		listener:         listener,
	})
}

func (ml *MergedListeners) appendHttpsListener(
	listener gwv1.Listener,
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
		queries:             ml.queries,
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
		gatewayNamespace:  ml.gatewayNamespace,
		port:              finalPort,
		httpsFilterChains: []httpsFilterChain{mfc},
		listenerReporter:  reporter,
		listener:          listener,
	})
}

func (ml *MergedListeners) AppendTcpListener(
	listener gwv1.Listener,
	routeInfos []*query.RouteInfo,
	reporter reports.ListenerReporter,
) {
	var validRouteInfos []*query.RouteInfo

	for _, routeInfo := range routeInfos {
		tRoute, ok := routeInfo.Object.(*gwv1a2.TCPRoute)
		if !ok {
			continue
		}

		if len(tRoute.Spec.ParentRefs) == 0 {
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
		parents: []tcpFilterChainParent{parent},
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
		gatewayNamespace: ml.gatewayNamespace,
		port:             finalPort,
		TcpFilterChains:  []tcpFilterChain{fc},
		listenerReporter: reporter,
		listener:         listener,
	})
}

func buildTcpListener(
	tRoute *gwv1a2.TCPRoute,
	defaultPort gwv1.PortNumber,
) *v1.TcpListener {
	var tcpHosts []*v1.TcpHost

	for i, rule := range tRoute.Spec.Rules {
		// Ensure unique names by appending the rule index to the TCPRoute name
		tcpHostName := fmt.Sprintf("%s-rule-%d", tRoute.Name, i)
		tcpHost := buildTcpHost(tcpHostName, defaultPort, rule.BackendRefs)
		tcpHosts = append(tcpHosts, tcpHost)
	}

	return &v1.TcpListener{
		TcpHosts: tcpHosts,
	}
}

// Helper function to build a TcpHost from backend references
func buildTcpHost(
	tcpRouteName string,
	defaultPort gwv1.PortNumber,
	backendRefs []gwv1.BackendRef,
) *v1.TcpHost {
	// If there are no backendRefs, return nil to skip this TcpHost
	if len(backendRefs) == 0 {
		return nil
	}

	// Use the TCPRoute name for the tcpHost name
	tcpHost := &v1.TcpHost{Name: tcpRouteName}

	if len(backendRefs) == 1 {
		backendRef := backendRefs[0]
		port := defaultPort
		if backendRef.Port != nil {
			port = *backendRef.Port
		}
		tcpHost.Destination = buildSingleDestination(backendRef, port)
	} else {
		tcpHost.Destination = buildMultiDestination(backendRefs, defaultPort)
	}

	return tcpHost
}

// Helper function to build a single destination
func buildSingleDestination(
	backendRef gwv1.BackendRef,
	port gwv1.PortNumber,
) *v1.TcpHost_TcpAction {
	namespace := "default"
	if backendRef.Namespace != nil {
		namespace = string(*backendRef.Namespace)
	}

	return &v1.TcpHost_TcpAction{
		Destination: &v1.TcpHost_TcpAction_Single{
			Single: &v1.Destination{
				DestinationType: &v1.Destination_Kube{
					Kube: &v1.KubernetesServiceDestination{
						Ref: &core.ResourceRef{
							Name:      string(backendRef.Name),
							Namespace: namespace,
						},
						Port: uint32(port),
					},
				},
			},
		},
	}
}

// Helper function to build multiple destinations
func buildMultiDestination(
	backendRefs []gwv1.BackendRef,
	defaultPort gwv1.PortNumber,
) *v1.TcpHost_TcpAction {
	var weightedDestinations []*v1.WeightedDestination

	for _, backendRef := range backendRefs {
		namespace := "default"
		if backendRef.Namespace != nil {
			namespace = string(*backendRef.Namespace)
		}

		port := defaultPort
		if backendRef.Port != nil {
			port = *backendRef.Port
		}

		// Use backendRef's weight if set, otherwise default to 0.
		weight := uint32(0)
		if backendRef.Weight != nil {
			weight = uint32(*backendRef.Weight)
		}

		weightedDestinations = append(weightedDestinations, &v1.WeightedDestination{
			Destination: &v1.Destination{
				DestinationType: &v1.Destination_Kube{
					Kube: &v1.KubernetesServiceDestination{
						Ref: &core.ResourceRef{
							Name:      string(backendRef.Name),
							Namespace: namespace,
						},
						Port: uint32(port),
					},
				},
			},
			Weight: wrapperspb.UInt32(weight),
		})
	}

	return &v1.TcpHost_TcpAction{
		Destination: &v1.TcpHost_TcpAction_Multi{
			Multi: &v1.MultiDestination{
				Destinations: weightedDestinations,
			},
		},
	}
}

func (ml *MergedListeners) translateListeners(
	ctx context.Context,
	pluginRegistry registry.PluginRegistry,
	queries query.GatewayQueries,
	reporter reports.Reporter,
) []*v1.Listener {
	var listeners []*v1.Listener
	for _, mergedListener := range ml.Listeners {
		listener := mergedListener.TranslateListener(ctx, pluginRegistry, queries, reporter)

		// run listener plugins
		for _, listenerPlugin := range pluginRegistry.GetListenerPlugins() {
			err := listenerPlugin.ApplyListenerPlugin(ctx, &plugins.ListenerContext{
				Gateway:    &ml.parentGw,
				GwListener: &mergedListener.listener,
			}, listener)
			if err != nil {
				contextutils.LoggerFrom(ctx).Errorf("error in ListenerPlugin: %v", err)
			}
		}

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
	listener          gwv1.Listener

	// TODO(policy via http listener options)
}

func (ml *MergedListener) TranslateListener(
	ctx context.Context,
	pluginRegistry registry.PluginRegistry,
	queries query.GatewayQueries,
	reporter reports.Reporter,
) *v1.Listener {
	var (
		httpFilterChains    []*v1.AggregateListener_HttpFilterChain
		mergedVhosts        = map[string]*v1.VirtualHost{}
		matchedTcpListeners []*v1.MatchedTcpListener
	)

	// Translate HTTP filter chains
	if ml.httpFilterChain != nil {
		httpFilterChain, vhostsForFilterchain := ml.httpFilterChain.translateHttpFilterChain(
			ctx,
			ml.name,
			ml.listener,
			pluginRegistry,
			reporter,
		)
		httpFilterChains = append(httpFilterChains, httpFilterChain)
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
	}

	// Translate HTTPS filter chains
	for _, mfc := range ml.httpsFilterChains {
		httpsFilterChain, vhostsForFilterchain := mfc.translateHttpsFilterChain(
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

		httpFilterChains = append(httpFilterChains, httpsFilterChain)

		for vhostRef, vhost := range vhostsForFilterchain {
			if _, ok := mergedVhosts[vhostRef]; ok {
				contextutils.LoggerFrom(ctx).Errorf("Duplicate virtual host found: %s", vhostRef)
				continue
			}
			mergedVhosts[vhostRef] = vhost
		}
	}

	// Translate TCP listeners (if any exist)
	for _, tfc := range ml.TcpFilterChains {
		tcpListener := tfc.translateTcpFilterChain(ctx, ml.listener, pluginRegistry, reporter)
		matchedTcpListeners = append(matchedTcpListeners, &v1.MatchedTcpListener{
			TcpListener: tcpListener,
		})
	}

	// Create and return the listener with all filter chains and TCP listeners
	return &v1.Listener{
		Name:        ml.name,
		BindAddress: "::",
		BindPort:    uint32(ml.port),
		ListenerType: &v1.Listener_AggregateListener{
			AggregateListener: &v1.AggregateListener{
				HttpResources: &v1.AggregateListener_HttpResources{
					VirtualHosts: mergedVhosts,
					HttpOptions:  nil, // HttpListenerOptions will be added by policy plugins
				},
				HttpFilterChains: httpFilterChains,
				TcpListeners:     matchedTcpListeners,
			},
		},
		Options:      nil, // Listener options will be added by policy plugins
		RouteOptions: nil,
	}
}

// tcpFilterChain each one represents a Gateway listener that has been merged into a single Gloo Listener
// (with distinct filter chains). In the case where no Gateway listener merging takes place, every listener
// will use a Gloo AggregatedListener with one TCP filter chain.
type tcpFilterChain struct {
	parents []tcpFilterChainParent
}

type tcpFilterChainParent struct {
	gatewayListenerName string
	routesWithHosts     []*query.RouteInfo
}

func (tc *tcpFilterChain) translateTcpFilterChain(
	ctx context.Context,
	listener gwv1.Listener,
	pluginRegistry registry.PluginRegistry,
	reporter reports.Reporter,
) *v1.TcpListener {
	var tcpHosts []*v1.TcpHost
	for _, parent := range tc.parents {
		for _, r := range parent.routesWithHosts {
			tRoute, ok := r.Object.(*gwv1a2.TCPRoute)
			if !ok {
				continue
			}
			for i, rule := range tRoute.Spec.Rules {
				// Ensure unique names by appending the rule index to the TCPRoute name
				tcpHostName := fmt.Sprintf("%s-rule-%d", tRoute.Name, i)
				tcpHost := buildTcpHost(tcpHostName, listener.Port, rule.BackendRefs)
				if tcpHost != nil {
					tcpHosts = append(tcpHosts, tcpHost)
				}
			}
		}
	}

	return &v1.TcpListener{
		TcpHosts: tcpHosts,
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
	listener gwv1.Listener,
	pluginRegistry registry.PluginRegistry,
	reporter reports.Reporter,
) (*v1.AggregateListener_HttpFilterChain, map[string]*v1.VirtualHost) {
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
		virtualHostRefs []string
		virtualHosts    = map[string]*v1.VirtualHost{}
	)
	for host, vhostRoutes := range routesByHost {
		sort.Stable(vhostRoutes)
		vhostName := makeVhostName(ctx, parentName, host)
		virtualHosts[vhostName] = &v1.VirtualHost{
			Name:    vhostName,
			Domains: []string{host},
			Routes:  vhostRoutes.ToRoutes(),
			Options: nil,
		}

		virtualHostRefs = append(virtualHostRefs, vhostName)
	}
	sort.Strings(virtualHostRefs)

	return &v1.AggregateListener_HttpFilterChain{
		Matcher:         &v1.Matcher{}, // http filter chain matcher is not used
		VirtualHostRefs: virtualHostRefs,
	}, virtualHosts
}

type httpsFilterChain struct {
	gatewayListenerName string
	sniDomain           *gwv1.Hostname
	tls                 *gwv1.GatewayTLSConfig
	routesWithHosts     []*query.RouteInfo
	queries             query.GatewayQueries
}

func (httpsFilterChain *httpsFilterChain) translateHttpsFilterChain(
	ctx context.Context,
	pluginRegistry registry.PluginRegistry,
	parentName string,
	gatewayNamespace string,
	listener gwv1.Listener,
	queries query.GatewayQueries,
	reporter reports.Reporter,
	listenerReporter reports.ListenerReporter,
) (*v1.AggregateListener_HttpFilterChain, map[string]*v1.VirtualHost) {
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
		virtualHostRefs []string
		virtualHosts    = map[string]*v1.VirtualHost{}
	)
	for host, vhostRoutes := range routesByHost {
		sort.Stable(vhostRoutes)
		vhostName := makeVhostName(ctx, parentName, host)
		virtualHosts[vhostName] = &v1.VirtualHost{
			Name:    vhostName,
			Domains: []string{host},
			Routes:  vhostRoutes.ToRoutes(),
			Options: nil,
		}

		virtualHostRefs = append(virtualHostRefs, vhostName)
	}
	sort.Strings(virtualHostRefs)

	sslConfig, err := translateSslConfig(
		ctx,
		gatewayNamespace,
		httpsFilterChain.sniDomain,
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
		return nil, nil
	}
	matcher := &v1.Matcher{SslConfig: sslConfig, SourcePrefixRanges: nil, PassthroughCipherSuites: nil}

	return &v1.AggregateListener_HttpFilterChain{
		Matcher:         matcher,
		VirtualHostRefs: virtualHostRefs,
	}, virtualHosts
}

func buildRoutesPerHost(
	ctx context.Context,
	routesByHost map[string]routeutils.SortableRoutes,
	routes []*query.RouteInfo,
	gwListener gwv1.Listener,
	pluginRegistry registry.PluginRegistry,
	reporter reports.Reporter,
) {
	for _, routeWithHosts := range routes {
		parentRefReporter := reporter.Route(routeWithHosts.Object).ParentRef(&routeWithHosts.ParentRef)
		routes := route.TranslateGatewayHTTPRouteRules(
			ctx,
			pluginRegistry,
			gwListener,
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
			routesByHost[host] = append(routesByHost[host], routeutils.ToSortable(routeWithHosts.Object, routes)...)
		}
	}
}

func translateSslConfig(
	ctx context.Context,
	parentNamespace string,
	sniDomain *gwv1.Hostname,
	tls *gwv1.GatewayTLSConfig,
	queries query.GatewayQueries,
) (*ssl.SslConfig, error) {
	if tls == nil {
		return nil, nil
	}

	// TODO support passthrough mode
	if tls.Mode == nil ||
		*tls.Mode != gwv1.TLSModeTerminate {
		return nil, nil
	}

	var secretRef *core.ResourceRef
	for _, certRef := range tls.CertificateRefs {
		// validate via query
		secret, err := queries.GetSecretForRef(ctx, query.FromGkNs{
			Gk: metav1.GroupKind{
				Group: gwv1.GroupName,
				Kind:  "Gateway",
			},
			Ns: parentNamespace,
		}, certRef)
		if err != nil {
			return nil, err
		}
		// The resulting sslconfig will still have to go through a real translation where we run through this again.
		// This means that while its nice to still fail early here we dont need to scrub the actual contents of the secret.
		if _, err := sslutils.ValidateTlsSecret(secret.(*corev1.Secret)); err != nil {
			return nil, err
		}

		// TODO verify secret ref / grant using query
		secretNamespace := parentNamespace
		if certRef.Namespace != nil {
			secretNamespace = string(*certRef.Namespace)
		}
		secretRef = &core.ResourceRef{
			Name:      string(certRef.Name),
			Namespace: secretNamespace,
		}
		break // TODO support multiple certs
	}
	if secretRef == nil {
		return nil, nil
	}

	var sniDomains []string
	if sniDomain != nil {
		sniDomains = []string{string(*sniDomain)}
	}
	return &ssl.SslConfig{
		SslSecrets:                    &ssl.SslConfig_SecretRef{SecretRef: secretRef},
		SniDomains:                    sniDomains,
		VerifySubjectAltName:          nil,
		Parameters:                    nil,
		AlpnProtocols:                 nil,
		OneWayTls:                     nil,
		DisableTlsSessionResumption:   nil,
		TransportSocketConnectTimeout: nil,
		OcspStaplePolicy:              0,
	}, nil
}

// makeVhostName computes the name of a virtual host based on the parent name and domain.
func makeVhostName(
	ctx context.Context,
	parentName string,
	domain string,
) string {
	return utils.SanitizeForEnvoy(ctx, parentName+"~"+domain, "vHost")
}
