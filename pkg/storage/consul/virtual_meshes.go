package consul

import (
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/base"
)

type virtualMeshesClient struct {
	base *base.ConsulStorageClient
}

func (c *virtualMeshesClient) Create(item *v1.VirtualMesh) (*v1.VirtualMesh, error) {
	out, err := c.base.Create(&base.StorableItem{VirtualMesh: item})
	if err != nil {
		return nil, err
	}
	return out.VirtualMesh, nil
}

func (c *virtualMeshesClient) Update(item *v1.VirtualMesh) (*v1.VirtualMesh, error) {
	out, err := c.base.Update(&base.StorableItem{VirtualMesh: item})
	if err != nil {
		return nil, err
	}
	return out.VirtualMesh, nil
}

func (c *virtualMeshesClient) Delete(name string) error {
	return c.base.Delete(name)
}

func (c *virtualMeshesClient) Get(name string) (*v1.VirtualMesh, error) {
	out, err := c.base.Get(name)
	if err != nil {
		return nil, err
	}
	return out.VirtualMesh, nil
}

func (c *virtualMeshesClient) List() ([]*v1.VirtualMesh, error) {
	list, err := c.base.List()
	if err != nil {
		return nil, err
	}
	var virtualMeshes []*v1.VirtualMesh
	for _, obj := range list {
		virtualMeshes = append(virtualMeshes, obj.VirtualMesh)
	}
	return virtualMeshes, nil
}

func (c *virtualMeshesClient) Watch(handlers ...storage.VirtualMeshEventHandler) (*storage.Watcher, error) {
	var baseHandlers []base.StorableItemEventHandler
	for _, h := range handlers {
		baseHandlers = append(baseHandlers, base.StorableItemEventHandler{VirtualMeshEventHandler: h})
	}
	return c.base.Watch(baseHandlers...)
}
