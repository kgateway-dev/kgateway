package ir

import (
	"context"
	"fmt"
	"sort"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	codecv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/upstream_codec/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/wrappers"
	"go.uber.org/zap"

	"github.com/solo-io/gloo/projects/controller/pkg/plugins"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gateway2/model"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/go-utils/contextutils"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	DefaultHttpStatPrefix  = "http"
	UpstreamCodeFilterName = "envoy.filters.http.upstream_codec"
)

type filterChainTranslator struct {
	gw       model.GatewayIR
	listener model.ListenerIR

	parentRef                gwv1.ParentReference
	routeConfigName          string
	reporter                 reports.Reporter
	requireTlsOnVirtualHosts bool
	PluginPass               map[schema.GroupKind]extensions.ProxyTranslationPass
}

func (h *filterChainTranslator) ComputeFilterChains(ctx context.Context, l model.ListenerIR, reporter reports.GatewayReporter) []*envoy_config_listener_v3.FilterChain {
	for _, hfc := range l.HttpFilterChain {
		h.computeHttpFilterChain(ctx, hfc, reporter.ListenerName(hfc.FilterChainName))
	}
	panic("TODO")
}

func (h *filterChainTranslator) computeHttpFilterChain(ctx context.Context, l model.HttpFilterChainIR, reporter reports.ListenerReporter) []*envoy_config_listener_v3.FilterChain {

	h.computeNetworkFilters(ctx, l, reporter)
	panic("TODO")
}

func (n *filterChainTranslator) computeNetworkFilters(ctx context.Context, l model.HttpFilterChainIR, reporter reports.ListenerReporter) ([]*envoy_config_listener_v3.Filter, error) {
	hcm := hcmNetworkFilterTranslator{
		routeConfigName: l.FilterChainName,
		PluginPass:      n.PluginPass,
		reporter:        reporter,
	}
	var networkFilters []*envoy_config_listener_v3.Filter
	networkFilter, err := hcm.computeNetworkFilters(ctx, l)
	if err != nil {
		return nil, err
	}
	networkFilters = append(networkFilters, networkFilter)
	return networkFilters, nil
}

type hcmNetworkFilterTranslator struct {
	routeConfigName string
	PluginPass      map[schema.GroupKind]extensions.ProxyTranslationPass
	reporter        reports.ListenerReporter
	listener        model.HttpFilterChainIR
}

func (h *hcmNetworkFilterTranslator) computeNetworkFilters(ctx context.Context, l model.HttpFilterChainIR) (*envoy_config_listener_v3.Filter, error) {
	ctx = contextutils.WithLogger(ctx, "compute_http_connection_manager")

	// 1. Initialize the HCM
	httpConnectionManager := h.initializeHCM()

	// 2. Apply HttpFilters
	var err error
	httpConnectionManager.HttpFilters = h.computeHttpFilters(ctx, l)

	// 3. Allow any HCM plugins to make their changes, with respect to any changes the core plugin made
	//	for _, hcmPlugin := range h.hcmPlugins {
	//		if err := hcmPlugin.ProcessHcmNetworkFilter(params, h.parentListener, h.listener, httpConnectionManager); err != nil {
	//			h.reporter.SetCondition(reports.ListenerCondition{
	//				Type:    gwv1.ListenerConditionProgrammed,
	//				Reason:  gwv1.ListenerReasonInvalid,
	//				Status:  metav1.ConditionFalse,
	//				Message: "Error processing HCM plugin: " + err.Error(),
	//			})
	//		}
	//	}

	// 4. Generate the typedConfig for the HCM
	hcmFilter, err := NewFilterWithTypedConfig(wellknown.HTTPConnectionManager, httpConnectionManager)
	if err != nil {
		contextutils.LoggerFrom(ctx).DPanic("failed to convert proto message to struct")
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
		CodecType:  envoyhttp.HttpConnectionManager_AUTO,
		StatPrefix: statPrefix,
		NormalizePath: &wrappers.BoolValue{
			Value: true,
		},
		RouteSpecifier: &envoyhttp.HttpConnectionManager_Rds{
			Rds: &envoyhttp.Rds{
				ConfigSource: &envoy_config_core_v3.ConfigSource{
					ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
					ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_Ads{
						Ads: &envoy_config_core_v3.AggregatedConfigSource{},
					},
				},
				RouteConfigName: h.routeConfigName,
			},
		},
	}
}

