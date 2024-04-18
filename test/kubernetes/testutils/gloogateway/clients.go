package gloogateway

import (
	"context"

	"github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
)

// ResourceClients is a set of clients for interacting with the Edge resources
type ResourceClients interface {
	RouteOptionClient() gatewayv1.RouteOptionClient
}

type clients struct {
	routeOptionClient gatewayv1.RouteOptionClient
}

func NewResourceClients(ctx context.Context, clusterCtx *cluster.Context) ResourceClients {
	sharedClientCache := kube.NewKubeCache(ctx)

	routeOptionClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gatewayv1.RouteOptionCrd,
		Cfg:         clusterCtx.RestConfig,
		SharedCache: sharedClientCache,
	}
	routeOptionClient, err := gatewayv1.NewRouteOptionClient(ctx, routeOptionClientFactory)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return &clients{
		routeOptionClient: routeOptionClient,
	}
}

func (c *clients) RouteOptionClient() gatewayv1.RouteOptionClient {
	return c.routeOptionClient
}
