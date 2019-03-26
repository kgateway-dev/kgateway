package translator

import (
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func UpstreamToClusterName(upstream core.ResourceRef) string {
	return upstream.Key()
}

func NewFilterWithConfig(name string, config proto.Message) (envoylistener.Filter, error) {

	s := envoylistener.Filter{
		Name: name,
	}

	if config != nil {
		marshalledConf, err := envoyutil.MessageToStruct(config)
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
		return envoyutil.StructToMessage(structt, config)
	}
	return nil
}

type configObject interface {
	GetConfig() *types.Struct
	GetTypedConfig() *types.Any
}
