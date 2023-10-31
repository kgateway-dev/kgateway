package listener

import (
	"sort"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute"
	"github.com/solo-io/gloo/projects/gateway2/translator/routeutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// TranslateListeners translates the set of gloo listeners required to produce a full output proxy (either form one Gateway or multiple merged Gateways)
func TranslateListeners(
	queries query.GatewayQueries,
	gateway *gwv1.Gateway,
	routesForGw query.RoutesForGwResult,
	reporter reports.Reporter,
) []*v1.Listener {
	validatedListeners := validateListeners(gateway, reporter.Gateway(gateway))

	return mergeGWListeners(queries, gateway.Namespace, validatedListeners, routesForGw, reporter).translateListeners(reporter)
}

func mergeGWListeners(
	queries query.GatewayQueries,
	gatewayNamespace string,
	listeners []gwv1.Listener,
	routesForGw query.RoutesForGwResult,
	reporter reports.Reporter,
) *mergedListeners {
	ml := &mergedListeners{
		gatewayNamespace: gatewayNamespace,
		queries:          queries,
	}
	for _, listener := range listeners {
		result, ok := routesForGw.ListenerResults[string(listener.Name)]
		if !ok || result.Error != nil {
			// TODO report
			continue
		}
		ml.append(listener, result.Routes, reporter)
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
	reporter reports.Reporter,
) error {

	var routesWithHosts []routeWithChildHosts
	for _, routeResult := range routes {
		routesWithHosts = append(routesWithHosts, routeWithChildHosts{
			childHosts: routeResult.Hostnames,
			route:      routeResult.Route,
		})
	}

	switch listener.Protocol {
	case gwv1.HTTPProtocolType:
		ml.appendHttpListener(listener, routesWithHosts)
	case gwv1.HTTPSProtocolType:
		ml.appendHttpsListener(listener, routesWithHosts)
	// TODO default handling
	default:
		return eris.Errorf("unsupported protocol: %v", listener.Protocol)
	}

	return nil
}

func (ml *mergedListeners) appendHttpListener(
	listener gwv1.Listener,
	routesWithHosts []routeWithChildHosts,
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
	for _, lis := range ml.listeners {
		if lis.port == listener.Port {
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
		port:             listener.Port,
		httpFilterChain:  fc,
	})

}

func (ml *mergedListeners) appendHttpsListener(
	listener gwv1.Listener,
	routesWithHosts []routeWithChildHosts,
) {

	// create a new filter chain for the listener
	//protocol:            listener.Protocol,
	mfc := httpsFilterChain{
		gatewayListenerName: string(listener.Name),
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
		port:              listener.Port,
		httpsFilterChains: []httpsFilterChain{mfc},
	})
}

func (ml *mergedListeners) translateListeners(reporter reports.Reporter) []*v1.Listener {
	var listeners []*v1.Listener
	for _, mergedListener := range ml.listeners {
		listener := mergedListener.translateListener(reporter)
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
	// TODO(policy via http listener options)
}

func (ml *mergedListener) translateListener(
	reporter reports.Reporter,
) *v1.Listener {

	var (
		httpFilterChains []*v1.AggregateListener_HttpFilterChain
		mergedVhosts     = map[string]*v1.VirtualHost{}
	)

	if ml.httpFilterChain != nil {

		httpFilterChain, vhostsForFilterchain := ml.httpFilterChain.translateHttpFilterChain(
			ml.name,
			ml.gatewayNamespace,
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
			mfc.gatewayListenerName,
			ml.gatewayNamespace,
			reporter,
		)
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
	routes              []routeWithChildHosts
}

type routeWithChildHosts struct {
	// the resolved child hosts to translate for the route table
	childHosts []string
	// the route to translate
	route gwv1.HTTPRoute
}

func (mfc *httpFilterChain) translateHttpFilterChain(
	parentName string,
	gatewayNamespace string,
	reporter reports.Reporter,
) (*v1.AggregateListener_HttpFilterChain, map[string]*v1.VirtualHost) {

	var (
		routesByHost = map[string]routeutils.SortableRoutes{}
	)
	for _, parent := range mfc.parents {
		for _, routeWithHosts := range parent.routes {
			routes := httproute.TranslateGatewayHTTPRouteRules(mfc.queries, gatewayNamespace, routeWithHosts.route, reporter)

			if len(routes) == 0 {
				// TODO report
				continue
			}

			for _, host := range routeWithHosts.childHosts {
				routesByHost[host] = append(routesByHost[host], routeutils.ToSortable(&routeWithHosts.route, routes)...)
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
	tls                 *gwv1.GatewayTLSConfig
	routesWithHosts     []routeWithChildHosts
	queries             query.GatewayQueries
}

func (mfc *httpsFilterChain) translateHttpsFilterChain(
	parentName string,
	gatewayNamespace string,
	reporter reports.Reporter,
) (*v1.AggregateListener_HttpFilterChain, map[string]*v1.VirtualHost) {

	matcher := &v1.Matcher{
		SslConfig:               translateSslConfig(mfc.tls),
		SourcePrefixRanges:      nil,
		PassthroughCipherSuites: nil,
	}

	var (
		routesByHost = map[string]routeutils.SortableRoutes{}
	)
	for _, routeWithHosts := range mfc.routesWithHosts {
		routes := httproute.TranslateGatewayHTTPRouteRules(mfc.queries, gatewayNamespace, routeWithHosts.route, reporter)

		if len(routes) == 0 {
			// TODO report
			continue
		}

		for _, host := range routeWithHosts.childHosts {
			routesByHost[host] = append(routesByHost[host], routeutils.ToSortable(&routeWithHosts.route, routes)...)
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
		Matcher:         matcher,
		VirtualHostRefs: virtualHostRefs,
	}, virtualHosts
}

// TODO ssl config
func translateSslConfig(tls *gwv1.GatewayTLSConfig) *ssl.SslConfig {
	// TODO validate ssl config

	return &ssl.SslConfig{
		SslSecrets:                    nil,
		SniDomains:                    nil,
		VerifySubjectAltName:          nil,
		Parameters:                    nil,
		AlpnProtocols:                 nil,
		OneWayTls:                     nil,
		DisableTlsSessionResumption:   nil,
		TransportSocketConnectTimeout: nil,
		OcspStaplePolicy:              0,
	}
}

// makeVhostName computes the name of a virtual host based on the parent name and domain.
func makeVhostName(
	parentName string,
	domain string,
) string {
	// TODO is this a valid vh name?
	return parentName + "~" + domain
}
