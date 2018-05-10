package fake

import (
	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/service"
)

type UpstreamSpec = service.UpstreamSpec

func DecodeUpstreamSpec(generic v1.UpstreamSpec) (UpstreamSpec, error) {
	return service.DecodeUpstreamSpec(generic)
}

func EncodeUpstreamSpec(spec UpstreamSpec) *types.Struct {
	return service.EncodeUpstreamSpec(spec)
}
