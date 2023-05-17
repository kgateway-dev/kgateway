package kube2e

import (
	"context"

	kubeconverters "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"

	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	externalrl "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/service"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	kubecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var _ helpers.ResourceClientSet = new(KubeResourceClientSet)

// KubeResourceClientSet is a set of ResourceClients
type KubeResourceClientSet struct {
	gatewayClient           gatewayv1.GatewayClient
	httpGatewayClient       gatewayv1.MatchableHttpGatewayClient
	tcpGatewayClient        gatewayv1.MatchableTcpGatewayClient
	virtualServiceClient    gatewayv1.VirtualServiceClient
	routeTableClient        gatewayv1.RouteTableClient
	virtualHostOptionClient gatewayv1.VirtualHostOptionClient
	routeOptionClient       gatewayv1.RouteOptionClient
	upstreamGroupClient     gloov1.UpstreamGroupClient
	upstreamClient          gloov1.UpstreamClient
	proxyClient             gloov1.ProxyClient
	rateLimitConfigClient   externalrl.RateLimitConfigClient
	authConfigClient        extauthv1.AuthConfigClient
	serviceClient           skkube.ServiceClient
	settingsClient          gloov1.SettingsClient
	artifactClient          gloov1.ArtifactClient
	secretClient            gloov1.SecretClient

	kubeClient *kubernetes.Clientset
}

func NewKubeResourceClientSet(ctx context.Context, cfg *rest.Config) (*KubeResourceClientSet, error) {
	resourceClientSet := &KubeResourceClientSet{}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	resourceClientSet.kubeClient = kubeClient

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
	resourceClientSet.gatewayClient = gatewayClient

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
	resourceClientSet.httpGatewayClient = httpGatewayClient

	// TcpGateway
	tcpGatewayClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gatewayv1.MatchableTcpGatewayCrd,
		Cfg:         cfg,
		SharedCache: cache,
	}
	tcpGatewayClient, err := gatewayv1.NewMatchableTcpGatewayClient(ctx, tcpGatewayClientFactory)
	if err != nil {
		return nil, err
	}
	if err = tcpGatewayClient.Register(); err != nil {
		return nil, err
	}
	resourceClientSet.tcpGatewayClient = tcpGatewayClient

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
	resourceClientSet.virtualServiceClient = virtualServiceClient

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
	resourceClientSet.routeTableClient = routeTableClient

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
	resourceClientSet.upstreamGroupClient = upstreamGroupClient

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
	resourceClientSet.upstreamClient = upstreamClient

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
	resourceClientSet.proxyClient = proxyClient

	// RateLimitConfig
	rateLimitConfigClientFactory := &factory.KubeResourceClientFactory{
		Crd:         externalrl.RateLimitConfigCrd,
		Cfg:         cfg,
		SharedCache: cache,
	}
	rateLimitConfigClient, err := externalrl.NewRateLimitConfigClient(ctx, rateLimitConfigClientFactory)
	if err != nil {
		return nil, err
	}
	if err = rateLimitConfigClient.Register(); err != nil {
		return nil, err
	}
	resourceClientSet.rateLimitConfigClient = rateLimitConfigClient

	// AuthConfig
	authConfigClientFactory := &factory.KubeResourceClientFactory{
		Crd:         extauthv1.AuthConfigCrd,
		Cfg:         cfg,
		SharedCache: cache,
	}
	authConfigClient, err := extauthv1.NewAuthConfigClient(ctx, authConfigClientFactory)
	if err != nil {
		return nil, err
	}
	if err = authConfigClient.Register(); err != nil {
		return nil, err
	}
	resourceClientSet.authConfigClient = authConfigClient

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
	resourceClientSet.virtualHostOptionClient = virtualHostOptionClient

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
	resourceClientSet.routeOptionClient = routeOptionClient

	// Settings
	settingsClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gloov1.SettingsCrd,
		Cfg:         cfg,
		SharedCache: kube.NewKubeCache(ctx),
	}
	settingsClient, err := gloov1.NewSettingsClient(ctx, settingsClientFactory)
	if err != nil {
		return nil, err
	}
	if err = settingsClient.Register(); err != nil {
		return nil, err
	}
	resourceClientSet.settingsClient = settingsClient

	// Artifact
	// Mirror kube setup from: https://github.com/solo-io/gloo/blob/dc96c0cd0e4d93457e77a848d69a0d652488a92e/projects/gloo/pkg/bootstrap/utils.go#L216
	artifactClientFactory := &factory.KubeConfigMapClientFactory{
		Clientset:       kubeClient,
		Cache:           kubeCoreCache,
		CustomConverter: kubeconverters.NewArtifactConverter(),
	}
	artifactClient, err := gloov1.NewArtifactClient(ctx, artifactClientFactory)
	if err != nil {
		return nil, err
	}
	if err = artifactClient.Register(); err != nil {
		return nil, err
	}
	resourceClientSet.artifactClient = artifactClient

	// Secret
	// Mirror kube setup from: https://github.com/solo-io/gloo/blob/dc96c0cd0e4d93457e77a848d69a0d652488a92e/projects/gloo/pkg/bootstrap/utils.go#L170
	secretClientFactory := &factory.KubeSecretClientFactory{
		Clientset:       kubeClient,
		Cache:           kubeCoreCache,
		SecretConverter: kubeconverters.GlooSecretConverterChain,
	}
	secretClient, err := gloov1.NewSecretClient(ctx, secretClientFactory)
	if err != nil {
		return nil, err
	}
	if err = secretClient.Register(); err != nil {
		return nil, err
	}
	resourceClientSet.secretClient = secretClient

	// Kube Service
	resourceClientSet.serviceClient = service.NewServiceClient(kubeClient, kubeCoreCache)

	return resourceClientSet, nil
}

