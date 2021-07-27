package translator

import (
	"fmt"
	"reflect"
	"sort"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/go-utils/contextutils"
)

func (t *translatorInstance) computeListener(
	params plugins.Params,
	proxy *v1.Proxy,
	listener *v1.Listener,
	listenerReport *validationapi.ListenerReport,
) *envoy_config_listener_v3.Listener {
	params.Ctx = contextutils.WithLogger(params.Ctx, "compute_listener."+listener.Name)
	validateListenerPorts(proxy, listenerReport)
	var filterChains []*envoy_config_listener_v3.FilterChain
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

	CheckForDuplicateFilterChainMatches(filterChains, listenerReport)

	out := &envoy_config_listener_v3.Listener{
		Name: listener.Name,
		Address: &envoy_config_core_v3.Address{
			Address: &envoy_config_core_v3.Address_SocketAddress{
				SocketAddress: &envoy_config_core_v3.SocketAddress{
					Protocol: envoy_config_core_v3.SocketAddress_TCP,
					Address:  listener.BindAddress,
					PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
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

func (t *translatorInstance) computeListenerFilters(params plugins.Params, listener *v1.Listener, listenerReport *validationapi.ListenerReport) []*envoy_config_listener_v3.Filter {
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

	// Check that we don't refer to nonexistent auth config
	for i, vHost := range httpListener.HttpListener.GetVirtualHosts() {
		acRef := vHost.GetOptions().GetExtauth().GetConfigRef()
		if acRef != nil {
			if _, err := params.Snapshot.AuthConfigs.Find(acRef.GetNamespace(), acRef.GetName()); err != nil {
				validation.AppendVirtualHostError(httpListenerReport.VirtualHostReports[i], validationapi.VirtualHostReport_Error_ProcessingError, "auth config not found: "+acRef.String())
			}
		}
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
func (t *translatorInstance) computeFilterChainsFromSslConfig(
	snap *v1.ApiSnapshot,
	listener *v1.Listener,
	listenerFilters []*envoy_config_listener_v3.Filter,
	listenerReport *validationapi.ListenerReport,
) []*envoy_config_listener_v3.FilterChain {
	// if no ssl config is provided, return a single insecure filter chain
	if len(listener.SslConfigurations) == 0 {
		return []*envoy_config_listener_v3.FilterChain{{
			Filters:       listenerFilters,
			UseProxyProto: listener.GetUseProxyProto(),
		}}
	}

	var secureFilterChains []*envoy_config_listener_v3.FilterChain

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
	// we can merge ssl config if they look the same except for SNI domains.
	// combine SNI information.
	// return merged result

	var result []*v1.SslConfig

	mergedSslSecrets := map[string]*v1.SslConfig{}

	for _, sslConfig := range sslConfigs {

		// make sure ssl configs are only different by sni domains
		sslConfigCopy := proto.Clone(sslConfig).(*v1.SslConfig)
		sslConfigCopy.SniDomains = nil
		hash, _ := sslConfigCopy.Hash(nil)

		key := fmt.Sprintf("%d", hash)

		if matchingCfg, ok := mergedSslSecrets[key]; ok {
			if len(matchingCfg.SniDomains) == 0 || len(sslConfig.SniDomains) == 0 {
				// if either of the configs match on everything; then match on everything
				matchingCfg.SniDomains = nil
			} else {
				matchingCfg.SniDomains = merge(matchingCfg.SniDomains, sslConfig.SniDomains...)
			}
		} else {
			ptrToCopy := proto.Clone(sslConfig).(*v1.SslConfig)
			mergedSslSecrets[key] = ptrToCopy
			result = append(result, ptrToCopy)
		}
	}

	return result
}

func merge(values []string, newvalues ...string) []string {
	existing := map[string]bool{}
	for _, v := range values {
		existing[v] = true
	}

	for _, v := range newvalues {
		if _, ok := existing[v]; !ok {
			values = append(values, v)
		}
	}
	return values
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

func newSslFilterChain(
	downstreamConfig *envoyauth.DownstreamTlsContext,
	sniDomains []string,
	useProxyProto *wrappers.BoolValue,
	listenerFilters []*envoy_config_listener_v3.Filter,
) *envoy_config_listener_v3.FilterChain {

	// copy listenerFilter so we can modify filter chain later without changing the filters on all of them!
	listenerFiltersCopy := make([]*envoy_config_listener_v3.Filter, len(listenerFilters))
	for i, lf := range listenerFilters {
		listenerFiltersCopy[i] = proto.Clone(lf).(*envoy_config_listener_v3.Filter)
	}

	return &envoy_config_listener_v3.FilterChain{
		FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
			ServerNames: sniDomains,
		},
		Filters: listenerFiltersCopy,

		TransportSocket: &envoy_config_core_v3.TransportSocket{
			Name:       wellknown.TransportSocketTls,
			ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: utils.MustMessageToAny(downstreamConfig)},
		},

		UseProxyProto: useProxyProto,
	}
}

func sortListenerFilters(filters plugins.StagedListenerFilterList) []*envoy_config_listener_v3.Filter {
	sort.Sort(filters)
	var sortedFilters []*envoy_config_listener_v3.Filter
	for _, filter := range filters {
		sortedFilters = append(sortedFilters, filter.ListenerFilter)
	}
	return sortedFilters
}

// Check for identical FilterChains to avoid the envoy error that occurs here:
// https://github.com/envoyproxy/envoy/blob/v1.15.0/source/server/filter_chain_manager_impl.cc#L162-L166
// Note: this is NOT address non-equal but overlapping FilterChainMatches, which is a separate check here:
// https://github.com/envoyproxy/envoy/blob/50ef0945fa2c5da4bff7627c3abf41fdd3b7cffd/source/server/filter_chain_manager_impl.cc#L218-L354
// Given the complexity of the overlap detection implementation, we don't want to duplicate that behavior here.
// We may want to consider invoking envoy from a library to detect overlapping and other issues, which would build
// off this discussion: https://github.com/solo-io/gloo/issues/2114
// Visible for testing
func CheckForDuplicateFilterChainMatches(filterChains []*envoy_config_listener_v3.FilterChain, listenerReport *validationapi.ListenerReport) {
	for idx1, filterChain := range filterChains {
		for idx2, otherFilterChain := range filterChains {
			// only need to compare each pair once
			if idx2 <= idx1 {
				continue
			}
			if reflect.DeepEqual(filterChain.FilterChainMatch, otherFilterChain.FilterChainMatch) {
				validation.AppendListenerError(listenerReport,
					validationapi.ListenerReport_Error_SSLConfigError, fmt.Sprintf("Tried to apply multiple filter chains "+
						"with the same FilterChainMatch {%v}. This is usually caused by overlapping sniDomains or multiple empty sniDomains in virtual services", filterChain.FilterChainMatch))
			}
		}
	}
}
