package gloogateway

import (
	"context"
	"github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
)

// Clientset is a set of clients for interacting with the Edge resources
type Clientset interface {
	RouteOptionClient() gatewayv1.RouteOptionClient
}

type clientsetImpl struct {
	routeOptionClient gatewayv1.RouteOptionClient
}

func NewClientset(ctx context.Context, clusterCtx *cluster.Context) Clientset {
	sharedClientCache := kube.NewKubeCache(ctx)

	routeOptionClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gatewayv1.RouteOptionCrd,
		Cfg:         clusterCtx.RestConfig,
		SharedCache: sharedClientCache,
	}
	routeOptionClient, err := gatewayv1.NewRouteOptionClient(ctx, routeOptionClientFactory)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return &clientsetImpl{
		routeOptionClient: routeOptionClient,
	}
}

func (c *clientsetImpl) RouteOptionClient() gatewayv1.RouteOptionClient {
	return c.routeOptionClient
}
