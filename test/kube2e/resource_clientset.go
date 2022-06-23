package kube2e

import (
	"context"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/service"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	kubecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// ResourceClientSet is a set of ResourceClients
type ResourceClientSet struct {
	GatewayClient           gatewayv1.GatewayClient
	HttpGatewayClient       gatewayv1.MatchableHttpGatewayClient
	VirtualServiceClient    gatewayv1.VirtualServiceClient
	RouteTableClient        gatewayv1.RouteTableClient
	VirtualHostOptionClient gatewayv1.VirtualHostOptionClient
	RouteOptionClient       gatewayv1.RouteOptionClient
	UpstreamGroupClient     gloov1.UpstreamGroupClient
	UpstreamClient          gloov1.UpstreamClient
	ProxyClient             gloov1.ProxyClient
	ServiceClient           skkube.ServiceClient
}

func NewKubeResourceClientSet(ctx context.Context, cfg *rest.Config) (*ResourceClientSet, error) {
	resourceClientSet := &ResourceClientSet{}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	cache := kube.NewKubeCache(ctx)
	kubeCoreCache, err := kubecache.NewKubeCoreCache(ctx, kubeClient)
	if err != nil {
		return nil, err
	}

	// Gateway
	gatewayClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gatewayv1.GatewayCrd,
		Cfg:         cfg,
		SharedCache: cache,
	}
	gatewayClient, err := gatewayv1.NewGatewayClient(ctx, gatewayClientFactory)
	if err != nil {
		return nil, err
	}
	if err = gatewayClient.Register(); err != nil {
		return nil, err
	}
	resourceClientSet.GatewayClient = gatewayClient

	// HttpGateway
	httpGatewayClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gatewayv1.MatchableHttpGatewayCrd,
		Cfg:         cfg,
		SharedCache: cache,
	}
	httpGatewayClient, err := gatewayv1.NewMatchableHttpGatewayClient(ctx, httpGatewayClientFactory)
	if err != nil {
		return nil, err
	}
	if err = httpGatewayClient.Register(); err != nil {
		return nil, err
	}
	resourceClientSet.HttpGatewayClient = httpGatewayClient

	// VirtualService
	virtualServiceClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gatewayv1.VirtualServiceCrd,
		Cfg:         cfg,
		SharedCache: cache,
	}
	virtualServiceClient, err := gatewayv1.NewVirtualServiceClient(ctx, virtualServiceClientFactory)
	if err != nil {
		return nil, err
	}
	if err = virtualServiceClient.Register(); err != nil {
		return nil, err
	}
	resourceClientSet.VirtualServiceClient = virtualServiceClient

	// RouteTable
	routeTableClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gatewayv1.RouteTableCrd,
		Cfg:         cfg,
		SharedCache: cache,
	}
	routeTableClient, err := gatewayv1.NewRouteTableClient(ctx, routeTableClientFactory)
	if err != nil {
		return nil, err
	}
	if err = routeTableClient.Register(); err != nil {
		return nil, err
	}
	resourceClientSet.RouteTableClient = routeTableClient

	// UpstreamGroup
	upstreamGroupClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gloov1.UpstreamGroupCrd,
		Cfg:         cfg,
		SharedCache: cache,
	}
	upstreamGroupClient, err := gloov1.NewUpstreamGroupClient(ctx, upstreamGroupClientFactory)
	if err != nil {
		return nil, err
	}
	if err = upstreamGroupClient.Register(); err != nil {
		return nil, err
	}
	resourceClientSet.UpstreamGroupClient = upstreamGroupClient

	// Upstream
	upstreamClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gloov1.UpstreamCrd,
		Cfg:         cfg,
		SharedCache: cache,
	}
	upstreamClient, err := gloov1.NewUpstreamClient(ctx, upstreamClientFactory)
	if err != nil {
		return nil, err
	}
	if err = upstreamClient.Register(); err != nil {
		return nil, err
	}
	resourceClientSet.UpstreamClient = upstreamClient

	// Proxy
	proxyClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gloov1.ProxyCrd,
		Cfg:         cfg,
		SharedCache: cache,
	}
	proxyClient, err := gloov1.NewProxyClient(ctx, proxyClientFactory)
	if err != nil {
		return nil, err
	}
	if err = proxyClient.Register(); err != nil {
		return nil, err
	}
	resourceClientSet.ProxyClient = proxyClient

	// VirtualHostOption
	virtualHostOptionClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gatewayv1.VirtualHostOptionCrd,
		Cfg:         cfg,
		SharedCache: cache,
	}
	virtualHostOptionClient, err := gatewayv1.NewVirtualHostOptionClient(ctx, virtualHostOptionClientFactory)
	if err != nil {
		return nil, err
	}
	if err = virtualHostOptionClient.Register(); err != nil {
		return nil, err
	}
	resourceClientSet.VirtualHostOptionClient = virtualHostOptionClient

	// RouteOption
	routeOptionClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gatewayv1.RouteOptionCrd,
		Cfg:         cfg,
		SharedCache: cache,
	}
	routeOptionClient, err := gatewayv1.NewRouteOptionClient(ctx, routeOptionClientFactory)
	if err != nil {
		return nil, err
	}
	if err = routeOptionClient.Register(); err != nil {
		return nil, err
	}
	resourceClientSet.RouteOptionClient = routeOptionClient

	// Kube Service
	resourceClientSet.ServiceClient = service.NewServiceClient(kubeClient, kubeCoreCache)

	return resourceClientSet, nil
}

func (r ResourceClientSet) WriteSnapshot(ctx context.Context, snapshot *gloosnapshot.ApiSnapshot) error {
	// TODO
	return nil
}

func (r ResourceClientSet) DeleteSnapshot(ctx context.Context, snapshot *gloosnapshot.ApiSnapshot) error {
	// TODO
	return nil
}
