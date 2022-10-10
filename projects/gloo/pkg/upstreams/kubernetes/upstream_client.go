package kubernetes

import (
	"context"
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/NoOpUpstreamClient"
	"github.com/solo-io/go-utils/contextutils"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	skclients "github.com/solo-io/solo-kit/pkg/api/v1/clients"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
)

// Contains invalid character so any accidental attempt to write to storage fails
const upstreamNamePrefix = "kube-svc:"

const notImplementedErrMsg = "this operation is not supported by this client"

func NewKubernetesUpstreamClient(serviceClient skkube.ServiceClient) v1.UpstreamClient {
	return &kubernetesUpstreamClient{serviceClient: serviceClient}
}

type kubernetesUpstreamClient struct {
	serviceClient skkube.ServiceClient
}

func (c *kubernetesUpstreamClient) BaseClient() skclients.ResourceClient {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return &NoOpUpstreamClient.NoOpUpstreamClient{}
}

func (c *kubernetesUpstreamClient) Register() error {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return fmt.Errorf(notImplementedErrMsg)
}

func (c *kubernetesUpstreamClient) Read(namespace, name string, opts skclients.ReadOpts) (*v1.Upstream, error) {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return nil, fmt.Errorf(notImplementedErrMsg)
}

func (c *kubernetesUpstreamClient) Write(resource *v1.Upstream, opts skclients.WriteOpts) (*v1.Upstream, error) {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return nil, fmt.Errorf(notImplementedErrMsg)
}

func (c *kubernetesUpstreamClient) Delete(namespace, name string, opts skclients.DeleteOpts) error {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return fmt.Errorf(notImplementedErrMsg)
}

func (c *kubernetesUpstreamClient) List(namespace string, opts skclients.ListOpts) (v1.UpstreamList, error) {
	services, err := c.serviceClient.List(namespace, opts)
	if err != nil {
		return nil, err
	}
	return KubeServicesToUpstreams(opts.Ctx, services), nil
}

func (c *kubernetesUpstreamClient) Watch(namespace string, opts skclients.WatchOpts) (<-chan v1.UpstreamList, <-chan error, error) {
	servicesChan, errChan, err := c.serviceClient.Watch(namespace, opts)
	if err != nil {
		return nil, nil, err
	}
	return transform(opts.Ctx, servicesChan), errChan, nil
}

func transform(ctx context.Context, src <-chan skkube.ServiceList) <-chan v1.UpstreamList {
	upstreams := make(chan v1.UpstreamList)

	go func() {
		for {
			select {
			case services, ok := <-src:
				if !ok {
					close(upstreams)
					return
				}
				select {
				case upstreams <- KubeServicesToUpstreams(ctx, services):
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return upstreams
}
