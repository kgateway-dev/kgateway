package translator

import (
	"fmt"

	"github.com/rotisserie/eris"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/duration"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/golang/protobuf/proto"
	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
)

type FilterChainTranslator interface {
	ComputeFilterChains(params plugins.Params) []*envoy_config_listener_v3.FilterChain
}

var _ FilterChainTranslator = new(tcpFilterChainTranslator)
var _ FilterChainTranslator = new(sslDuplicatedFilterChainTranslator)
var _ FilterChainTranslator = new(matcherFilterChainTranslator)

type tcpFilterChainTranslator struct {
	// List of TcpFilterChainPlugins to process
	plugins []plugins.TcpFilterChainPlugin
	// The parent Listener, this is only used to associate errors with the parent resource
	parentListener *v1.Listener
	// The TcpListener used to generate the list of FilterChains
	listener *v1.TcpListener
	// The report used to store processing errors
	report *validationapi.TcpListenerReport
}

func (t *tcpFilterChainTranslator) ComputeFilterChains(params plugins.Params) []*envoy_config_listener_v3.FilterChain {
	var filterChains []*envoy_config_listener_v3.FilterChain

	// run the tcp filter chain plugins
	for _, plug := range t.plugins {
		pluginFilterChains, err := plug.CreateTcpFilterChains(params, t.parentListener, t.listener)
		if err != nil {
			validation.AppendTCPListenerError(t.report,
				validationapi.TcpListenerReport_Error_ProcessingError,
				fmt.Sprintf("listener %s: %s", t.parentListener.GetName(), err.Error()))
			continue
		}

		filterChains = append(filterChains, pluginFilterChains...)
	}

	return filterChains
}

// An sslDuplicatedFilterChainTranslator configures a single set of NetworkFilters
// and then creates duplicate filter chains for each provided SslConfig.
type sslDuplicatedFilterChainTranslator struct {
	parentReport            *validationapi.ListenerReport
	networkFilterTranslator NetworkFilterTranslator
	sslConfigurations       []*v1.SslConfig
	sslConfigTranslator     utils.SslConfigTranslator
}

func (s *sslDuplicatedFilterChainTranslator) ComputeFilterChains(params plugins.Params) []*envoy_config_listener_v3.FilterChain {
	// generate all the network filters
	// this includes the HttpConnectionManager, which generates all http filters
	networkFilters := s.networkFilterTranslator.ComputeNetworkFilters(params)
	if len(networkFilters) == 0 {
		return nil
	}

	return s.computeFilterChainsFromSslConfig(params.Snapshot, networkFilters)
}

// create a duplicate of the listener filter chain for each ssl cert we want to serve
// if there is no SSL config on the listener, the envoy listener will have one insecure filter chain
func (s *sslDuplicatedFilterChainTranslator) computeFilterChainsFromSslConfig(
	snap *v1snap.ApiSnapshot,
	listenerFilters []*envoy_config_listener_v3.Filter,
) []*envoy_config_listener_v3.FilterChain {
	// if no ssl config is provided, return a single insecure filter chain
	if len(s.sslConfigurations) == 0 {
		return []*envoy_config_listener_v3.FilterChain{{
			Filters: listenerFilters,
		}}
	}

	var secureFilterChains []*envoy_config_listener_v3.FilterChain

	for _, sslConfig := range s.sslConfigurations {
		// get secrets
		downstreamTlsContext, err := s.sslConfigTranslator.ResolveDownstreamSslConfig(snap.Secrets, sslConfig)
		if err != nil {
			validation.AppendListenerError(s.parentReport,
				validationapi.ListenerReport_Error_SSLConfigError, err.Error())
			continue
		}

		filterChain := newSslFilterChain(
			downstreamTlsContext,
			sslConfig.GetSniDomains(),
			listenerFilters,
			sslConfig.GetTransportSocketConnectTimeout())
		secureFilterChains = append(secureFilterChains, filterChain)
	}
	return secureFilterChains
}

func newSslFilterChain(
	downstreamTlsContext *envoyauth.DownstreamTlsContext,
	sniDomains []string,
	listenerFilters []*envoy_config_listener_v3.Filter,
	timeout *duration.Duration,
) *envoy_config_listener_v3.FilterChain {

	// copy listenerFilter so we can modify filter chain later without changing the filters on all of them!
	clonedListenerFilters := cloneListenerFilters(listenerFilters)

	return &envoy_config_listener_v3.FilterChain{
		FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
			ServerNames: sniDomains,
		},
		Filters: clonedListenerFilters,
		TransportSocket: &envoy_config_core_v3.TransportSocket{
			Name:       wellknown.TransportSocketTls,
			ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: utils.MustMessageToAny(downstreamTlsContext)},
		},
		TransportSocketConnectTimeout: timeout,
	}
}

