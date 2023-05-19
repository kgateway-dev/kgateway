package upstreams

import (
	"context"
	"fmt"

	"github.com/solo-io/gloo/pkg/utils"

	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
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
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return &v1.Upstream{
		Metadata: &core.Metadata{
			Name:      "dev-error-placeholder",
			Namespace: utils.GetPodNamespace(),
		},
	}
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

func (c *readOnlyUpstreamBaseClient) ApplyStatus(statusClient resources.StatusClient, inputResource resources.InputResource, opts clients.ApplyStatusOpts) (resources.Resource, error) {
	name := inputResource.GetMetadata().GetName()
	if isRealUpstream(name) {
		return c.rc.ApplyStatus(statusClient, inputResource, opts)
	}
	// not a real upstream, so we don't need to apply status
	return nil, nil
}

func (c *readOnlyUpstreamBaseClient) Delete(namespace, name string, opts clients.DeleteOpts) error {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return fmt.Errorf(notImplementedErrMsg)
}

func (c *readOnlyUpstreamBaseClient) List(namespace string, opts clients.ListOpts) (resources.ResourceList, error) {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return nil, fmt.Errorf(notImplementedErrMsg)
}

func (c *readOnlyUpstreamBaseClient) Watch(namespace string, opts clients.WatchOpts) (<-chan resources.ResourceList, <-chan error, error) {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return nil, nil, fmt.Errorf(notImplementedErrMsg)
}
