package base

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

func key(rootPath, itemName string) string {
	return rootPath + "/" + itemName
}

func toKVPair(rootPath string, item *StorableItem) (*api.KVPair, error) {
	data, err := item.GetBytes()
	if err != nil {
		return nil, errors.Wrap(err, "getting bytes to store")
	}
	var modifyIndex uint64
	if item.GetResourceVersion() != "" {
		if i, err := strconv.Atoi(item.GetResourceVersion()); err == nil {
			modifyIndex = uint64(i)
		}
	}
	return &api.KVPair{
		Key:         key(rootPath, item.GetName()),
		Value:       data,
		Flags:       uint64(item.GetTypeFlag()),
		ModifyIndex: modifyIndex,
	}, nil
}

func setResourceVersion(item *StorableItem, p *api.KVPair) {
	resourceVersion := fmt.Sprintf("%v", p.ModifyIndex)
	item.SetResourceVersion(resourceVersion)
}

func itemFromKVPair(rootPath string, p *api.KVPair) (*StorableItem, error) {
	item := &StorableItem{}
	switch StorableItemType(p.Flags) {
	case StorableItemTypeUpstream:
		var us v1.Upstream
		err := proto.Unmarshal(p.Value, &us)
		if err != nil {
			return nil, errors.Wrap(err, "unmarshalling value as upstream")
		}
		item.Upstream = &us
	case StorableItemTypeVirtualService:
		var vs v1.VirtualService
		err := proto.Unmarshal(p.Value, &vs)
		if err != nil {
			return nil, errors.Wrap(err, "unmarshalling value as virtualservice")
		}
		item.VirtualService = &vs
	case StorableItemTypeFile:
		item.File = &dependencies.File{
			Ref:      strings.TrimPrefix(p.Key, rootPath+"/"),
			Contents: p.Value,
		}
	}
	setResourceVersion(item, p)
	return item, nil
}
