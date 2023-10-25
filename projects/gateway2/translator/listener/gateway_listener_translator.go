package listener

import (
	"sort"

	"github.com/solo-io/gloo/projects/gateway2/controller"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// TranslateListeners translates the set of gloo listeners required to produce a full output proxy (either form one Gateway or multiple merged Gateways)
func TranslateListeners(
	gateway *gwv1.Gateway,
	routes map[string]*controller.ListenerResult,
	reporter reports.Reporter,
) []*v1.Listener {
	validatedListeners := validateListeners(gateway.Spec.Listeners, reporter.Gateway(gateway))
	return mergeGWListeners(gateway.Namespace, validatedListeners).translateListeners(routes, reporter)
}

func mergeGWListeners(gatewayNamespace string, listeners []gwv1.Listener) *mergedListeners {
	ml := &mergedListeners{
		gatewayNamespace: gatewayNamespace,
	}
	for _, listener := range listeners {
		ml.append(listener)
	}
	return ml
}

type mergedListeners struct {
	gatewayNamespace string
	listeners        []*mergedListener
}

func (ml *mergedListeners) appendHttpListener(listener gwv1.Listener) {
	parent := httpFilterChainParent{
		gatewayListenerName: string(listener.Name),
		host:                listener.Hostname,
	}

	fc := &httpFilterChain{
		parents: []httpFilterChainParent{parent},
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

func (ml *mergedListeners) appendHttpsListener(listener gwv1.Listener) {

	// create a new filter chain for the listener
	//protocol:            listener.Protocol,
	mfc := httpsFilterChain{
		gatewayListenerName: string(listener.Name),
		host:                listener.Hostname,
		protocol:            listener.Protocol,
		tls:                 listener.TLS,
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

func (ml *mergedListeners) append(listener gwv1.Listener) {

	switch listener.Protocol {
	case gwv1.HTTPProtocolType:
		ml.appendHttpListener(listener)
	case gwv1.HTTPSProtocolType:
		ml.appendHttpsListener(listener)
		// TODO default handling
	}
}

func (ml *mergedListeners) translateListeners(routes map[string]*controller.ListenerResult,
	reporter reports.Reporter) []*v1.Listener {
	var listeners []*v1.Listener
	for _, mergedListener := range ml.listeners {
		listener := mergedListener.translateListener(routes, reporter)
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
	routes map[string]*controller.ListenerResult,
	reporter reports.Reporter,
) *v1.Listener {

	var (
		httpFilterChains []*v1.AggregateListener_HttpFilterChain
		mergedVhosts     = map[string]*v1.VirtualHost{}
	)

	if ml.httpFilterChain != nil {
		var routesForFilterChain []gwv1.HTTPRoute
		for _, hr := range routes[ml.name].Routes {
			if hr.Error != nil {
				rt := hr.Route
				routesForFilterChain = append(routesForFilterChain, rt)
			}
		}
		httpFilterChain, vhostsForFilterchain := ml.httpFilterChain.translateHttpFilterChain(
			ml.name,
			ml.gatewayNamespace,
			routesForFilterChain,
			reporter,
		)
		httpFilterChains = append(httpFilterChains, httpFilterChain)
		for vhostRef, vhost := range vhostsForFilterchain {
			if _, ok := mergedVhosts[vhostRef]; ok {
				// TODO handle internal error

			}
			mergedVhosts[vhostRef] = vhost
		}
	}
	for _, mfc := range ml.httpsFilterChains {
		var routesForFilterChain []gwv1.HTTPRoute
		for _, hr := range routes[mfc.gatewayListenerName].Routes {
			if hr.Error != nil {
				rt := hr.Route
				routesForFilterChain = append(routesForFilterChain, rt)
			}
		}
		// each virtual host name must be unique across all filter chains
		// to prevent collisions because the vhosts have to be re-computed for each set
		httpsFilterChain, vhostsForFilterchain := mfc.translateHttpsFilterChain(
			mfc.gatewayListenerName,
			ml.gatewayNamespace,
			routesForFilterChain,
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
}

type httpFilterChainParent struct {
	gatewayListenerName string
	host                *gwv1.Hostname
}

func (mfc *httpFilterChain) translateHttpFilterChain(
	parentName string,
	gatewayNamespace string,
	routes []gwv1.HTTPRoute,
	reporter reports.Reporter,
) (*v1.AggregateListener_HttpFilterChain, map[string]*v1.VirtualHost) {

	var (
		virtualHostRefs []string
		virtualHosts    = map[string]*v1.VirtualHost{}
	)
	for _, parent := range mfc.parents {
		vhostsForParent := httproute.TranslateGatewayHTTPRoutes(parentName, gatewayNamespace, parent.host, routes, reporter)

		for virtualHostRef, virtualHost := range vhostsForParent {
			virtualHostRefs = append(virtualHostRefs, virtualHostRef)
			virtualHosts[virtualHostRef] = virtualHost
		}
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
	host                *gwv1.Hostname
	protocol            gwv1.ProtocolType
	tls                 *gwv1.GatewayTLSConfig
}

func (mfc *httpsFilterChain) translateHttpsFilterChain(
	parentName string,
	gatewayNamespace string,
	routes []gwv1.HTTPRoute,
	reporter reports.Reporter,
) (*v1.AggregateListener_HttpFilterChain, map[string]*v1.VirtualHost) {

	matcher := &v1.Matcher{
		SslConfig:               translateSslConfig(mfc.tls),
		SourcePrefixRanges:      nil,
		PassthroughCipherSuites: nil,
	}
	virtualHosts := httproute.TranslateGatewayHTTPRoutes(parentName, gatewayNamespace, mfc.host, routes, reporter)

	var virtualHostRefs []string
	for _, virtualHost := range virtualHosts {
		virtualHostRef := virtualHost.GetName()
		virtualHostRefs = append(virtualHostRefs, virtualHostRef)
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
