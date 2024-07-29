package translator

import (
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	http_connection_managerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

func CustomNetworkFiltersHTTP(listener *v1.HttpListener) []plugins.StagedFilter[plugins.WellKnownFilterStage, *listenerv3.Filter] {
	var out []plugins.StagedFilter[plugins.WellKnownFilterStage, *listenerv3.Filter]
	for _, customFilter := range listener.GetCustomNetworkFilters() {
		out = append(out, plugins.StagedFilter[plugins.WellKnownFilterStage, *listenerv3.Filter]{
			Filter: &listenerv3.Filter{
				Name: customFilter.GetName(),
				ConfigType: &listenerv3.Filter_TypedConfig{
					TypedConfig: customFilter.GetConfig(),
				},
			},
			Stage: *plugins.ConvertFilterStage(customFilter.GetFilterStage()),
		})
	}
	return out
}

func CustomNetworkFiltersTCP(listener *v1.TcpListener) []plugins.StagedFilter[plugins.WellKnownFilterStage, *listenerv3.Filter] {
	var out []plugins.StagedFilter[plugins.WellKnownFilterStage, *listenerv3.Filter]
	for _, customFilter := range listener.GetCustomNetworkFilters() {
		out = append(out, plugins.StagedFilter[plugins.WellKnownFilterStage, *listenerv3.Filter]{
			Filter: &listenerv3.Filter{
				Name: customFilter.GetName(),
				ConfigType: &listenerv3.Filter_TypedConfig{
					TypedConfig: customFilter.GetConfig(),
				},
			},
			Stage: *plugins.ConvertFilterStage(customFilter.GetFilterStage()),
		})
	}
	return out
}

func CustomHttpFilters(listener *v1.HttpListener) []plugins.StagedFilter[plugins.WellKnownFilterStage, *http_connection_managerv3.HttpFilter] {
	var out []plugins.StagedFilter[plugins.WellKnownFilterStage, *http_connection_managerv3.HttpFilter]
	for _, customFilter := range listener.GetCustomHttpFilters() {
		out = append(out, plugins.StagedFilter[plugins.WellKnownFilterStage, *http_connection_managerv3.HttpFilter]{
			Filter: &http_connection_managerv3.HttpFilter{
				Name: customFilter.GetName(),
				ConfigType: &http_connection_managerv3.HttpFilter_TypedConfig{
					TypedConfig: customFilter.GetConfig(),
				},
			},
			Stage: *plugins.ConvertFilterStage(customFilter.GetFilterStage()),
		})
	}
	return out
}
