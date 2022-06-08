package plugins

import (
	"errors"

	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/golang/protobuf/proto"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

// NewStagedFilterWithConfig is deprecated as config is now always needed
// See the new signature of NewStagedFilter
func NewStagedFilterWithConfig(name string, config proto.Message, stage FilterStage) (StagedHttpFilter, error) {
	return NewStagedFilter(name, config, stage)
}

// NewStagedFilter creates an instance of the named filter with the desired stage
func NewStagedFilter(name string, config proto.Message, stage FilterStage) (StagedHttpFilter, error) {

	s := StagedHttpFilter{
		HttpFilter: &envoyhttp.HttpFilter{
			Name: name,
		},
		Stage: stage,
	}

	if config == nil {
		return s, errors.New("filters must have a config specified to derive its type")
	}

	marshalledConf, err := utils.MessageToAny(config)
	if err != nil {
		// all config types should already be known
		// therefore this should never happen
		return StagedHttpFilter{}, err
	}

	s.HttpFilter.ConfigType = &envoyhttp.HttpFilter_TypedConfig{
		TypedConfig: marshalledConf,
	}

	return s, nil
}

// StagedFilterListContainsName checks for a given named filter.
// This is not a check of the type url but rather the now mostly unused name
func StagedFilterListContainsName(filters StagedHttpFilterList, filterName string) bool {
	for _, filter := range filters {
		if filter.HttpFilter.GetName() == filterName {
			return true
		}
	}

	return false
}