func (h *hcmNetworkFilterTranslator) computeHttpFilters(ctx context.Context, l model.HttpFilterChainIR) []*envoyhttp.HttpFilter {
	var httpFilters plugins.StagedHttpFilterList

	log := contextutils.LoggerFrom(ctx).Desugar()

	// run the HttpFilter Plugins
	for _, plug := range h.PluginPass {
		stagedFilters, err := plug.HttpFilters(ctx)
		if err != nil {
			// what to do with errors here? ignore the listener??
			h.reporter.SetCondition(reports.ListenerCondition{
				Type:    gwv1.ListenerConditionProgrammed,
				Reason:  gwv1.ListenerReasonInvalid,
				Status:  metav1.ConditionFalse,
				Message: "Error processing http plugin: " + err.Error(),
			})
			// TODO: return false?
		}

		for _, httpFilter := range stagedFilters {
			if httpFilter.Filter == nil {
				log.Warn("HttpFilters() returned nil", zap.String("name", plug.Name()))
				continue
			}
			httpFilters = append(httpFilters, httpFilter)
		}
	}
	//	httpFilters = append(httpFilters, CustomHttpFilters(h.listener)...)

	// https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/http/http_filters#filter-ordering
	// HttpFilter ordering determines the order in which the HCM will execute the filter.

	// 1. Sort filters by stage
	// "Stage" is the type we use to specify when a filter should be run
	envoyHttpFilters := sortHttpFilters(httpFilters)

	// 2. Configure the router filter
	// As outlined by the Envoy docs, the last configured filter has to be a terminal filter.
	// We set the Router filter (https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/router_filter#config-http-filters-router)
	// as the terminal filter in k8sgateway.
	routerV3 := routerv3.Router{}

	h.computeUpstreamHTTPFilters(ctx, l, &routerV3)

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
		h.reporter.SetCondition(reports.ListenerCondition{
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

func (h *hcmNetworkFilterTranslator) computeUpstreamHTTPFilters(ctx context.Context, l model.HttpFilterChainIR, routerV3 *routerv3.Router) {
	upstreamHttpFilters := plugins.StagedUpstreamHttpFilterList{}
	log := contextutils.LoggerFrom(ctx).Desugar()
	for _, plug := range h.PluginPass {
		stagedFilters, err := plug.UpstreamHttpFilters(ctx)
		if err != nil {
			// what to do with errors here? ignore the listener??
			h.reporter.SetCondition(reports.ListenerCondition{
				Type:    gwv1.ListenerConditionProgrammed,
				Reason:  gwv1.ListenerReasonInvalid,
				Status:  metav1.ConditionFalse,
				Message: "Error processing upstream http plugin: " + err.Error(),
			})
			// TODO: return false?
		}
		for _, httpFilter := range stagedFilters {
			if httpFilter.Filter == nil {
				log.Warn("HttpFilters() returned nil", zap.String("name", plug.Name()))
				continue
			}
			upstreamHttpFilters = append(upstreamHttpFilters, httpFilter)
		}
	}

	if len(upstreamHttpFilters) == 0 {
		return
	}

	sort.Sort(upstreamHttpFilters)

	sortedFilters := make([]*envoyhttp.HttpFilter, len(upstreamHttpFilters))
	for i, filter := range upstreamHttpFilters {
		sortedFilters[i] = filter.Filter
	}

	msg, err := anypb.New(&codecv3.UpstreamCodec{})
	if err != nil {
		// what to do with errors here? ignore the listener??
		h.reporter.SetCondition(reports.ListenerCondition{
			Type:    gwv1.ListenerConditionProgrammed,
			Reason:  gwv1.ListenerReasonInvalid,
			Status:  metav1.ConditionFalse,
			Message: "failed to convert proto message to any: " + err.Error(),
		})
		return
	}

	routerV3.UpstreamHttpFilters = sortedFilters
	routerV3.UpstreamHttpFilters = append(routerV3.GetUpstreamHttpFilters(), &envoyhttp.HttpFilter{
		Name: UpstreamCodeFilterName,
		ConfigType: &envoyhttp.HttpFilter_TypedConfig{
			TypedConfig: msg,
		},
	})
}

func sortHttpFilters(filters plugins.StagedHttpFilterList) []*envoyhttp.HttpFilter {
	sort.Sort(filters)
	var sortedFilters []*envoyhttp.HttpFilter
	for _, filter := range filters {
		sortedFilters = append(sortedFilters, filter.Filter)
	}
	return sortedFilters
}

func NewFilterWithTypedConfig(name string, config proto.Message) (*envoy_config_listener_v3.Filter, error) {

	s := &envoy_config_listener_v3.Filter{
		Name: name,
	}

	if config != nil {
		marshalledConf, err := anypb.New(config)
		if err != nil {
			// this should NEVER HAPPEN!
			return &envoy_config_listener_v3.Filter{}, err
		}

		s.ConfigType = &envoy_config_listener_v3.Filter_TypedConfig{
			TypedConfig: marshalledConf,
		}
	}

	return s, nil
}
