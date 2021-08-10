package upstreams

import (
	"github.com/rotisserie/eris"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
)

// This client implements only the `Kind` `Read` and `Write` functions and panics on all the other functions.
// It is meant to be used in the API event loop reporter, which calls only those two functions.
type readOnlyUpstreamBaseClient struct {
	rc clients.ResourceClient
}

func newHybridBaseClient(rc clients.ResourceClient) *readOnlyUpstreamBaseClient {
	return &readOnlyUpstreamBaseClient{
		rc: rc,
	}
}

func (c *readOnlyUpstreamBaseClient) Kind() string {
	return c.rc.Kind()
}

func (c *readOnlyUpstreamBaseClient) NewResource() resources.Resource {
	panic(notImplementedErrMsg)
}

func (c *readOnlyUpstreamBaseClient) Register() error {
	return nil
}

func (c *readOnlyUpstreamBaseClient) Read(namespace, name string, opts clients.ReadOpts) (resources.Resource, error) {
	if isRealUpstream(name) {
		return c.rc.Read(namespace, name, opts)
	}
	return nil, eris.New(notImplementedErrMsg)
}

// TODO(marco): this will not write reports but still log an info message. Find a way of avoiding it.
func (c *readOnlyUpstreamBaseClient) Write(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	if isRealUpstream(resource.GetMetadata().GetName()) {
		return c.rc.Write(resource, opts)
	}
	return resources.Clone(resource), nil
}

func (c *readOnlyUpstreamBaseClient) Delete(namespace, name string, opts clients.DeleteOpts) error {
	panic(notImplementedErrMsg)
}

func (c *readOnlyUpstreamBaseClient) List(namespace string, opts clients.ListOpts) (resources.ResourceList, error) {
	panic(notImplementedErrMsg)
}

func (c *readOnlyUpstreamBaseClient) Watch(namespace string, opts clients.WatchOpts) (<-chan resources.ResourceList, <-chan error, error) {
	panic(notImplementedErrMsg)
}
