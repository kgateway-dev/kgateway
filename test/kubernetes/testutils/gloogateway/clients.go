package gloogateway

import (
	"context"

	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/service"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
)

// ResourceClients is a set of clients for interacting with the Edge resources
type ResourceClients interface {
	RouteOptionClient() gatewayv1.RouteOptionClient
	ServiceClient() skkube.ServiceClient
}

type clients struct {
	routeOptionClient gatewayv1.RouteOptionClient
	serviceClient     skkube.ServiceClient
}

func NewResourceClients(ctx context.Context, clusterCtx *cluster.Context) (ResourceClients, error) {
	sharedClientCache := kube.NewKubeCache(ctx)

	routeOptionClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gatewayv1.RouteOptionCrd,
		Cfg:         clusterCtx.RestConfig,
		SharedCache: sharedClientCache,
	}
	routeOptionClient, err := gatewayv1.NewRouteOptionClient(ctx, routeOptionClientFactory)
	if err != nil {
		return nil, err
	}

	kubeCoreCache, err := cache.NewKubeCoreCache(ctx, clusterCtx.Clientset)
	if err != nil {
		return nil, err
	}
	serviceClient := service.NewServiceClient(clusterCtx.Clientset, kubeCoreCache)

	return &clients{
		routeOptionClient: routeOptionClient,
		serviceClient:     serviceClient,
	}, nil
}

func (c *clients) RouteOptionClient() gatewayv1.RouteOptionClient {
	return c.routeOptionClient
}

func (c *clients) ServiceClient() skkube.ServiceClient {
	return c.serviceClient
}
