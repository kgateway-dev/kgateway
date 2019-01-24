package service

import (
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

// TODO (ilackarms): consider adding generators for these kind of clients to solo-kit

type ClientWithSelector struct {
	v1.KubeServiceClient
	Selector map[string]string
}

func NewClientWithSelector(kubeServiceClient v1.KubeServiceClient, selector map[string]string) v1.KubeServiceClient {
	return &ClientWithSelector{KubeServiceClient: kubeServiceClient, Selector: selector}
}

func (c *ClientWithSelector) Watch(namespace string, opts clients.WatchOpts) (<-chan v1.KubeServiceList, <-chan error, error) {
	// override selector
	opts.Selector = c.Selector
	return c.KubeServiceClient.Watch(namespace, opts)
}
