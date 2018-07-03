package consul

import (
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/base"
)

type attributesClient struct {
	base *base.ConsulStorageClient
}

func (c *attributesClient) Create(item *v1.Attribute) (*v1.Attribute, error) {
	if item.Name == "" {
		return nil, errors.Errorf("name required")
	}
	out, err := c.base.Create(&base.StorableItem{Attribute: item})
	if err != nil {
		return nil, err
	}
	return out.Attribute, nil
}

func (c *attributesClient) Update(item *v1.Attribute) (*v1.Attribute, error) {
	if item.Name == "" {
		return nil, errors.Errorf("name required")
	}
	out, err := c.base.Update(&base.StorableItem{Attribute: item})
	if err != nil {
		return nil, err
	}
	return out.Attribute, nil
}

func (c *attributesClient) Delete(name string) error {
	return c.base.Delete(name)
}

func (c *attributesClient) Get(name string) (*v1.Attribute, error) {
	out, err := c.base.Get(name)
	if err != nil {
		return nil, err
	}
	return out.Attribute, nil
}

func (c *attributesClient) List() ([]*v1.Attribute, error) {
	list, err := c.base.List()
	if err != nil {
		return nil, err
	}
	var attributes []*v1.Attribute
	for _, obj := range list {
		attributes = append(attributes, obj.Attribute)
	}
	return attributes, nil
}

func (c *attributesClient) Watch(handlers ...storage.AttributeEventHandler) (*storage.Watcher, error) {
	var baseHandlers []base.StorableItemEventHandler
	for _, h := range handlers {
		baseHandlers = append(baseHandlers, base.StorableItemEventHandler{AttributeEventHandler: h})
	}
	return c.base.Watch(baseHandlers...)
}
