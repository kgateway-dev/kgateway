package waypoint

import (
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/filters"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func CustomNetworkFilters(
	extraFilters []*envoylistenerv3.Filter,
	stage filters.FilterStage_Stage,
	predicate filters.FilterStage_Predicate,
) []*ir.CustomEnvoyFilter {
	customFilters := make([]*ir.CustomEnvoyFilter, 0, len(extraFilters))
	for _, f := range extraFilters {
		customFilters = append(customFilters, CustomNetworkFilter(f, stage, predicate))
	}
	return customFilters
}

func CustomNetworkFilter(
	f *envoylistenerv3.Filter,
	stage filters.FilterStage_Stage,
	predicate filters.FilterStage_Predicate,
) *ir.CustomEnvoyFilter {
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
) []*ir.CustomEnvoyFilter {
	customFilters := make([]*ir.CustomEnvoyFilter, 0, len(extraFilters))
	for _, f := range extraFilters {
		customFilters = append(customFilters, CustomHTTPFilter(f, stage, predicate))
	}
	return customFilters
}

func CustomHTTPFilter(
	f *hcmv3.HttpFilter,
	stage filters.FilterStage_Stage,
	predicate filters.FilterStage_Predicate,
) *ir.CustomEnvoyFilter {
	config := f.GetTypedConfig()

	return customFiltersHelper(stage, predicate, f.GetName(), config)
}

func customFiltersHelper(
	stage filters.FilterStage_Stage,
	predicate filters.FilterStage_Predicate,
	name string,
	config *anypb.Any,
) *ir.CustomEnvoyFilter {
	return &ir.CustomEnvoyFilter{
		FilterStage: plugins.HTTPOrNetworkFilterStage{
			RelativeTo:     plugins.WellKnownFilterStage(int(stage)),
			RelativeWeight: int(predicate),
		},
		Name:   name,
		Config: config,
	}
}
