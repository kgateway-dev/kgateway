package translator

import (
	"fmt"
	"sort"
	"strings"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/gogo/protobuf/types"
	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/go-utils/contextutils"
)

func (t *translator) computeListener(params plugins.Params, proxy *v1.Proxy, listener *v1.Listener, listenerReport *validationapi.ListenerReport) *envoyapi.Listener {
	params.Ctx = contextutils.WithLogger(params.Ctx, "compute_listener."+listener.Name)

	validateListenerPorts(proxy, listenerReport)
	var filterChains []envoylistener.FilterChain
	switch listener.GetListenerType().(type) {
	case *v1.Listener_HttpListener:
		// run the http filter chain plugins and listener plugins
		listenerFilters := t.computeListenerFilters(params, listener, listenerReport)
		if len(listenerFilters) == 0 {
			return nil
		}
		filterChains = t.computeFilterChainsFromSslConfig(params.Snapshot, listener, listenerFilters, listenerReport)
	case *v1.Listener_TcpListener:
		// run the tcp filter chain plugins
		for _, plug := range t.plugins {
			listenerPlugin, ok := plug.(plugins.ListenerFilterChainPlugin)
			if !ok {
				continue
			}
			result, err := listenerPlugin.ProcessListenerFilterChain(params, listener)
			if err != nil {
				validation.AppendListenerError(listenerReport,
					validationapi.ListenerReport_Error_ProcessingError,
					err.Error())
				continue
			}
			filterChains = append(filterChains, result...)
		}
	}

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
			validation.AppendListenerError(listenerReport,
				validationapi.ListenerReport_Error_ProcessingError,
				err.Error())
		}
	}

	return out
}

func (t *translator) computeListenerFilters(params plugins.Params, listener *v1.Listener, listenerReport *validationapi.ListenerReport) []envoylistener.Filter {
	var listenerFilters []plugins.StagedListenerFilter
	// run the Listener Filter Plugins
	for _, plug := range t.plugins {
		filterPlugin, ok := plug.(plugins.ListenerFilterPlugin)
		if !ok {
			continue
		}
		stagedFilters, err := filterPlugin.ProcessListenerFilter(params, listener)
		if err != nil {
			validation.AppendListenerError(listenerReport,
				validationapi.ListenerReport_Error_ProcessingError,
				err.Error())
		}
		for _, listenerFilter := range stagedFilters {
			listenerFilters = append(listenerFilters, listenerFilter)
		}
	}

	// return if listener type != http || no virtual hosts
	httpListener, ok := listener.ListenerType.(*v1.Listener_HttpListener)
	if !ok || len(httpListener.HttpListener.VirtualHosts) == 0 {
		return nil
	}

	httpListenerReport := listenerReport.GetHttpListenerReport()
	if httpListenerReport == nil {
		contextutils.LoggerFrom(params.Ctx).DPanic("internal error: listener report was not http type")
	}

	// add the http connection manager filter after all the InAuth Listener Filters
	rdsName := routeConfigName(listener)
	httpConnMgr := t.computeHttpConnectionManagerFilter(params, httpListener.HttpListener, rdsName, httpListenerReport)
	listenerFilters = append(listenerFilters, plugins.StagedListenerFilter{
		ListenerFilter: httpConnMgr,
		Stage:          plugins.AfterStage(plugins.AuthZStage),
	})

	return sortListenerFilters(listenerFilters)
}

// create a duplicate of the listener filter chain for each ssl cert we want to serve
// if there is no SSL config on the listener, the envoy listener will have one insecure filter chain
func (t *translator) computeFilterChainsFromSslConfig(snap *v1.ApiSnapshot, listener *v1.Listener, listenerFilters []envoylistener.Filter, listenerReport *validationapi.ListenerReport) []envoylistener.FilterChain {

	// if no ssl config is provided, return a single insecure filter chain
	if len(listener.SslConfigurations) == 0 {
		return []envoylistener.FilterChain{{
			Filters:       listenerFilters,
			UseProxyProto: listener.UseProxyProto,
		}}
	}

	var secureFilterChains []envoylistener.FilterChain

	for _, sslConfig := range mergeSslConfigs(listener.SslConfigurations) {
		// get secrets
		downstreamConfig, err := t.sslConfigTranslator.ResolveDownstreamSslConfig(snap.Secrets, sslConfig)
		if err != nil {
			validation.AppendListenerError(listenerReport,
				validationapi.ListenerReport_Error_SSLConfigError, err.Error())
			continue
		}
		filterChain := newSslFilterChain(downstreamConfig, sslConfig.SniDomains, listener.UseProxyProto, listenerFilters)
		secureFilterChains = append(secureFilterChains, filterChain)
	}
	return secureFilterChains
}

