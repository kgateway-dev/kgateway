package irtranslator

import (
	"context"
	"fmt"
	"sort"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoy_tls_inspector "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3"
	"google.golang.org/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	sdkreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

const (
	DefaultHttpStatPrefix  = "http"
	UpstreamCodeFilterName = "envoy.filters.http.upstream_codec"
)

type filterChainTranslator struct {
	listener        ir.ListenerIR
	gateway         ir.GatewayIR
	routeConfigName string
	reporter        sdkreporter.Reporter

	PluginPass TranslationPassPlugins
}

func computeListenerAddress(bindAddress string, port uint32, reporter sdkreporter.GatewayReporter) *envoycorev3.Address {
	_, isIpv4Address, err := utils.IsIpv4Address(bindAddress)
	if err != nil {
		// TODO: return error ????
		reporter.SetCondition(sdkreporter.GatewayCondition{
			Type:    gwv1.GatewayConditionProgrammed,
			Reason:  gwv1.GatewayReasonInvalid,
			Status:  metav1.ConditionFalse,
			Message: "Error processing listener: " + err.Error(),
		})
	}

	return &envoycorev3.Address{
		Address: &envoycorev3.Address_SocketAddress{
			SocketAddress: &envoycorev3.SocketAddress{
				Protocol: envoycorev3.SocketAddress_TCP,
				Address:  bindAddress,
				PortSpecifier: &envoycorev3.SocketAddress_PortValue{
					PortValue: port,
				},
				// As of Envoy 1.22: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.22/v1.22.0.html
				// the Ipv4Compat flag can only be set on Ipv6 address and Ipv4-mapped Ipv6 address.
				// Check if this is a non-padded pure ipv4 address and unset the compat flag if so.
				Ipv4Compat: !isIpv4Address,
			},
		},
	}
}

func tlsInspectorFilter() *envoylistenerv3.ListenerFilter {
	configEnvoy := &envoy_tls_inspector.TlsInspector{}
	msg, _ := utils.MessageToAny(configEnvoy)
	return &envoylistenerv3.ListenerFilter{
		Name: wellknown.TlsInspector,
		ConfigType: &envoylistenerv3.ListenerFilter_TypedConfig{
			TypedConfig: msg,
		},
	}
}

func (h *filterChainTranslator) initFilterChain(ctx context.Context, fcc ir.FilterChainCommon, reporter sdkreporter.ListenerReporter) *envoylistenerv3.FilterChain {
	info := &FilterChainInfo{
		Match: fcc.Matcher,
		TLS:   fcc.TLS,
	}

	fc := &envoylistenerv3.FilterChain{
		Name:             fcc.FilterChainName,
		FilterChainMatch: info.toMatch(),
		TransportSocket:  info.toTransportSocket(),
	}

	return fc
}

func (h *filterChainTranslator) computeHttpFilters(ctx context.Context, l ir.HttpFilterChainIR, reporter sdkreporter.ListenerReporter) []*envoylistenerv3.Filter {
	// 1. Generate all the network filters (including the HttpConnectionManager)
	networkFilters, err := h.computeNetworkFiltersForHttp(ctx, l, reporter)
	if err != nil {
		logger.Error("error computing network filters", "error", err)
		// TODO: report? return error?
		return nil
	}
	if len(networkFilters) == 0 {
		return nil
	}

	return networkFilters
}

func (n *filterChainTranslator) computeNetworkFiltersForHttp(ctx context.Context, l ir.HttpFilterChainIR, listenerReporter sdkreporter.ListenerReporter) ([]*envoylistenerv3.Filter, error) {
	hcm := hcmNetworkFilterTranslator{
		routeConfigName:   n.routeConfigName,
		PluginPass:        n.PluginPass,
		listenerReporter:  listenerReporter,
		reporter:          n.reporter,
		gateway:           n.gateway, // corresponds to Gateway API listener
		policyAncestorRef: n.listener.PolicyAncestorRef,
	}
	networkFilters := sortNetworkFilters(n.computeCustomFilters(ctx, l.CustomNetworkFilters, listenerReporter))
	networkFilter, err := hcm.computeNetworkFilters(ctx, l)
	if err != nil {
		return nil, err
	}
	networkFilters = append(networkFilters, networkFilter)
	return networkFilters, nil
}

// computeCustomFilters computes all custom filters, first from plugins, second
// from embedded filters on the FilterChain itself.
// For HTTP FilterChains these must be added before HCM.
func (n *filterChainTranslator) computeCustomFilters(
	ctx context.Context,
	customNetworkFilters []ir.CustomEnvoyFilter,
	listenerReporter sdkreporter.ListenerReporter,
) []plugins.StagedNetworkFilter {
	var networkFilters []plugins.StagedNetworkFilter
	// Process the network filters.
	for _, plug := range n.PluginPass {
		stagedFilters, err := plug.NetworkFilters(ctx)
		if err != nil {
			listenerReporter.SetCondition(sdkreporter.ListenerCondition{
				Type:    gwv1.ListenerConditionProgrammed,
				Reason:  gwv1.ListenerReasonInvalid,
				Status:  metav1.ConditionFalse,
				Message: "Error processing network plugin: " + err.Error(),
			})
			// TODO: return error?
		}

		for _, nf := range stagedFilters {
			if nf.Filter == nil {
				continue
			}
			networkFilters = append(networkFilters, nf)
		}
	}
	networkFilters = append(networkFilters, convertCustomNetworkFilters(customNetworkFilters)...)
	return networkFilters
}

func convertCustomNetworkFilters(customNetworkFilters []ir.CustomEnvoyFilter) []plugins.StagedNetworkFilter {
	var out []plugins.StagedNetworkFilter
	for _, customFilter := range customNetworkFilters {
		out = append(out, plugins.StagedNetworkFilter{
			Filter: &envoylistenerv3.Filter{
				Name: customFilter.Name,
				ConfigType: &envoylistenerv3.Filter_TypedConfig{
					TypedConfig: customFilter.Config,
				},
			},
			Stage: customFilter.FilterStage,
		})
	}
	return out
}

func sortNetworkFilters(filters plugins.StagedNetworkFilterList) []*envoylistenerv3.Filter {
	sort.Sort(filters)
	var sortedFilters []*envoylistenerv3.Filter
	for _, filter := range filters {
		sortedFilters = append(sortedFilters, filter.Filter)
	}
	return sortedFilters
}

type hcmNetworkFilterTranslator struct {
	routeConfigName   string
	PluginPass        TranslationPassPlugins
	listenerReporter  sdkreporter.ListenerReporter
	reporter          sdkreporter.Reporter
	listener          ir.HttpFilterChainIR // policies attached to listener
	gateway           ir.GatewayIR         // policies attached to gateway
	policyAncestorRef gwv1.ParentReference
}

func (h *hcmNetworkFilterTranslator) computeNetworkFilters(ctx context.Context, l ir.HttpFilterChainIR) (*envoylistenerv3.Filter, error) {
	// 1. Initialize the HttpConnectionManager (HCM)
	httpConnectionManager := h.initializeHCM()

	// 2. Apply HttpFilters
	var err error
	httpConnectionManager.HttpFilters = h.computeHttpFilters(ctx, l)

	pass := h.PluginPass

	// 3. Allow any HCM plugins to make their changes, with respect to any changes the core plugin made
	var attachedPolicies ir.AttachedPolicies
	// Listener policies take precedence over gateway policies, so they are ordered first
	attachedPolicies.Append(l.AttachedPolicies, h.gateway.AttachedHttpPolicies)
	for _, gk := range attachedPolicies.ApplyOrderedGroupKinds() {
		pols := attachedPolicies.Policies[gk]
		pass := pass[gk]
		if pass == nil {
			// TODO: report user error - they attached a non http policy
			continue
		}
		reportPolicyAcceptanceStatus(h.reporter, h.policyAncestorRef, pols...)
		policies, mergeOrigins := mergePolicies(pass, pols)
		for _, pol := range policies {
			pctx := &ir.HcmContext{
				Policy:  pol.PolicyIr,
				Gateway: h.gateway,
			}
			if err := pass.ApplyHCM(ctx, pctx, httpConnectionManager); err != nil {
				h.listenerReporter.SetCondition(sdkreporter.ListenerCondition{
					Type:    gwv1.ListenerConditionProgrammed,
					Reason:  gwv1.ListenerReasonInvalid,
					Status:  metav1.ConditionFalse,
					Message: "Error processing HCM plugin: " + err.Error(),
				})
			}
		}
		reportPolicyAttachmentStatus(h.reporter, h.policyAncestorRef, mergeOrigins, pols...)
	}

	// TODO: should we enable websockets by default?

	// 4. Generate the typedConfig for the HCM
	hcmFilter, err := NewFilterWithTypedConfig(wellknown.HTTPConnectionManager, httpConnectionManager)
	if err != nil {
		logger.Error("failed to convert proto message to any", "error", err)
		return nil, fmt.Errorf("failed to convert proto message to any: %w", err)
	}

	return hcmFilter, nil
}

func (h *hcmNetworkFilterTranslator) initializeHCM() *envoyhttp.HttpConnectionManager {
	statPrefix := h.listener.FilterChainName
	if statPrefix == "" {
		statPrefix = DefaultHttpStatPrefix
	}

	return &envoyhttp.HttpConnectionManager{
		CodecType:        envoyhttp.HttpConnectionManager_AUTO,
		StatPrefix:       statPrefix,
		NormalizePath:    wrapperspb.Bool(true),
		MergeSlashes:     true,
		UseRemoteAddress: wrapperspb.Bool(true),
		RouteSpecifier: &envoyhttp.HttpConnectionManager_Rds{
			Rds: &envoyhttp.Rds{
				ConfigSource: &envoycorev3.ConfigSource{
					ResourceApiVersion: envoycorev3.ApiVersion_V3,
					ConfigSourceSpecifier: &envoycorev3.ConfigSource_Ads{
						Ads: &envoycorev3.AggregatedConfigSource{},
					},
				},
				RouteConfigName: h.routeConfigName,
			},
		},
	}
}

func (h *hcmNetworkFilterTranslator) computeHttpFilters(ctx context.Context, l ir.HttpFilterChainIR) []*envoyhttp.HttpFilter {
	var httpFilters plugins.StagedHttpFilterList

	// run the HttpFilter Plugins
	for _, plug := range h.PluginPass {
		stagedFilters, err := plug.HttpFilters(ctx, l.FilterChainCommon)
		if err != nil {
			// what to do with errors here? ignore the listener??
			h.listenerReporter.SetCondition(sdkreporter.ListenerCondition{
				Type:    gwv1.ListenerConditionProgrammed,
				Reason:  gwv1.ListenerReasonInvalid,
				Status:  metav1.ConditionFalse,
				Message: "Error processing http plugin: " + err.Error(),
			})
		}

		for _, httpFilter := range stagedFilters {
			if httpFilter.Filter == nil {
				logger.Warn("got nil Filter from HttpFilters()", "plugin", plug.Name)
				continue
			}
			httpFilters = append(httpFilters, httpFilter)
		}
	}
	httpFilters = append(httpFilters, convertCustomHttpFilters(l.CustomHTTPFilters)...)

	// https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/http/http_filters#filter-ordering
	// HttpFilter ordering determines the order in which the HCM will execute the filter.

	// 1. Sort filters by stage
	// "Stage" is the type we use to specify when a filter should be run
	envoyHttpFilters := sortHttpFilters(httpFilters)

	// 2. Configure the router filter
	// As outlined by the Envoy docs, the last configured filter has to be a terminal filter.
	// We set the Router filter (https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/router_filter#config-http-filters-router)
	// as the terminal filter in kgateway.
	routerV3 := routerv3.Router{}

	//	// TODO it would be ideal of SuppressEnvoyHeaders and DynamicStats could be moved out of here set
	//	// in a separate router plugin
	//	if h.listener.GetOptions().GetRouter().GetSuppressEnvoyHeaders().GetValue() {
	//		routerV3.SuppressEnvoyHeaders = true
	//	}
	//
	//	routerV3.DynamicStats = h.listener.GetOptions().GetRouter().GetDynamicStats()

	newStagedFilter, err := plugins.NewStagedFilter(
		wellknown.Router,
		&routerV3,
		plugins.AfterStage(plugins.RouteStage),
	)
	if err != nil {
		h.listenerReporter.SetCondition(sdkreporter.ListenerCondition{
			Type:    gwv1.ListenerConditionProgrammed,
			Reason:  gwv1.ListenerReasonInvalid,
			Status:  metav1.ConditionFalse,
			Message: "Error processing http plugins: " + err.Error(),
		})
		// TODO: return false?
	}

	envoyHttpFilters = append(envoyHttpFilters, newStagedFilter.Filter)

	return envoyHttpFilters
}

func convertCustomHttpFilters(customHttpFilters []ir.CustomEnvoyFilter) []plugins.StagedHttpFilter {
	var out []plugins.StagedHttpFilter
	for _, customFilter := range customHttpFilters {
		stagedFilter := plugins.StagedHttpFilter{
			Filter: &envoyhttp.HttpFilter{
				Name: customFilter.Name,
				ConfigType: &envoyhttp.HttpFilter_TypedConfig{
					TypedConfig: customFilter.Config,
				},
			},
			Stage: customFilter.FilterStage,
		}
		out = append(out, stagedFilter)
	}
	return out
}

func sortHttpFilters(filters plugins.StagedHttpFilterList) []*envoyhttp.HttpFilter {
	sort.Sort(filters)
	var sortedFilters []*envoyhttp.HttpFilter
	for _, filter := range filters {
		if len(sortedFilters) > 0 && proto.Equal(sortedFilters[len(sortedFilters)-1], filter.Filter) {
			// skip repeated equal filters
			continue
		}
		sortedFilters = append(sortedFilters, filter.Filter)
	}
	return sortedFilters
}

func (h *filterChainTranslator) computeTcpFilters(ctx context.Context, l ir.TcpIR, reporter sdkreporter.ListenerReporter) []*envoylistenerv3.Filter {
	networkFilters := sortNetworkFilters(h.computeCustomFilters(ctx, l.CustomNetworkFilters, reporter))

	cfg := &envoytcp.TcpProxy{
		StatPrefix: l.FilterChainName,
	}
	if len(l.BackendRefs) == 1 {
		cfg.ClusterSpecifier = &envoytcp.TcpProxy_Cluster{
			Cluster: l.BackendRefs[0].ClusterName,
		}
	} else {
		var wc envoytcp.TcpProxy_WeightedCluster
		for _, route := range l.BackendRefs {
			w := route.Weight
			if w == 0 {
				w = 1
			}
			wc.Clusters = append(wc.GetClusters(), &envoytcp.TcpProxy_WeightedCluster_ClusterWeight{
				Name:   route.ClusterName,
				Weight: w,
			})
		}
		cfg.ClusterSpecifier = &envoytcp.TcpProxy_WeightedClusters{
			WeightedClusters: &wc,
		}
	}

	tcpFilter, _ := NewFilterWithTypedConfig(wellknown.TCPProxy, cfg)

	return append(networkFilters, tcpFilter)
}

func NewFilterWithTypedConfig(name string, config proto.Message) (*envoylistenerv3.Filter, error) {
	s := &envoylistenerv3.Filter{
		Name: name,
	}

	if config != nil {
		marshalledConf, err := utils.MessageToAny(config)
		if err != nil {
			// this should NEVER HAPPEN!
			return &envoylistenerv3.Filter{}, err
		}

		s.ConfigType = &envoylistenerv3.Filter_TypedConfig{
			TypedConfig: marshalledConf,
		}
	}

	return s, nil
}

type SslConfig struct {
	Bundle     TlsBundle
	SniDomains []string
}
type TlsBundle struct {
	CA         []byte
	PrivateKey []byte
	CertChain  []byte
}

type FilterChainInfo struct {
	Match ir.FilterChainMatch
	TLS   *ir.TlsBundle
}

func (info *FilterChainInfo) toMatch() *envoylistenerv3.FilterChainMatch {
	if info == nil {
		return nil
	}

	// if all fields are empty, return nil
	if len(info.Match.SniDomains) == 0 && info.Match.DestinationPort == nil && len(info.Match.PrefixRanges) == 0 {
		return nil
	}

	return &envoylistenerv3.FilterChainMatch{
		ServerNames:     info.Match.SniDomains,
		DestinationPort: info.Match.DestinationPort,
		PrefixRanges:    info.Match.PrefixRanges,
	}
}

func (info *FilterChainInfo) toTransportSocket() *envoycorev3.TransportSocket {
	if info == nil {
		return nil
	}
	ssl := info.TLS
	if ssl == nil {
		return nil
	}

	common := &envoytlsv3.CommonTlsContext{
		// default params
		TlsParams:     &envoytlsv3.TlsParameters{},
		AlpnProtocols: ssl.AlpnProtocols,
	}

	common.TlsCertificates = []*envoytlsv3.TlsCertificate{
		{
			CertificateChain: bytesDataSource(ssl.CertChain),
			PrivateKey:       bytesDataSource(ssl.PrivateKey),
		},
	}

	//	var requireClientCert *wrappers.BoolValue
	//	if common.GetValidationContextType() != nil {
	//		requireClientCert = &wrappers.BoolValue{Value: !dc.GetOneWayTls().GetValue()}
	//	}

	// default alpn for downstreams.
	//	if len(common.GetAlpnProtocols()) == 0 {
	//		common.AlpnProtocols = []string{"h2", "http/1.1"}
	//	} else if len(common.GetAlpnProtocols()) == 1 && common.GetAlpnProtocols()[0] == AllowEmpty { // allow override for advanced usage to set to a dangerous setting
	//		common.AlpnProtocols = []string{}
	//	}

	out := &envoytlsv3.DownstreamTlsContext{
		CommonTlsContext: common,
	}
	typedConfig, _ := utils.MessageToAny(out)

	return &envoycorev3.TransportSocket{
		Name:       wellknown.TransportSocketTls,
		ConfigType: &envoycorev3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
	}
}

func bytesDataSource(s []byte) *envoycorev3.DataSource {
	return &envoycorev3.DataSource{
		Specifier: &envoycorev3.DataSource_InlineBytes{
			InlineBytes: s,
		},
	}
}
