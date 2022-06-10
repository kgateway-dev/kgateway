package plugins

import (
	"errors"

	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/golang/protobuf/proto"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

// NewStagedFilterWithConfig creates an instance of the named filter with the desired stage.
// Deprecated: config is now always needed and so NewStagedFilter should always be used.
func NewStagedFilterWithConfig(name string, config proto.Message, stage FilterStage) (StagedHttpFilter, error) {
	return NewStagedFilter(name, config, stage)
}

// MustNewStagedFilter creates an instance of the named filter with the desired stage.
// Returns a filter even if an error occured.
// Should rarely be used as disregarding an error is bad practice but does make
// appending easier.
// If not directly appending consider using NewStagedFilter instead of this function.
func MustNewStagedFilter(name string, config proto.Message, stage FilterStage) StagedHttpFilter {
	s, _ := NewStagedFilter(name, config, stage)
	return s
}

// NewStagedFilter creates an instance of the named filter with the desired stage.
// Errors if the config is nil or we cannot determine the type of the config.
// Config type determination may fail if the config is both  unknown and has no fields.
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
