package translator

import (
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
)

func UpstreamToClusterName(upstream core.ResourceRef) string {
	return upstream.Key()
}

func NewFilterWithConfig(name string, config proto.Message) (envoylistener.Filter, error) {

	s := envoylistener.Filter{
		Name: name,
	}

	if config != nil {
		marshalledConf, err := protoutils.MarshalStruct(config)
		if err != nil {
			// this should NEVER HAPPEN!
			return envoylistener.Filter{}, err
		}

		s.ConfigType = &envoylistener.Filter_Config{
			Config: marshalledConf,
		}
	}

	return s, nil
}

func ParseConfig(c configObject, config proto.Message) error {
	any := c.GetTypedConfig()
	if any != nil {
		return types.UnmarshalAny(any, config)
	}
	structt := c.GetConfig()
	if structt != nil {
		return protoutils.UnmarshalStruct(structt, config)
	}
	return nil
}

type configObject interface {
	GetConfig() *types.Struct
	GetTypedConfig() *types.Any
}
