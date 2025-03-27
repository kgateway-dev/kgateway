package ir

import (
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/filters"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"

	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
)

func CustomNetworkFilters(
	extraFilters []*listenerv3.Filter,
	stage filters.FilterStage_Stage,
	predicate filters.FilterStage_Predicate,
) []*CustomEnvoyFilter {
	customFilters := make([]*CustomEnvoyFilter, 0, len(extraFilters))
	for _, f := range extraFilters {
		customFilters = append(customFilters, CustomNetworkFilter(f, stage, predicate))
	}
	return customFilters
}

func CustomNetworkFilter(
	f *listenerv3.Filter,
	stage filters.FilterStage_Stage,
	predicate filters.FilterStage_Predicate,
) *CustomEnvoyFilter {
	config := f.GetTypedConfig()
	if config == nil {
		return nil
	}

	return customFiltersHelper(stage, predicate, f.GetName(), config)
}

func CustomHTTPFilters(
	extraFilters []*hcmv3.HttpFilter,
	stage filters.FilterStage_Stage,
	predicate filters.FilterStage_Predicate,
) []*CustomEnvoyFilter {
	customFilters := make([]*CustomEnvoyFilter, 0, len(extraFilters))
	for _, f := range extraFilters {
		customFilters = append(customFilters, CustomHTTPFilter(f, stage, predicate))
	}
	return customFilters
}

func CustomHTTPFilter(
	f *hcmv3.HttpFilter,
	stage filters.FilterStage_Stage,
	predicate filters.FilterStage_Predicate,
) *CustomEnvoyFilter {
	config := f.GetTypedConfig()

	return customFiltersHelper(stage, predicate, f.GetName(), config)
}

func customFiltersHelper(
	stage filters.FilterStage_Stage,
	predicate filters.FilterStage_Predicate,
	name string,
	config *anypb.Any,
) *CustomEnvoyFilter {
	return &CustomEnvoyFilter{
		FilterStage: plugins.FilterStage[plugins.WellKnownFilterStage]{
			RelativeTo: plugins.WellKnownFilterStage(int(stage)),
			Weight:     int(predicate),
		},
		Name:   name,
		Config: config,
	}
}
