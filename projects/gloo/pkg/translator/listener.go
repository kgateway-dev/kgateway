package translator

import (
	"sort"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/utils/contextutils"
)

func (t *translator) computeListener(params plugins.Params, proxy *v1.Proxy, listener *v1.Listener, translatorReport reportFunc) *envoyapi.Listener {
	params.Ctx = contextutils.WithLogger(params.Ctx, "compute_listener."+listener.Name)

	report := func(err error, format string, args ...interface{}) {
		translatorReport(err, "listener."+format, args...)
	}
	validateListenerPorts(proxy, report)

	listenerFilters := t.computeListenerFilters(params, listener, report)

	filterChains := computeFilterChainsFromSslConfig(params.Snapshot, listener, listenerFilters, report)

	out := &envoyapi.Listener{
		Name: listener.Name,
		Address: envoycore.Address{
			Address: &envoycore.Address_SocketAddress{
				SocketAddress: &envoycore.SocketAddress{
					Protocol: envoycore.TCP,
					Address:  listener.BindAddress,
					PortSpecifier: &envoycore.SocketAddress_PortValue{
						PortValue: listener.BindPort,
					},
					Ipv4Compat: true,
				},
			},
		},
		FilterChains: filterChains,
	}

	// run the Listener Plugins
	for _, plug := range t.plugins {
		listenerPlugin, ok := plug.(plugins.ListenerPlugin)
		if !ok {
			continue
		}
		if err := listenerPlugin.ProcessListener(params, listener, out); err != nil {
			report(err, "plugin error on listener")
		}
	}

	return out
}

func (t *translator) computeListenerFilters(params plugins.Params, listener *v1.Listener, report reportFunc) []envoylistener.Filter {
	var listenerFilters []plugins.StagedListenerFilter
	// run the Listener Filter Plugins
	for _, plug := range t.plugins {
		filterPlugin, ok := plug.(plugins.ListenerFilterPlugin)
		if !ok {
			continue
		}
		stagedFilters, err := filterPlugin.ProcessListenerFilter(params, listener)
		if err != nil {
			report(err, "listener plugin error")
		}
		for _, listenerFilter := range stagedFilters {
			listenerFilters = append(listenerFilters, listenerFilter)
		}
	}

	// add the http connection manager if listener is HTTP and has >= 1 virtual hosts
	httpListener, ok := listener.ListenerType.(*v1.Listener_HttpListener)
	if !ok || len(httpListener.HttpListener.VirtualHosts) == 0 {
		return sortListenerFilters(listenerFilters)
	}

	// add the http connection manager filter after all the InAuth Listener Filters
	rdsName := routeConfigName(listener)
	httpConnMgr := t.computeHttpConnectionManagerFilter(params, httpListener.HttpListener, rdsName, report)
	listenerFilters = append(listenerFilters, plugins.StagedListenerFilter{
		ListenerFilter: httpConnMgr,
		Stage:          plugins.PostInAuth,
	})

	// sort filters by stage
	return sortListenerFilters(listenerFilters)
}

// create a duplicate of the listener filter chain for each ssl cert we want to serve
// if there is no SSL config on the listener, the envoy listener will have one insecure filter chain
func computeFilterChainsFromSslConfig(snap *v1.ApiSnapshot, listener *v1.Listener, listenerFilters []envoylistener.Filter, report reportFunc) []envoylistener.FilterChain {
	// no filters = no filter chains
	if len(listenerFilters) == 0 {
		report(errors.Errorf("listener %v configured with 0 virtual services and 0 filters", listener.Name),
			"invalid listener")
	}

	// if no ssl config is provided, return a single insecure filter chain
	if len(listener.SslConfiguations) == 0 {
		return []envoylistener.FilterChain{{
			Filters: listenerFilters,
		}}
	}

	var secureFilterChains []envoylistener.FilterChain

	sslCfgTranslator := utils.NewSslConfigTranslator(snap.Secrets.List())
	for _, sslConfig := range listener.SslConfiguations {
		// get secrets
		downstreamConfig, err := sslCfgTranslator.ResolveDownstreamSslConfig(sslConfig)
		if err != nil {
			report(err, "invalid secrets for listener %v", listener.Name)
			continue
		}
		filterChain := newSslFilterChain(downstreamConfig, sslConfig.SniDomains, listenerFilters)
		secureFilterChains = append(secureFilterChains, filterChain)
	}
	return secureFilterChains
}

func validateListenerPorts(proxy *v1.Proxy, report reportFunc) {
	listenersByPort := make(map[uint32][]string)
	for _, listener := range proxy.Listeners {
		listenersByPort[listener.BindPort] = append(listenersByPort[listener.BindPort], listener.Name)
	}
	for port, listeners := range listenersByPort {
		if len(listeners) == 1 {
			continue
		}
		report(errors.Errorf("port %v is shared by listeners %v", port, listeners), "invalid listener config")
	}
}

func newSslFilterChain(downstreamConfig *envoyauth.DownstreamTlsContext, sniDomains []string, listenerFilters []envoylistener.Filter) envoylistener.FilterChain {

	return envoylistener.FilterChain{
		FilterChainMatch: &envoylistener.FilterChainMatch{
			ServerNames: sniDomains,
		},
		Filters:    listenerFilters,
		TlsContext: downstreamConfig,
	}
}

func sortListenerFilters(filters []plugins.StagedListenerFilter) []envoylistener.Filter {
	// sort them first by stage, then by name.
	less := func(i, j int) bool {
		filteri := filters[i]
		filterj := filters[j]
		if filteri.Stage != filterj.Stage {
			return filteri.Stage < filterj.Stage
		}
		return filteri.ListenerFilter.Name < filterj.ListenerFilter.Name
	}
	sort.SliceStable(filters, less)

	var sortedFilters []envoylistener.Filter
	for _, filter := range filters {
		sortedFilters = append(sortedFilters, filter.ListenerFilter)
	}

	return sortedFilters
}
