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
	SetName(name string)
	SetStatus(status *Status)
	SetMetadata(meta *Metadata)
}

// because proto refuses to do setters

func (item *Upstream) SetName(name string) {
	item.Name = name
}

func (item *Upstream) SetStatus(status *Status) {
	item.Status = status
}

func (item *Upstream) SetMetadata(meta *Metadata) {
	item.Metadata = meta
}

func (item *VirtualService) SetName(name string) {
	item.Name = name
}

func (item *VirtualService) SetStatus(status *Status) {
	item.Status = status
}

func (item *VirtualService) SetMetadata(meta *Metadata) {
	item.Metadata = meta
}

func (item *Role) SetName(name string) {
	item.Name = name
}

func (item *Role) SetStatus(status *Status) {
	item.Status = status
}

func (item *Role) SetMetadata(meta *Metadata) {
	item.Metadata = meta
}

func (item *Attribute) SetName(name string) {
	item.Name = name
}

func (item *Attribute) SetStatus(status *Status) {
	item.Status = status
}

func (item *Attribute) SetMetadata(meta *Metadata) {
	item.Metadata = meta
}
