package fake

import (
	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/static"
)

type UpstreamSpec = static.UpstreamSpec

func DecodeUpstreamSpec(generic v1.UpstreamSpec) (*UpstreamSpec, error) {
	return static.DecodeUpstreamSpec(generic)
}

func EncodeUpstreamSpec(spec UpstreamSpec) *types.Struct {
	return static.EncodeUpstreamSpec(&spec)
}
