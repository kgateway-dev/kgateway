package upstreams

import (
	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
)

// Delegates all the function calls to the underlying client in case of real upstreams and does nothing in case of
// service-derived upstreams.
//
// NOTE: This is only to be used in reporters, which only call the Write function
type readOnlyUpstreamBaseClient struct {
	rc clients.ResourceClient
}

func NewReadOnlyBaseClient(rc clients.ResourceClient) *readOnlyUpstreamBaseClient {
	return &readOnlyUpstreamBaseClient{
		rc: rc,
	}
}

func (c *readOnlyUpstreamBaseClient) Kind() string {
	return c.rc.Kind()
}

func (c *readOnlyUpstreamBaseClient) NewResource() resources.Resource {
	return c.rc.NewResource()
}

func (c *readOnlyUpstreamBaseClient) Register() error {
	return nil
}

func (c *readOnlyUpstreamBaseClient) Read(namespace, name string, opts clients.ReadOpts) (resources.Resource, error) {
	if isRealUpstream(name) {
		return c.rc.Read(namespace, name, opts)
	}
	return nil, errors.Errorf("this client cannot read the given resource %s.%s", namespace, name)
}

// TODO(marco): this will not write reports but still log an info message. Find a way of avoiding it.
func (c *readOnlyUpstreamBaseClient) Write(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	if isRealUpstream(resource.GetMetadata().Name) {
		return c.rc.Write(resource, opts)
	}
	return resource, nil
}

func (c *readOnlyUpstreamBaseClient) Delete(namespace, name string, opts clients.DeleteOpts) error {
	if isRealUpstream(name) {
		return c.rc.Delete(namespace, name, opts)
	}
	return nil
}

func (c *readOnlyUpstreamBaseClient) List(namespace string, opts clients.ListOpts) (resources.ResourceList, error) {
	return c.rc.List(namespace, opts)
}

func (c *readOnlyUpstreamBaseClient) Watch(namespace string, opts clients.WatchOpts) (<-chan resources.ResourceList, <-chan error, error) {
	return c.rc.Watch(namespace, opts)
}