func (k KubeResourceClientSet) GatewayClient() gatewayv1.GatewayClient {
	return k.gatewayClient
}

func (k KubeResourceClientSet) HttpGatewayClient() gatewayv1.MatchableHttpGatewayClient {
	return k.httpGatewayClient
}

func (k KubeResourceClientSet) TcpGatewayClient() gatewayv1.MatchableTcpGatewayClient {
	return k.tcpGatewayClient
}

func (k KubeResourceClientSet) VirtualServiceClient() gatewayv1.VirtualServiceClient {
	return k.virtualServiceClient
}

func (k KubeResourceClientSet) RouteTableClient() gatewayv1.RouteTableClient {
	return k.routeTableClient
}

func (k KubeResourceClientSet) VirtualHostOptionClient() gatewayv1.VirtualHostOptionClient {
	return k.virtualHostOptionClient
}

func (k KubeResourceClientSet) RouteOptionClient() gatewayv1.RouteOptionClient {
	return k.routeOptionClient
}

func (k KubeResourceClientSet) UpstreamGroupClient() gloov1.UpstreamGroupClient {
	return k.upstreamGroupClient
}

func (k KubeResourceClientSet) UpstreamClient() gloov1.UpstreamClient {
	return k.upstreamClient
}

func (k KubeResourceClientSet) ProxyClient() gloov1.ProxyClient {
	return k.proxyClient
}

func (k KubeResourceClientSet) RateLimitConfigClient() externalrl.RateLimitConfigClient {
	return k.rateLimitConfigClient
}

func (k KubeResourceClientSet) AuthConfigClient() extauthv1.AuthConfigClient {
	return k.authConfigClient
}

func (k KubeResourceClientSet) SettingsClient() gloov1.SettingsClient {
	return k.settingsClient
}

func (k KubeResourceClientSet) SecretClient() gloov1.SecretClient {
	return k.secretClient
}

func (k KubeResourceClientSet) ArtifactClient() gloov1.ArtifactClient {
	return k.artifactClient
}

func (k KubeResourceClientSet) KubeClients() *kubernetes.Clientset {
	return k.kubeClient
}

func (k KubeResourceClientSet) ServiceClient() skkube.ServiceClient {
	return k.serviceClient
}
