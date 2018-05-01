package consul

import (
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/base"
)

type rolesClient struct {
	base *base.ConsulStorageClient
}

func (c *rolesClient) Create(item *v1.Role) (*v1.Role, error) {
	out, err := c.base.Create(&base.StorableItem{Role: item})
	if err != nil {
		return nil, err
	}
	return out.Role, nil
}

func (c *rolesClient) Update(item *v1.Role) (*v1.Role, error) {
	out, err := c.base.Update(&base.StorableItem{Role: item})
	if err != nil {
		return nil, err
	}
	return out.Role, nil
}

func (c *rolesClient) Delete(name string) error {
	return c.base.Delete(name)
}

func (c *rolesClient) Get(name string) (*v1.Role, error) {
	out, err := c.base.Get(name)
	if err != nil {
		return nil, err
	}
	return out.Role, nil
}

func (c *rolesClient) List() ([]*v1.Role, error) {
	list, err := c.base.List()
	if err != nil {
		return nil, err
	}
	var roles []*v1.Role
	for _, obj := range list {
		roles = append(roles, obj.Role)
	}
	return roles, nil
}

func (c *rolesClient) Watch(handlers ...storage.RoleEventHandler) (*storage.Watcher, error) {
	var baseHandlers []base.StorableItemEventHandler
	for _, h := range handlers {
		baseHandlers = append(baseHandlers, base.StorableItemEventHandler{RoleEventHandler: h})
	}
	return c.base.Watch(baseHandlers...)
}
