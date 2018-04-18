package v1

import (
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
)

type UpstreamSpec *types.Struct
type FunctionSpec *types.Struct

type ConfigObject interface {
	proto.Message
	GetName() string
	GetStatus() *Status
	GetMetadata() *Metadata
}
