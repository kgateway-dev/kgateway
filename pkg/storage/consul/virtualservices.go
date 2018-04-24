package consul

import (
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/base"
)

type virtualServicesClient struct {
	base *base.ConsulStorageClient
}

func (c *virtualServicesClient) Create(item *v1.VirtualService) (*v1.VirtualService, error) {
	out, err := c.base.Create(&base.StorableItem{VirtualService: item})
	if err != nil {
		return nil, err
	}
	return out.VirtualService, nil
}

func (c *virtualServicesClient) Update(item *v1.VirtualService) (*v1.VirtualService, error) {
	out, err := c.base.Update(&base.StorableItem{VirtualService: item})
	if err != nil {
		return nil, err
	}
	return out.VirtualService, nil
}

func (c *virtualServicesClient) Delete(name string) error {
	return c.base.Delete(name)
}

func (c *virtualServicesClient) Get(name string) (*v1.VirtualService, error) {
	out, err := c.base.Get(name)
	if err != nil {
		return nil, err
	}
	return out.VirtualService, nil
}

func (c *virtualServicesClient) List() ([]*v1.VirtualService, error) {
	list, err := c.base.List()
	if err != nil {
		return nil, err
	}
	var virtualServices []*v1.VirtualService
	for _, obj := range list {
		virtualServices = append(virtualServices, obj.VirtualService)
	}
	return virtualServices, nil
}

func (c *virtualServicesClient) Watch(handlers ...storage.VirtualServiceEventHandler) (*storage.Watcher, error) {
	var baseHandlers []base.StorableItemEventHandler
	for _, h := range handlers {
		baseHandlers = append(baseHandlers, base.StorableItemEventHandler{VirtualServiceEventHandler: h})
	}
	return c.base.Watch(baseHandlers...)
}
