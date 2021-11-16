package translator

import (
	"fmt"
	"sort"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/duration"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/golang/protobuf/proto"
	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	sslutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
)

type FilterChainTranslator interface {
	ComputeFilterChains(params plugins.Params) []*envoy_config_listener_v3.FilterChain
}

var _ FilterChainTranslator = new(tcpFilterChainTranslator)
var _ FilterChainTranslator = new(httpFilterChainTranslator)

type tcpFilterChainTranslator struct {
	plugins []plugins.Plugin

	parentListener *v1.Listener
	listener       *v1.TcpListener

	report *validationapi.TcpListenerReport
}

func (t *tcpFilterChainTranslator) ComputeFilterChains(params plugins.Params) []*envoy_config_listener_v3.FilterChain {
	var filterChains []*envoy_config_listener_v3.FilterChain

	// run the tcp filter chain plugins
	for _, plug := range t.plugins {
		listenerPlugin, ok := plug.(plugins.ListenerFilterChainPlugin)
		if !ok {
			continue
		}
		result, err := listenerPlugin.ProcessListenerFilterChain(params, t.parentListener)
		if err != nil {
			validation.AppendTCPListenerError(t.report,
				validationapi.TcpListenerReport_Error_ProcessingError,
				err.Error())
			continue
		}
		filterChains = append(filterChains, result...)
	}

	return filterChains
}

type httpFilterChainTranslator struct {
	plugins             []plugins.Plugin
	sslConfigTranslator sslutils.SslConfigTranslator

	parentListener *v1.Listener
	listener       *v1.HttpListener

	parentReport *validationapi.ListenerReport
	report       *validationapi.HttpListenerReport

	routeConfigName string
}

func (h *httpFilterChainTranslator) ComputeFilterChains(params plugins.Params) []*envoy_config_listener_v3.FilterChain {
	// run the http filter chain plugins and listener plugins
	listenerFilters := h.computeNetworkFilters(params)
	if len(listenerFilters) == 0 {
		return nil
	}

	return h.computeFilterChainsFromSslConfig(params.Snapshot, listenerFilters)
}

func (h *httpFilterChainTranslator) computeNetworkFilters(params plugins.Params) []*envoy_config_listener_v3.Filter {
	// return if listener has no virtual hosts
	if len(h.listener.GetVirtualHosts()) == 0 {
		return nil
	}

	var listenerFilters []plugins.StagedListenerFilter
	// run the Listener Filter Plugins
	for _, plug := range h.plugins {
		filterPlugin, ok := plug.(plugins.ListenerFilterPlugin)
		if !ok {
			continue
		}
		stagedFilters, err := filterPlugin.ProcessListenerFilter(params, h.parentListener)
		if err != nil {
			validation.AppendListenerError(h.parentReport,
				validationapi.ListenerReport_Error_ProcessingError,
				err.Error())
		}
		for _, listenerFilter := range stagedFilters {
			listenerFilters = append(listenerFilters, listenerFilter)
		}
	}

	// Check that we don't refer to nonexistent auth config
	// TODO (sam-heilbron)
	// This is a partial duplicate of the open source ExtauthTranslatorSyncer
	// We should find a single place to define this configuration
	for i, vHost := range h.listener.GetVirtualHosts() {
		acRef := vHost.GetOptions().GetExtauth().GetConfigRef()
		if acRef != nil {
			if _, err := params.Snapshot.AuthConfigs.Find(acRef.GetNamespace(), acRef.GetName()); err != nil {
				validation.AppendVirtualHostError(
					h.report.GetVirtualHostReports()[i],
					validationapi.VirtualHostReport_Error_ProcessingError,
					"auth config not found: "+acRef.String())
			}
		}
	}

	// add the http connection manager filter after all the InAuth Listener Filters
	httpConnMgr := h.computeHttpConnectionManagerFilter(params)
	listenerFilters = append(listenerFilters, plugins.StagedListenerFilter{
		ListenerFilter: httpConnMgr,
		Stage:          plugins.AfterStage(plugins.AuthZStage),
	})

	return sortListenerFilters(listenerFilters)
}

// create a duplicate of the listener filter chain for each ssl cert we want to serve
// if there is no SSL config on the listener, the envoy listener will have one insecure filter chain
func (h *httpFilterChainTranslator) computeFilterChainsFromSslConfig(
	snap *v1.ApiSnapshot,
	listenerFilters []*envoy_config_listener_v3.Filter,
) []*envoy_config_listener_v3.FilterChain {
	// if no ssl config is provided, return a single insecure filter chain
	sslConfigurations := h.parentListener.GetSslConfigurations()

	if len(sslConfigurations) == 0 {
		return []*envoy_config_listener_v3.FilterChain{{
			Filters: listenerFilters,
		}}
	}

	var secureFilterChains []*envoy_config_listener_v3.FilterChain

	for _, sslConfig := range mergeSslConfigs(sslConfigurations) {
		// get secrets
		downstreamConfig, err := h.sslConfigTranslator.ResolveDownstreamSslConfig(snap.Secrets, sslConfig)
		if err != nil {
			validation.AppendListenerError(h.parentReport,
				validationapi.ListenerReport_Error_SSLConfigError, err.Error())
			continue
		}
		filterChain := newSslFilterChain(downstreamConfig, sslConfig.GetSniDomains(), listenerFilters, sslConfig.GetTransportSocketConnectTimeout())

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
			if len(matchingCfg.GetSniDomains()) == 0 || len(sslConfig.GetSniDomains()) == 0 {
				// if either of the configs match on everything; then match on everything
				matchingCfg.SniDomains = nil
			} else {
				matchingCfg.SniDomains = merge(matchingCfg.GetSniDomains(), sslConfig.GetSniDomains()...)
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

func newSslFilterChain(
	downstreamConfig *envoyauth.DownstreamTlsContext,
	sniDomains []string,
	listenerFilters []*envoy_config_listener_v3.Filter,
	timeout *duration.Duration,
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
			ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: sslutils.MustMessageToAny(downstreamConfig)},
		},
		TransportSocketConnectTimeout: timeout,
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