func mergeSslConfigs(sslConfigs []*v1.SslConfig) []*v1.SslConfig {
	// we can merge ssl config if:
	// they have the same SslSecrets and VerifySubjectAltName
	// combine SNI information.
	// return merged result

	var result []*v1.SslConfig

	mergedSslSecrets := map[string]*v1.SslConfig{}

	for _, sslConfig := range sslConfigs {

		key := ""
		switch sslCfg := sslConfig.SslSecrets.(type) {
		case *v1.SslConfig_SecretRef:
			key = sslCfg.SecretRef.GetName() + "," + sslCfg.SecretRef.GetName()
		case *v1.SslConfig_SslFiles:
			key = sslCfg.SslFiles.GetTlsCert() + "," + sslCfg.SslFiles.GetTlsKey() + "," + sslCfg.SslFiles.GetRootCa()
		default:
			result = append(result, sslConfig)
			continue
		}

		tmp := make([]string, len(sslConfig.VerifySubjectAltName))
		copy(tmp, sslConfig.VerifySubjectAltName)
		sort.Strings(tmp)
		key = key + ";" + strings.Join(tmp, ",")

		if matchingCfg, ok := mergedSslSecrets[key]; ok {
			matchingCfg.SslSecrets = sslConfig.SslSecrets
			matchingCfg.VerifySubjectAltName = sslConfig.VerifySubjectAltName
			if len(matchingCfg.SniDomains) == 0 || len(sslConfig.SniDomains) == 0 {
				// if either of the configs match on everything; then match on everything
				matchingCfg.SniDomains = nil
			} else {
				matchingCfg.SniDomains = append(mergedSslSecrets[key].SniDomains, sslConfig.SniDomains...)
			}
		} else {
			cfgCopy := *sslConfig
			ptrToCopy := &cfgCopy
			mergedSslSecrets[key] = ptrToCopy
			result = append(result, ptrToCopy)
		}
	}

	return result
}

func validateListenerPorts(proxy *v1.Proxy, listenerReport *validationapi.ListenerReport) {
	listenersByPort := make(map[uint32][]int)
	for i, listener := range proxy.Listeners {
		listenersByPort[listener.BindPort] = append(listenersByPort[listener.BindPort], i)
	}
	for port, listeners := range listenersByPort {
		if len(listeners) == 1 {
			continue
		}
		var listenerNames []string
		for _, idx := range listeners {
			listenerNames = append(listenerNames, proxy.Listeners[idx].Name)
		}
		validation.AppendListenerError(listenerReport,
			validationapi.ListenerReport_Error_BindPortNotUniqueError,
			fmt.Sprintf("port %v is shared by listeners %v", port, listeners),
		)
	}
}

func newSslFilterChain(downstreamConfig *envoyauth.DownstreamTlsContext, sniDomains []string, useProxyProto *types.BoolValue, listenerFilters []envoylistener.Filter) envoylistener.FilterChain {

	return envoylistener.FilterChain{
		FilterChainMatch: &envoylistener.FilterChainMatch{
			ServerNames: sniDomains,
		},
		Filters:       listenerFilters,
		TlsContext:    downstreamConfig,
		UseProxyProto: useProxyProto,
	}
}

func sortListenerFilters(filters plugins.StagedListenerFilterList) []envoylistener.Filter {
	sort.Sort(filters)
	var sortedFilters []envoylistener.Filter
	for _, filter := range filters {
		sortedFilters = append(sortedFilters, filter.ListenerFilter)
	}
	return sortedFilters
}