func cloneListenerFilters(originalListenerFilters []*envoy_config_listener_v3.Filter) []*envoy_config_listener_v3.Filter {
	clonedListenerFilters := make([]*envoy_config_listener_v3.Filter, len(originalListenerFilters))
	for i, lf := range originalListenerFilters {
		clonedListenerFilters[i] = proto.Clone(lf).(*envoy_config_listener_v3.Filter)
	}

	return clonedListenerFilters
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

func merge(values []string, newValues ...string) []string {
	existingValues := make(map[string]struct{}, len(values))
	for _, v := range values {
		existingValues[v] = struct{}{}
	}

	for _, v := range newValues {
		if _, ok := existingValues[v]; !ok {
			values = append(values, v)
		}
	}
	return values
}

type matcherFilterChainTranslator struct {
	// http
	httpPlugins         []plugins.HttpFilterPlugin
	hcmPlugins          []plugins.HttpConnectionManagerPlugin
	parentReport        *validationapi.ListenerReport
	sslConfigTranslator utils.SslConfigTranslator

	// List of TcpFilterChainPlugins to process
	tcpPlugins []plugins.TcpFilterChainPlugin

	listener       *v1.HybridListener
	parentListener *v1.Listener
	report         *validationapi.HybridListenerReport
}

func (m *matcherFilterChainTranslator) ComputeFilterChains(params plugins.Params) []*envoy_config_listener_v3.FilterChain {
	var outFilterChains []*envoy_config_listener_v3.FilterChain
	for _, matchedListener := range m.listener.GetMatchedListeners() {
		switch listenerType := matchedListener.GetListenerType().(type) {
		case *v1.MatchedListener_HttpListener:
			nft := NewHttpListenerNetworkFilterTranslator(
				m.parentListener,
				listenerType.HttpListener,
				m.report.GetMatchedListenerReports()[utils.MatchedRouteConfigName(m.parentListener, matchedListener.GetMatcher())].GetHttpListenerReport(),
				m.httpPlugins,
				m.hcmPlugins,
				utils.MatchedRouteConfigName(m.parentListener, matchedListener.GetMatcher()))
			networkFilters := nft.ComputeNetworkFilters(params)
			if len(networkFilters) == 0 {
				continue
			}

			httpListenerFilterChain := m.computeFilterChainFromMatchedListener(params.Snapshot, networkFilters, matchedListener.GetMatcher())
			if httpListenerFilterChain != nil {
				outFilterChains = append(outFilterChains, httpListenerFilterChain)
			}

		case *v1.MatchedListener_TcpListener:
			sublistenerFilterChainTranslator := tcpFilterChainTranslator{
				plugins: m.tcpPlugins,

				parentListener: m.parentListener,
				listener:       matchedListener.GetTcpListener(),

				report: m.report.GetMatchedListenerReports()[utils.MatchedRouteConfigName(m.parentListener, matchedListener.GetMatcher())].GetTcpListenerReport(),
			}

			tcpFilterChains := sublistenerFilterChainTranslator.ComputeFilterChains(params)
			for _, fc := range tcpFilterChains {
				if err := m.applyMatcherToFilterChain(params.Snapshot, matchedListener.GetMatcher(), fc); err != nil {
					continue
				}

				outFilterChains = append(outFilterChains, fc)
			}
		}
	}

	return outFilterChains
}

func (m *matcherFilterChainTranslator) computeFilterChainFromMatchedListener(snap *v1snap.ApiSnapshot, listenerFilters []*envoy_config_listener_v3.Filter, matcher *v1.Matcher) *envoy_config_listener_v3.FilterChain {
	fc := &envoy_config_listener_v3.FilterChain{
		Filters:          listenerFilters,
		FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{},
	}

	if err := m.applyMatcherToFilterChain(snap, matcher, fc); err != nil {
		return nil
	}

	return fc
}

// apply a Matcher to an existing FilterChain. If unsuccessful, return an error
func (m *matcherFilterChainTranslator) applyMatcherToFilterChain(snap *v1snap.ApiSnapshot, matcher *v1.Matcher, fc *envoy_config_listener_v3.FilterChain) error {
	fcm := fc.GetFilterChainMatch()
	if fcm == nil {
		// Require that the invoking function initialize the FilterChainMatch
		return eris.New("FilterChainMatch not defined on FilterChain")
	}

	if sslConfig := matcher.GetSslConfig(); sslConfig != nil {
		// Logic derived from computeFilterChainsFromSslConfig()
		downstreamConfig, err := m.sslConfigTranslator.ResolveDownstreamSslConfig(snap.Secrets, sslConfig)
		if err != nil {
			validation.AppendListenerError(m.parentReport,
				validationapi.ListenerReport_Error_SSLConfigError, err.Error())
			return err
		}

		fcm.ServerNames = sslConfig.GetSniDomains()

		fc.TransportSocket = &envoy_config_core_v3.TransportSocket{
			Name:       wellknown.TransportSocketTls,
			ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: utils.MustMessageToAny(downstreamConfig)},
		}
		fc.TransportSocketConnectTimeout = sslConfig.GetTransportSocketConnectTimeout()
	}

	var sourcePrefixRanges []*envoy_config_core_v3.CidrRange
	for _, spr := range matcher.GetSourcePrefixRanges() {
		outSpr := &envoy_config_core_v3.CidrRange{
			AddressPrefix: spr.GetAddressPrefix(),
			PrefixLen:     spr.GetPrefixLen(),
		}
		sourcePrefixRanges = append(sourcePrefixRanges, outSpr)
	}

	fcm.SourcePrefixRanges = sourcePrefixRanges

	return nil
}
