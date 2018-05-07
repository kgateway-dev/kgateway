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
	GetMetadata() *Metadata
	SetName(name string)
	SetMetadata(meta *Metadata)
}

// because proto refuses to do setters

func (item *Upstream) SetName(name string) {
	item.Name = name
}

func (item *Upstream) SetMetadata(meta *Metadata) {
	item.Metadata = meta
}

func (item *VirtualService) SetName(name string) {
	item.Name = name
}

func (item *VirtualService) SetMetadata(meta *Metadata) {
	item.Metadata = meta
}
