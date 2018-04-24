package consul

import (
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/storage/base"

	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

type fileStorage struct {
	base *base.ConsulStorageClient
}

func NewFileStorage(cfg *api.Config, rootPath string, syncFrequency time.Duration) (dependencies.FileStorage, error) {
	cfg.WaitTime = syncFrequency

	// Get a new client
	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "creating consul client")
	}

	return &fileStorage{
		base: base.NewConsulStorageClient(rootPath+"/files", client),
	}, nil
}

func (c *fileStorage) Create(item *dependencies.File) (*dependencies.File, error) {
	out, err := c.base.Create(&base.StorableItem{File: item})
	if err != nil {
		return nil, err
	}
	return out.File, nil
}

func (c *fileStorage) Update(item *dependencies.File) (*dependencies.File, error) {
	out, err := c.base.Update(&base.StorableItem{File: item})
	if err != nil {
		return nil, err
	}
	return out.File, nil
}

func (c *fileStorage) Delete(name string) error {
	return c.base.Delete(name)
}

func (c *fileStorage) Get(name string) (*dependencies.File, error) {
	out, err := c.base.Get(name)
	if err != nil {
		return nil, err
	}
	return out.File, nil
}

func (c *fileStorage) List() ([]*dependencies.File, error) {
	list, err := c.base.List()
	if err != nil {
		return nil, err
	}
	var virtualServices []*dependencies.File
	for _, obj := range list {
		virtualServices = append(virtualServices, obj.File)
	}
	return virtualServices, nil
}

func (c *fileStorage) Watch(handlers ...dependencies.FileEventHandler) (*storage.Watcher, error) {
	var baseHandlers []base.StorableItemEventHandler
	for _, h := range handlers {
		baseHandlers = append(baseHandlers, base.StorableItemEventHandler{FileEventHandler: h})
	}
	return c.base.Watch(baseHandlers...)
}
