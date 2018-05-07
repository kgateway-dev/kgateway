package consul

import (
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/base"
)

type reportsClient struct {
	base *base.ConsulStorageClient
}

func (c *reportsClient) Create(item *v1.Report) (*v1.Report, error) {
	out, err := c.base.Create(&base.StorableItem{Report: item})
	if err != nil {
		return nil, err
	}
	return out.Report, nil
}

func (c *reportsClient) Update(item *v1.Report) (*v1.Report, error) {
	out, err := c.base.Update(&base.StorableItem{Report: item})
	if err != nil {
		return nil, err
	}
	return out.Report, nil
}

func (c *reportsClient) Delete(name string) error {
	return c.base.Delete(name)
}

func (c *reportsClient) Get(name string) (*v1.Report, error) {
	out, err := c.base.Get(name)
	if err != nil {
		return nil, err
	}
	return out.Report, nil
}

func (c *reportsClient) List() ([]*v1.Report, error) {
	list, err := c.base.List()
	if err != nil {
		return nil, err
	}
	var reports []*v1.Report
	for _, obj := range list {
		reports = append(reports, obj.Report)
	}
	return reports, nil
}

func (c *reportsClient) Watch(handlers ...storage.ReportEventHandler) (*storage.Watcher, error) {
	var baseHandlers []base.StorableItemEventHandler
	for _, h := range handlers {
		baseHandlers = append(baseHandlers, base.StorableItemEventHandler{ReportEventHandler: h})
	}
	return c.base.Watch(baseHandlers...)
}
