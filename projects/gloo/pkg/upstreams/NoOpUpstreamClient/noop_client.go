package NoOpUpstreamClient

import (
	"context"
	"fmt"
	"reflect"

	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
)

const notImplementedErrMsg = "this operation is not supported by this client"

type NoOpUpstreamClient struct {
	clientMap map[string]v1.UpstreamClient
}

func (c *NoOpUpstreamClient) NewResource() resources.Resource {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return &v1.Upstream{
		Metadata: &core.Metadata{
			Name:      "dev-error-placeholder",
			Namespace: utils.GetPodNamespace(),
		},
	}
}

func (c *NoOpUpstreamClient) Kind() string {
	return reflect.TypeOf(&v1.Upstream{}).String()
}

func (c *NoOpUpstreamClient) Register() error {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return fmt.Errorf(notImplementedErrMsg)
}

func (c *NoOpUpstreamClient) Read(namespace, name string, opts clients.ReadOpts) (resources.Resource, error) {
	return nil, fmt.Errorf(notImplementedErrMsg)
}

func (c *NoOpUpstreamClient) Write(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	return nil, fmt.Errorf(notImplementedErrMsg)
}

func (c *NoOpUpstreamClient) Delete(namespace, name string, opts clients.DeleteOpts) error {
	return fmt.Errorf(notImplementedErrMsg)
}

func (rc *NoOpUpstreamClient) ApplyStatus(statusClient resources.StatusClient, inputResource resources.InputResource, opts clients.ApplyStatusOpts) (resources.Resource, error) {
	return nil, fmt.Errorf(notImplementedErrMsg)
}

func (c *NoOpUpstreamClient) List(namespace string, opts clients.ListOpts) (resources.ResourceList, error) {
	return nil, fmt.Errorf(notImplementedErrMsg)
}

func (c *NoOpUpstreamClient) Watch(namespace string, opts clients.WatchOpts) (<-chan resources.ResourceList, <-chan error, error) {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return nil, nil, fmt.Errorf(notImplementedErrMsg)
}
