package gloogateway

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"

	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/service"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
)

// ResourceClients is a set of clients for interacting with the Edge resources
type ResourceClients interface {
	RouteOptionClient() gatewayv1.RouteOptionClient
	VirtualHostOptionClient() gatewayv1.VirtualHostOptionClient
	ServiceClient() skkube.ServiceClient
	UpstreamClient() v1.UpstreamClient
}

type Clients struct {
	routeOptionClient       gatewayv1.RouteOptionClient
	serviceClient           skkube.ServiceClient
	upstreamClient          v1.UpstreamClient
	virtualHostOptionClient gatewayv1.VirtualHostOptionClient
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

	upstreamClientFactory := &factory.KubeResourceClientFactory{
		Crd:         v1.UpstreamCrd,
		Cfg:         clusterCtx.RestConfig,
		SharedCache: sharedClientCache,
	}
	upstreamClient, err := v1.NewUpstreamClient(ctx, upstreamClientFactory)
	if err != nil {
		return nil, err
	}

	kubeCoreCache, err := cache.NewKubeCoreCache(ctx, clusterCtx.Clientset)
	if err != nil {
		return nil, err
	}
	serviceClient := service.NewServiceClient(clusterCtx.Clientset, kubeCoreCache)

	virtualHostOptionClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gatewayv1.VirtualHostOptionCrd,
		Cfg:         clusterCtx.RestConfig,
		SharedCache: sharedClientCache,
	}
	virtualHostOptionClient, err := gatewayv1.NewVirtualHostOptionClient(ctx, virtualHostOptionClientFactory)
	if err != nil {
		return nil, err
	}

	return &Clients{
		routeOptionClient:       routeOptionClient,
		serviceClient:           serviceClient,
		upstreamClient:          upstreamClient,
		virtualHostOptionClient: virtualHostOptionClient,
	}, nil
}

func (c *Clients) RouteOptionClient() gatewayv1.RouteOptionClient {
	return c.routeOptionClient
}

func (c *Clients) VirtualHostOptionClient() gatewayv1.VirtualHostOptionClient {
	return c.virtualHostOptionClient
}

func (c *Clients) ServiceClient() skkube.ServiceClient {
	return c.serviceClient
}

func (c *Clients) UpstreamClient() v1.UpstreamClient {
	return c.upstreamClient
}
