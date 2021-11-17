package translator

import (
	"sort"

	"github.com/solo-io/go-utils/contextutils"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/wrappers"
	errors "github.com/rotisserie/eris"
	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/go-utils/log"
)

const (
	DefaultHttpStatPrefix = "http"
)

func NewHttpConnectionManager(
	httpFilters []*envoyhttp.HttpFilter,
	rdsName string,
	statPrefix string,
) *envoyhttp.HttpConnectionManager {
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
				RouteConfigName: rdsName,
			},
		},
		HttpFilters: httpFilters,
	}
}

func (h *httpNetworkFilterTranslator) computeHttpConnectionManagerFilter(params plugins.Params) *envoy_config_listener_v3.Filter {
	params.Ctx = contextutils.WithLogger(params.Ctx, "compute_http_connection_manager")

	httpFilters := h.computeHttpFilters(params)

	httpConnMgr := NewHttpConnectionManager(httpFilters, h.routeConfigName, h.listener.GetStatPrefix())

	hcmFilter, err := NewFilterWithTypedConfig(wellknown.HTTPConnectionManager, httpConnMgr)
	if err != nil {
		panic(errors.Wrap(err, "failed to convert proto message to struct"))
	}
	return hcmFilter
}

func (h *httpNetworkFilterTranslator) computeHttpFilters(params plugins.Params) []*envoyhttp.HttpFilter {
	var httpFilters []plugins.StagedHttpFilter

	// run the HttpFilter Plugins
	for _, plug := range h.plugins {
		stagedFilters, err := plug.HttpFilters(params, h.listener)
		if err != nil {
			validation.AppendHTTPListenerError(h.report, validationapi.HttpListenerReport_Error_ProcessingError, err.Error())
		}

		for _, httpFilter := range stagedFilters {
			if httpFilter.HttpFilter == nil {
				log.Warnf("plugin implements HttpFilters() but returned nil")
				continue
			}
			httpFilters = append(httpFilters, httpFilter)
		}
	}

	// https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/http/http_filters#filter-ordering
	// HttpFilter ordering determines the order in which the HCM will execute the filter.

	// 1. Sort filters by stage
	// "Stage" is the type we use to specify when a filter should be run
	envoyHttpFilters := sortHttpFilters(httpFilters)

	// 2. Configure the router filter
	// As outlined by the Envoy docs, the last configured filter has to be a terminal filter.
	// We set the Router filter (https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/router_filter#config-http-filters-router)
	// as the terminal filter in Gloo Edge.
	envoyHttpFilters = append(envoyHttpFilters, &envoyhttp.HttpFilter{Name: wellknown.Router})

	return envoyHttpFilters
}

func sortHttpFilters(filters plugins.StagedHttpFilterList) []*envoyhttp.HttpFilter {
	sort.Sort(filters)
	var sortedFilters []*envoyhttp.HttpFilter
	for _, filter := range filters {
		sortedFilters = append(sortedFilters, filter.HttpFilter)
	}
	return sortedFilters
}
