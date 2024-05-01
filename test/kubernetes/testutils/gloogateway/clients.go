package gloogateway

import (
	"context"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
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
	SettingsClient() gloov1.SettingsClient
}

type clients struct {
	routeOptionClient gatewayv1.RouteOptionClient
	serviceClient     skkube.ServiceClient
	settingsClient    gloov1.SettingsClient
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

	settingsClient, err := gloov1.NewSettingsClient(ctx, &factory.KubeResourceClientFactory{
		Crd:         gloov1.SettingsCrd,
		Cfg:         clusterCtx.RestConfig,
		SharedCache: sharedClientCache,
	})
	if err != nil {
		return nil, err
	}

	return &clients{
		routeOptionClient: routeOptionClient,
		serviceClient:     serviceClient,
		settingsClient:    settingsClient,
	}, nil
}

func (c *clients) RouteOptionClient() gatewayv1.RouteOptionClient {
	return c.routeOptionClient
}

func (c *clients) ServiceClient() skkube.ServiceClient {
	return c.serviceClient
}

func (c *clients) SettingsClient() gloov1.SettingsClient {
	return c.settingsClient
}
