package plugins

import (
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/gogo/protobuf/proto"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

func NewStagedFilter(name string, stage FilterStage) StagedHttpFilter {
	s, _ := NewStagedFilterWithConfig(name, nil, stage)
	return s
}

func NewStagedFilterWithConfig(name string, config proto.Message, stage FilterStage) (StagedHttpFilter, error) {

	s := StagedHttpFilter{
		HttpFilter: &envoyhttp.HttpFilter{
			Name: name,
		},
		Stage: stage,
	}

	if config != nil {

		marshalledConf, err := utils.MessageToAny(config)
		if err != nil {
			// this should NEVER HAPPEN!
			return StagedHttpFilter{}, err
		}

		s.HttpFilter.ConfigType = &envoyhttp.HttpFilter_TypedConfig{
			TypedConfig: marshalledConf,
		}
	}

	return s, nil
}
