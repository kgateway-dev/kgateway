package translator

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"

	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyal "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/conversion"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/ptypes"
	golangptypes "github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/util"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// returns the name of the cluster created for a given upstream
func UpstreamToClusterName(upstream core.ResourceRef) string {

	// For non-namespaced resources, return only name
	if upstream.Namespace == "" {
		return upstream.Name
	}

	// Don't use dots in the name as it messes up prometheus stats
	return fmt.Sprintf("%s_%s", upstream.Name, upstream.Namespace)
}

func NewFilterWithConfig(name string, config proto.Message) (*envoylistener.Filter, error) {

	s := &envoylistener.Filter{
		Name: name,
	}

	if config != nil {
		marshalledConf, err := envoyutil.MessageToStruct(config)
		if err != nil {
			// this should NEVER HAPPEN!
			return &envoylistener.Filter{}, err
		}

		s.ConfigType = &envoylistener.Filter_Config{
			Config: marshalledConf,
		}
	}

	return s, nil
}

func NewFilterWithTypedConfig(name string, config proto.Message) (*envoylistener.Filter, error) {

	s := &envoylistener.Filter{
		Name: name,
	}

	if config != nil {
		marshalledConf, err := pluginutils.MessageToAny(config)
		if err != nil {
			// this should NEVER HAPPEN!
			return &envoylistener.Filter{}, err
		}

		s.ConfigType = &envoylistener.Filter_TypedConfig{
			TypedConfig: marshalledConf,
		}
	}

	return s, nil
}

func NewAccessLogWithConfig(name string, config proto.Message) (envoyal.AccessLog, error) {
	s := envoyal.AccessLog{
		Name: name,
	}

	if config != nil {
		marshalledConf, err := golangptypes.MarshalAny(config)
		if err != nil {
			// this should NEVER HAPPEN!
			return envoyal.AccessLog{}, err
		}

		s.ConfigType = &envoyal.AccessLog_TypedConfig{
			TypedConfig: marshalledConf,
		}
	}

	return s, nil
}

func ParseGogoConfig(c gogoConfigObject, config proto.Message) error {
	any := c.GetTypedConfig()
	if any != nil {
		return types.UnmarshalAny(any, config)
	}
	structt := c.GetConfig()
	if structt != nil {
		return util.StructToMessage(structt, config)
	}
	return nil
}

type gogoConfigObject interface {
	GetConfig() *types.Struct
	GetTypedConfig() *types.Any
}

func ParseConfig(c configObject, config proto.Message) error {
	any := c.GetTypedConfig()
	if any != nil {
		return ptypes.UnmarshalAny(any, config)
	}
	structt := c.GetConfig()
	if structt != nil {
		return envoyutil.StructToMessage(structt, config)
	}
	return nil
}

type configObject interface {
	GetConfig() *structpb.Struct
	GetTypedConfig() *any.Any
}

func ParseTypedConfig(c typedConfigObject, config proto.Message) error {
	any := c.GetTypedConfig()
	if any != nil {
		return ptypes.UnmarshalAny(any, config)
	}
	return nil
}

type typedConfigObject interface {
	GetTypedConfig() *any.Any
}
