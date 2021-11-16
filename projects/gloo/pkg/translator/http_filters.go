package translator

import (
	"sort"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/wrappers"
	errors "github.com/rotisserie/eris"
	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/log"
)

const (
	DefaultHttpStatPrefix = "http"
)

func NewHttpConnectionManager(
	listener *v1.HttpListener,
	httpFilters []*envoyhttp.HttpFilter,
	rdsName string,
) *envoyhttp.HttpConnectionManager {
	statPrefix := listener.GetStatPrefix()
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

func (h *httpFilterChainTranslator) computeHttpConnectionManagerFilter(params plugins.Params) *envoy_config_listener_v3.Filter {
	httpFilters := h.computeHttpFilters(params)
	params.Ctx = contextutils.WithLogger(params.Ctx, "compute_http_connection_manager")

	httpConnMgr := NewHttpConnectionManager(h.listener, httpFilters, h.routeConfigName)

	hcmFilter, err := NewFilterWithTypedConfig(wellknown.HTTPConnectionManager, httpConnMgr)
	if err != nil {
		panic(errors.Wrap(err, "failed to convert proto message to struct"))
	}
	return hcmFilter
}

func (h *httpFilterChainTranslator) computeHttpFilters(params plugins.Params) []*envoyhttp.HttpFilter {
	var httpFilters []plugins.StagedHttpFilter
	// run the Http Filter Plugins
	for _, plug := range h.plugins {
		filterPlugin, ok := plug.(plugins.HttpFilterPlugin)
		if !ok {
			continue
		}
		stagedFilters, err := filterPlugin.HttpFilters(params, h.listener)
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

	// sort filters by stage
	envoyHttpFilters := sortFilters(httpFilters)
	envoyHttpFilters = append(envoyHttpFilters, &envoyhttp.HttpFilter{Name: wellknown.Router})
	return envoyHttpFilters
}

func sortFilters(filters plugins.StagedHttpFilterList) []*envoyhttp.HttpFilter {
	sort.Sort(filters)
	var sortedFilters []*envoyhttp.HttpFilter
	for _, filter := range filters {
		sortedFilters = append(sortedFilters, filter.HttpFilter)
	}
	return sortedFilters
}
