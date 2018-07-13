package routing

import (
	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/protoutil"
)

func DecodeRouteExtensions(generic *types.Struct) (*RouteExtensions, error) {
	cfg := new(RouteExtensions)
	if err := protoutil.UnmarshalStruct(generic, cfg); err != nil {
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
