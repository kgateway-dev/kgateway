package listener

import (
	"context"
	"errors"
	"sort"

	"github.com/solo-io/gloo/projects/gateway2/translator/sslutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway2/ports"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/translator/routeutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	corev1 "k8s.io/api/core/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// TranslateListeners translates the set of gloo listeners required to produce a full output proxy (either form one Gateway or multiple merged Gateways)
func TranslateListeners(
	ctx context.Context,
	queries query.GatewayQueries,
	plugins registry.HTTPFilterPluginRegistry,
	gateway *gwv1.Gateway,
	routesForGw query.RoutesForGwResult,
	reporter reports.Reporter,
) []*v1.Listener {
	validatedListeners := validateListeners(gateway, reporter.Gateway(gateway))

	mergedListeners := mergeGWListeners(queries, gateway.Namespace, validatedListeners, routesForGw, reporter.Gateway(gateway))
	translatedListeners := mergedListeners.translateListeners(ctx, plugins, queries, reporter)
	return translatedListeners
}

func mergeGWListeners(
	queries query.GatewayQueries,
	gatewayNamespace string,
	listeners []gwv1.Listener,
	routesForGw query.RoutesForGwResult,
	reporter reports.GatewayReporter,
) *mergedListeners {
	ml := &mergedListeners{
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
		var routes []*query.ListenerRouteResult
		if result != nil {
			routes = result.Routes
		}
		ml.append(listener, routes, listenerReporter)
	}
	return ml
}

type mergedListeners struct {
	gatewayNamespace string
	listeners        []*mergedListener
	queries          query.GatewayQueries
}

func (ml *mergedListeners) append(
	listener gwv1.Listener,
	routes []*query.ListenerRouteResult,
	reporter reports.ListenerReporter,
) error {
	switch listener.Protocol {
	case gwv1.HTTPProtocolType:
		ml.appendHttpListener(listener, routes, reporter)
	case gwv1.HTTPSProtocolType:
		ml.appendHttpsListener(listener, routes, reporter)
	// TODO default handling
	default:
		return eris.Errorf("unsupported protocol: %v", listener.Protocol)
	}

	return nil
}

func (ml *mergedListeners) appendHttpListener(
	listener gwv1.Listener,
	routesWithHosts []*query.ListenerRouteResult,
	reporter reports.ListenerReporter,
) {
	parent := httpFilterChainParent{
		gatewayListenerName: string(listener.Name),
		routes:              routesWithHosts,
	}

	fc := &httpFilterChain{
		parents: []httpFilterChainParent{parent},
		queries: ml.queries,
	}
	listenerName := string(listener.Name)
	finalPort := gwv1.PortNumber(ports.TranslatePort(uint16(listener.Port)))

	for _, lis := range ml.listeners {
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
	ml.listeners = append(ml.listeners, &mergedListener{
		name:             listenerName,
		gatewayNamespace: ml.gatewayNamespace,
		port:             finalPort,
		httpFilterChain:  fc,
		listenerReporter: reporter,
	})

}

func (ml *mergedListeners) appendHttpsListener(
	listener gwv1.Listener,
	routesWithHosts []*query.ListenerRouteResult,
	reporter reports.ListenerReporter,
) {

	// create a new filter chain for the listener
	//protocol:            listener.Protocol,
	mfc := httpsFilterChain{
		gatewayListenerName: string(listener.Name),
		sniDomain:           listener.Hostname,
		tls:                 listener.TLS,
		routesWithHosts:     routesWithHosts,
		queries:             ml.queries,
	}

	listenerName := string(listener.Name)
	for _, lis := range ml.listeners {
		if lis.port == listener.Port {
			// concatenate the names on the parent output listener
			// TODO is this valid listener name?
			lis.name += "~" + listenerName
			lis.httpsFilterChains = append(lis.httpsFilterChains, mfc)
			return
		}
	}
	ml.listeners = append(ml.listeners, &mergedListener{
		name:              listenerName,
		gatewayNamespace:  ml.gatewayNamespace,
		port:              gwv1.PortNumber(ports.TranslatePort(uint16(listener.Port))),
		httpsFilterChains: []httpsFilterChain{mfc},
		listenerReporter:  reporter,
	})
}

func (ml *mergedListeners) translateListeners(
	ctx context.Context,
	plugins registry.HTTPFilterPluginRegistry,
	queries query.GatewayQueries,
	reporter reports.Reporter) []*v1.Listener {
	var listeners []*v1.Listener
	for _, mergedListener := range ml.listeners {
		listener := mergedListener.translateListener(ctx, plugins, queries, reporter)
		listeners = append(listeners, listener)
	}
	return listeners
}

type mergedListener struct {
	name              string
	gatewayNamespace  string
	port              gwv1.PortNumber
	httpFilterChain   *httpFilterChain
	httpsFilterChains []httpsFilterChain
	listenerReporter  reports.ListenerReporter
	// TODO(policy via http listener options)
}

func (ml *mergedListener) translateListener(
	ctx context.Context,
	plugins registry.HTTPFilterPluginRegistry,
	queries query.GatewayQueries,
	reporter reports.Reporter,
) *v1.Listener {
	var (
		httpFilterChains []*v1.AggregateListener_HttpFilterChain
		mergedVhosts     = map[string]*v1.VirtualHost{}
	)

	if ml.httpFilterChain != nil {
		httpFilterChain, vhostsForFilterchain := ml.httpFilterChain.translateHttpFilterChain(
			ctx,
			ml.name,
			ml.gatewayNamespace,
			plugins,
			reporter,
		)
		httpFilterChains = append(httpFilterChains, httpFilterChain)
		for vhostRef, vhost := range vhostsForFilterchain {
			if _, ok := mergedVhosts[vhostRef]; ok {
				// TODO handle internal error, should never overlap

			}
			mergedVhosts[vhostRef] = vhost
		}
	}
	for _, mfc := range ml.httpsFilterChains {
		// each virtual host name must be unique across all filter chains
		// to prevent collisions because the vhosts have to be re-computed for each set
		httpsFilterChain, vhostsForFilterchain := mfc.translateHttpsFilterChain(
			ctx,
			plugins,
			mfc.gatewayListenerName,
			ml.gatewayNamespace,
			queries,
			reporter,
			ml.listenerReporter,
		)
		if httpsFilterChain == nil {
			// TODO report
			continue
		}
		httpFilterChains = append(httpFilterChains, httpsFilterChain)
		for vhostRef, vhost := range vhostsForFilterchain {
			if _, ok := mergedVhosts[vhostRef]; ok {
				// TODO handle internal error
			}
			mergedVhosts[vhostRef] = vhost
		}
	}

	return &v1.Listener{
		Name:        ml.name,
		BindAddress: "::",
		BindPort:    uint32(ml.port),
		ListenerType: &v1.Listener_AggregateListener{
			AggregateListener: &v1.AggregateListener{
				HttpResources: &v1.AggregateListener_HttpResources{
					VirtualHosts: mergedVhosts,
					// TODO(ilackarms): mid term - add http listener options
					HttpOptions: nil,
				},
				HttpFilterChains: httpFilterChains,
				// TODO(ilackarms): mid term - add http listener options
				TcpListeners: nil,
			},
		},
		// TODO(ilackarms): mid term - add listener options
		Options:      nil,
		RouteOptions: nil,
	}

}

// httpFilterChain each one represents a GW Listener that has been merged into a single Gloo Listener (with distinct filter chains).
// In the case where no GW Listener merging takes place, every listener will use a Gloo AggregatedListeener with 1 HTTP filter chain.
type httpFilterChain struct {
	parents []httpFilterChainParent
	queries query.GatewayQueries
}

type httpFilterChainParent struct {
	gatewayListenerName string
	routes              []*query.ListenerRouteResult
}

func (mfc *httpFilterChain) translateHttpFilterChain(
	ctx context.Context,
	parentName string,
	gatewayNamespace string,
	plugins registry.HTTPFilterPluginRegistry,
	reporter reports.Reporter,
) (*v1.AggregateListener_HttpFilterChain, map[string]*v1.VirtualHost) {

	var (
		routesByHost = map[string]routeutils.SortableRoutes{}
	)
	for _, parent := range mfc.parents {
		for _, routeWithHosts := range parent.routes {
			parentRefReporter := reporter.Route(&routeWithHosts.Route).ParentRef(&routeWithHosts.ParentRef)
			routes := httproute.TranslateGatewayHTTPRouteRules(
				ctx,
				plugins,
				mfc.queries,
				routeWithHosts.Route,
				parentRefReporter,
			)

			if len(routes) == 0 {
				// TODO report
				continue
			}

			hostnames := routeWithHosts.Hostnames
			if len(hostnames) == 0 {
				hostnames = []string{"*"}
			}

			for _, host := range hostnames {
				routesByHost[host] = append(routesByHost[host], routes...)
			}
		}
	}

	var (
		virtualHostRefs []string
		virtualHosts    = map[string]*v1.VirtualHost{}
	)
	for host, vhostRoutes := range routesByHost {
		sort.Stable(vhostRoutes)
		vhostName := makeVhostName(parentName, host)
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

// httpFilterChain each one represents a GW Listener that has been merged into a single Gloo Listener (with distinct filter chains).
// In the case where no GW Listener merging takes place, every listener will use a Gloo AggregatedListeener with 1 HTTP filter chain.
type httpsFilterChain struct {
	gatewayListenerName string
	sniDomain           *gwv1.Hostname
	tls                 *gwv1.GatewayTLSConfig
	routesWithHosts     []*query.ListenerRouteResult
	queries             query.GatewayQueries
}

func (mfc *httpsFilterChain) translateHttpsFilterChain(
	ctx context.Context,
	plugins registry.HTTPFilterPluginRegistry,
	parentName string,
	gatewayNamespace string,
	queries query.GatewayQueries,
	reporter reports.Reporter,
	listenerReporter reports.ListenerReporter,
) (*v1.AggregateListener_HttpFilterChain, map[string]*v1.VirtualHost) {
	// process routes first, so any route related errors are reported on the httproute.
	var (
		routesByHost = map[string]routeutils.SortableRoutes{}
	)
	for _, routeWithHosts := range mfc.routesWithHosts {
		parentRefReporter := reporter.Route(&routeWithHosts.Route).ParentRef(&routeWithHosts.ParentRef)
		routes := httproute.TranslateGatewayHTTPRouteRules(
			ctx,
			plugins,
			mfc.queries,
			routeWithHosts.Route,
			parentRefReporter,
		)

		if len(routes) == 0 {
			// TODO report
			continue
		}

		for _, host := range routeWithHosts.Hostnames {
			routesByHost[host] = append(routesByHost[host], routeutils.ToSortable(&routeWithHosts.Route, routes)...)
		}
	}

	sslConfig, err := translateSslConfig(
		ctx,
		gatewayNamespace,
		mfc.sniDomain,
		mfc.tls,
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

	var (
		virtualHostRefs []string
		virtualHosts    = map[string]*v1.VirtualHost{}
	)
	for host, vhostRoutes := range routesByHost {
		sort.Stable(vhostRoutes)
		vhostName := makeVhostName(parentName, host)
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
		Matcher:         matcher,
		VirtualHostRefs: virtualHostRefs,
	}, virtualHosts
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
		if err := sslutils.ValidateTlsSecret(secret.(*corev1.Secret)); err != nil {
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
	parentName string,
	domain string,
) string {
	// TODO is this a valid vh name?
	return parentName + "~" + domain
}
