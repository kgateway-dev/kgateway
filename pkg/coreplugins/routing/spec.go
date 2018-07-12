package routing

import (
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
	s, err := protoutil.MarshalStruct(spec)
	if err != nil {
		panic("failed to encode listener config: " + err.Error())
	}
	return s
}
