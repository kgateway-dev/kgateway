package routing

import (
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
)

func DecodeRouteExtensions(generic *types.Struct) (*RouteExtensions, error) {
	cfg := new(RouteExtensions)
	if err := util.StructToMessage(generic, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func EncodeRouteExtensionSpec(spec *RouteExtensions) *types.Struct {
	if spec == nil {
		return nil
	}
	s, err := util.MessageToStruct(spec)
	if err != nil {
		panic("failed to encode listener config: " + err.Error())
	}
	return s
}
