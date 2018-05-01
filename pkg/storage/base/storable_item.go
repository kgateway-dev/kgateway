package base

import (
	"github.com/gogo/protobuf/proto"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

type StorableItem struct {
	Upstream       *v1.Upstream
	VirtualService *v1.VirtualService
	Role    *v1.Role
	File           *dependencies.File
}

func (item *StorableItem) GetName() string {
	switch {
	case item.Upstream != nil:
		return item.Upstream.GetName()
	case item.VirtualService != nil:
		return item.VirtualService.GetName()
	case item.Role != nil:
		return item.Role.GetName()
	case item.File != nil:
		return item.File.Ref
	default:
		panic("virtual service, role, fileor upstream must be set")
	}
}

func (item *StorableItem) GetResourceVersion() string {
	switch {
	case item.Upstream != nil:
		if item.Upstream.GetMetadata() == nil {
			return ""
		}
		return item.Upstream.GetMetadata().GetResourceVersion()
	case item.VirtualService != nil:
		if item.VirtualService.GetMetadata() == nil {
			return ""
		}
		return item.VirtualService.GetMetadata().GetResourceVersion()
	case item.Role != nil:
		if item.Role.GetMetadata() == nil {
			return ""
		}
		return item.Role.GetMetadata().GetResourceVersion()
	case item.File != nil:
		return item.File.ResourceVersion
	default:
		panic("virtual service, role, fileor upstream must be set")
	}
}

func (item *StorableItem) SetResourceVersion(rv string) {
	switch {
	case item.Upstream != nil:
		if item.Upstream.GetMetadata() == nil {
			item.Upstream.Metadata = &v1.Metadata{}
		}
		item.Upstream.Metadata.ResourceVersion = rv
	case item.VirtualService != nil:
		if item.VirtualService.GetMetadata() == nil {
			item.VirtualService.Metadata = &v1.Metadata{}
		}
		item.VirtualService.Metadata.ResourceVersion = rv
	case item.Role != nil:
		if item.Role.GetMetadata() == nil {
			item.Role.Metadata = &v1.Metadata{}
		}
		item.Role.Metadata.ResourceVersion = rv
	case item.File != nil:
		item.File.ResourceVersion = rv
	default:
		panic("virtual service, role, fileor upstream must be set")
	}
}

func (item *StorableItem) GetBytes() ([]byte, error) {
	switch {
	case item.Upstream != nil:
		return proto.Marshal(item.Upstream)
	case item.VirtualService != nil:
		return proto.Marshal(item.VirtualService)
	case item.Role != nil:
		return proto.Marshal(item.Role)
	case item.File != nil:
		return item.File.Contents, nil
	default:
		panic("virtual service, role, fileor upstream must be set")
	}
}

func (item *StorableItem) GetTypeFlag() StorableItemType {
	switch {
	case item.Upstream != nil:
		return StorableItemTypeUpstream
	case item.VirtualService != nil:
		return StorableItemTypeVirtualService
	case item.Role != nil:
		return StorableItemTypeRole
	case item.File != nil:
		return StorableItemTypeFile
	default:
		panic("virtual service, role, fileor upstream must be set")
	}
}

type StorableItemType uint64

const (
	StorableItemTypeUpstream StorableItemType = iota
	StorableItemTypeVirtualService
	StorableItemTypeRole
	StorableItemTypeFile
)

type StorableItemEventHandler struct {
	UpstreamEventHandler       storage.UpstreamEventHandler
	VirtualServiceEventHandler storage.VirtualServiceEventHandler
	RoleEventHandler    storage.RoleEventHandler
	FileEventHandler           dependencies.FileEventHandler
}
