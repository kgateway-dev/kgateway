package listener

import (
	"github.com/solo-io/gloo/projects/gateway2/controller"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
	"sort"
)

// TranslateListeners translates the set of gloo listeners required to produce a full output proxy (either form one Gateway or multiple merged Gateways)
func TranslateListeners(
	gateway *v1beta1.Gateway,
	routes map[string][]v1beta1.HTTPRoute,
	reporter reports.Reporter,
) []*v1.Listener {
	validatedListeners := validateListeners(gateway.Spec.Listeners, reporter.Gateway(gateway))
	return mergeGWListeners(validatedListeners).translateListeners(routes, reporter)
}

func mergeGWListeners(listeners []v1beta1.Listener) *mergedListeners {
	ml := &mergedListeners{}
	for _, listener := range listeners {
		ml.append(listener)
	}
	return ml
}

type mergedListeners struct {
	listeners []*mergedListener
}

func (ml *mergedListeners) appendHttpListener(listener v1beta1.Listener) {
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
		name:            listenerName,
		port:            listener.Port,
		httpFilterChain: fc,
	})

}

func (ml *mergedListeners) appendHttpsListener(listener v1beta1.Listener) {

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

func (ml *mergedListeners) append(listener v1beta1.Listener) {

	switch listener.Protocol {
	case v1beta1.HTTPProtocolType:
		ml.appendHttpListener(listener)
	case v1beta1.HTTPSProtocolType:
		ml.appendHttpsListener(listener)
		// TODO default handling
	}
}

func (ml *mergedListeners) translateListeners(routes map[string][]v1beta1.HTTPRoute,
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
	port              v1beta1.PortNumber
	httpFilterChain   *httpFilterChain
	httpsFilterChains []httpsFilterChain
	// TODO(policy via http listener options)
}

func (ml *mergedListener) translateListener(
	routes map[string][]v1beta1.HTTPRoute,
	reporter reports.Reporter,
) *v1.Listener {

	var (
		httpFilterChains []*v1.AggregateListener_HttpFilterChain
		mergedVhosts     = map[string]*v1.VirtualHost{}
	)

	for _, mfc := range ml.httpsFilterChains {
		routesForFilterChain := routes[mfc.gatewayListenerName]
		// each virtual host name must be unique across all filter chains
		// to prevent collisions because the vhosts have to be re-computed for each set
		httpFilterChain, vhostsForFilterchain := mfc.translateFilterChain(mfc.gatewayListenerName, routesForFilterChain, reporter)
		httpFilterChains = append(httpFilterChains, httpFilterChain)
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
	host                *v1beta1.Hostname
}

func (mfc *httpFilterChain) translateFilterChain(
	parentName string,
	routes []v1beta1.HTTPRoute,
	reporter reports.Reporter,
) (*v1.AggregateListener_HttpFilterChain, map[string]*v1.VirtualHost) {

	// TODO translate FC matcher
	matcher := &v1.Matcher{}
	var (
		virtualHostRefs []string
		virtualHosts    = map[string]*v1.VirtualHost{}
	)
	for _, parent := range mfc.parents {
		vhostsForParent := httproute.TranslateGatewayHTTPRoutes(parentName, parent.host, routes, reporter)

		for virtualHostRef, virtualHost := range vhostsForParent {
			virtualHostRefs = append(virtualHostRefs, virtualHostRef)
			virtualHosts[virtualHostRef] = virtualHost
		}
	}
	sort.Strings(virtualHostRefs)

	return &v1.AggregateListener_HttpFilterChain{
		Matcher:         matcher,
		VirtualHostRefs: virtualHostRefs,
	}, virtualHosts
}

// httpFilterChain each one represents a GW Listener that has been merged into a single Gloo Listener (with distinct filter chains).
// In the case where no GW Listener merging takes place, every listener will use a Gloo AggregatedListeener with 1 HTTP filter chain.
type httpsFilterChain struct {
	gatewayListenerName string
	host                *v1beta1.Hostname
	protocol            v1beta1.ProtocolType
	tls                 *v1beta1.GatewayTLSConfig
}

func (mfc *httpsFilterChain) translateFilterChain(
	parentName string,
	routes []v1beta1.HTTPRoute,
	reporter reports.Reporter,
) (*v1.AggregateListener_HttpFilterChain, map[string]*v1.VirtualHost) {

	// TODO translate FC matcher
	matcher := &v1.Matcher{
		SslConfig:               translateSslConfig(mfc.tls),
		SourcePrefixRanges:      nil,
		PassthroughCipherSuites: nil,
	}
	virtualHosts := httproute.TranslateGatewayHTTPRoutes(parentName, mfc.host, routes, reporter)

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

func translateSslConfig(tls *v1beta1.GatewayTLSConfig) *ssl.SslConfig {
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

// TODO: cross-listener validation
// return valid for translation
func validateGateway(gateway *v1beta1.Gateway, inputs controller.GatewayQueries, reporter reports.Reporter) bool {

	return true
}

// TODO: cross-listener validation
func validateListeners(listeners []v1beta1.Listener, gwReporter reports.GatewayReporter) []v1beta1.Listener {

	// gateway must contain at least 1 listener
	if len(listeners) == 0 {
		gwReporter.Err("gateway must contain at least 1 listener")
	}
	// each gateway listener must not match exactly the same traffic
	// validate - only supporting HTTP and HTTPS protocols right now

	// TODO
	// validate names
	// validate uniquenesss
	// return valid for translation
	return listeners
}
