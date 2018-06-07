package consul

import (
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/base"
)

type upstreamsClient struct {
	base *base.ConsulStorageClient
}

func (c *upstreamsClient) Create(item *v1.Upstream) (*v1.Upstream, error) {
	if item.Name == "" {
		return nil, errors.Errorf("name required")
	}
	out, err := c.base.Create(&base.StorableItem{Upstream: item})
	if err != nil {
		return nil, err
	}
	return out.Upstream, nil
}

func (c *upstreamsClient) Update(item *v1.Upstream) (*v1.Upstream, error) {
	if item.Name == "" {
		return nil, errors.Errorf("name required")
	}
	out, err := c.base.Update(&base.StorableItem{Upstream: item})
	if err != nil {
		return nil, err
	}
	return out.Upstream, nil
}

func (c *upstreamsClient) Delete(name string) error {
	return c.base.Delete(name)
}

func (c *upstreamsClient) Get(name string) (*v1.Upstream, error) {
	out, err := c.base.Get(name)
	if err != nil {
		return nil, err
	}
	return out.Upstream, nil
}

func (c *upstreamsClient) List() ([]*v1.Upstream, error) {
	list, err := c.base.List()
	if err != nil {
		return nil, err
	}
	var upstreams []*v1.Upstream
	for _, obj := range list {
		upstreams = append(upstreams, obj.Upstream)
	}
	return upstreams, nil
}

func (c *upstreamsClient) Watch(handlers ...storage.UpstreamEventHandler) (*storage.Watcher, error) {
	var baseHandlers []base.StorableItemEventHandler
	for _, h := range handlers {
		baseHandlers = append(baseHandlers, base.StorableItemEventHandler{UpstreamEventHandler: h})
	}
	return c.base.Watch(baseHandlers...)
}
