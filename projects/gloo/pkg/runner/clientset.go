package runner

import (
	"context"

	vaultapi "github.com/hashicorp/vault/api"
	errors "github.com/rotisserie/eris"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	ratelimitv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// GenerateGlooClientsets returns the set of clients used to power the Gloo component
func GenerateGlooClientsets(ctx context.Context, settings *gloov1.Settings, kubeCache kube.SharedCache, memCache memory.InMemoryResourceCache) (ResourceClientset, TypedClientset, error) {
	var (
		cfg           *rest.Config
		kubeCoreCache corecache.KubeCoreCache
		kubeClient    kubernetes.Interface

		// Wrapper types for Gloo Edge
		typedClientset    TypedClientset
		resourceClientset ResourceClientset
	)

	failedToConstruct := func(err error) (ResourceClientset, TypedClientset, error) {
		return resourceClientset, typedClientset, err
	}

	consulClient, err := bootstrap.ConsulClientForSettings(ctx, settings)
	if err != nil {
		return failedToConstruct(err)
	}

	// if vault service discovery specified, initialize consul watcher
	var consulWatcher consul.ConsulWatcher
	if consulServiceDiscovery := settings.GetConsul().GetServiceDiscovery(); consulServiceDiscovery != nil {
		// Set up Consul client
		consulClientWrapper, err := consul.NewConsulWatcher(consulClient, consulServiceDiscovery.GetDataCenters())
		if err != nil {
			return failedToConstruct(err)
		}
		consulWatcher = consulClientWrapper
	}

	var vaultClient *vaultapi.Client
	if vaultSettings := settings.GetVaultSecretSource(); vaultSettings != nil {
		vaultClient, err = bootstrap.VaultClientForSettings(vaultSettings)
		if err != nil {
			return failedToConstruct(err)
		}
	}

	params := bootstrap.NewConfigFactoryParams(
		settings,
		memCache,
		kubeCache,
		&cfg,
		consulClient,
	)

	kubeServiceClient, err := bootstrap.KubeServiceClientForSettings(
		ctx,
		settings,
		memCache,
		&cfg,
		&kubeClient,
		&kubeCoreCache,
	)
	if err != nil {
		return failedToConstruct(err)
	}

	upstreamFactory, err := bootstrap.ConfigFactoryForSettings(params, gloov1.UpstreamCrd)
	if err != nil {
		return failedToConstruct(errors.Wrap(err, "creating config source from settings"))
	}

	var proxyFactory factory.ResourceClientFactory
	if settings.GetGateway().GetPersistProxySpec().GetValue() {
		proxyFactory, err = bootstrap.ConfigFactoryForSettings(params, gloov1.ProxyCrd)
		if err != nil {
			return failedToConstruct(err)
		}
	} else {
		proxyFactory = &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}
	}

	secretFactory, err := bootstrap.SecretFactoryForSettings(
		ctx,
		settings,
		memCache,
		&cfg,
		&kubeClient,
		&kubeCoreCache,
		vaultClient,
		gloov1.SecretCrd.Plural,
	)
	if err != nil {
		return failedToConstruct(err)
	}

	upstreamGroupFactory, err := bootstrap.ConfigFactoryForSettings(params, gloov1.UpstreamGroupCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	artifactFactory, err := bootstrap.ArtifactFactoryForSettings(
		ctx,
		settings,
		memCache,
		&cfg,
		&kubeClient,
		&kubeCoreCache,
		consulClient,
		gloov1.ArtifactCrd.Plural,
	)
	if err != nil {
		return failedToConstruct(err)
	}

	authConfigFactory, err := bootstrap.ConfigFactoryForSettings(params, extauthv1.AuthConfigCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	rateLimitConfigFactory, err := bootstrap.ConfigFactoryForSettings(params, ratelimitv1.RateLimitConfigCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	graphqlApiFactory, err := bootstrap.ConfigFactoryForSettings(params, v1beta1.GraphQLApiCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	virtualServiceFactory, err := bootstrap.ConfigFactoryForSettings(params, gatewayv1.VirtualServiceCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	routeTableFactory, err := bootstrap.ConfigFactoryForSettings(params, gatewayv1.RouteTableCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	virtualHostOptionFactory, err := bootstrap.ConfigFactoryForSettings(params, gatewayv1.VirtualHostOptionCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	routeOptionFactory, err := bootstrap.ConfigFactoryForSettings(params, gatewayv1.RouteOptionCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	gatewayFactory, err := bootstrap.ConfigFactoryForSettings(params, gatewayv1.GatewayCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	matchableHttpGatewayFactory, err := bootstrap.ConfigFactoryForSettings(params, gatewayv1.MatchableHttpGatewayCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	endpointsFactory := &factory.MemoryResourceClientFactory{
		Cache: memCache,
	}

	upstreamClient, err := gloov1.NewUpstreamClient(ctx, upstreamFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := upstreamClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	proxyClient, err := gloov1.NewProxyClient(ctx, proxyFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := proxyClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	upstreamGroupClient, err := gloov1.NewUpstreamGroupClient(ctx, upstreamGroupFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := upstreamGroupClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	endpointClient, err := gloov1.NewEndpointClient(ctx, endpointsFactory)
	if err != nil {
		return failedToConstruct(err)
	}

	secretClient, err := gloov1.NewSecretClient(ctx, secretFactory)
	if err != nil {
		return failedToConstruct(err)
	}

	artifactClient, err := gloov1.NewArtifactClient(ctx, artifactFactory)
	if err != nil {
		return failedToConstruct(err)
	}

	authConfigClient, err := extauthv1.NewAuthConfigClient(ctx, authConfigFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := authConfigClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	graphqlApiClient, err := v1beta1.NewGraphQLApiClient(ctx, graphqlApiFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := graphqlApiClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	rateLimitClient, rateLimitReporterClient, err := ratelimitv1.NewRateLimitClients(ctx, rateLimitConfigFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := rateLimitClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	virtualServiceClient, err := gatewayv1.NewVirtualServiceClient(ctx, virtualServiceFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := virtualServiceClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	routeTableClient, err := gatewayv1.NewRouteTableClient(ctx, routeTableFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := routeTableClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	gatewayClient, err := gatewayv1.NewGatewayClient(ctx, gatewayFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := gatewayClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	matchableHttpGatewayClient, err := gatewayv1.NewMatchableHttpGatewayClient(ctx, matchableHttpGatewayFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := matchableHttpGatewayClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	virtualHostOptionClient, err := gatewayv1.NewVirtualHostOptionClient(ctx, virtualHostOptionFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := virtualHostOptionClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	routeOptionClient, err := gatewayv1.NewRouteOptionClient(ctx, routeOptionFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := routeOptionClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	resourceClientset = ResourceClientset{
		// Gateway resources
		VirtualServices:       virtualServiceClient,
		RouteTables:           routeTableClient,
		Gateways:              gatewayClient,
		MatchableHttpGateways: matchableHttpGatewayClient,
		VirtualHostOptions:    virtualHostOptionClient,
		RouteOptions:          routeOptionClient,

		// Gloo resources
		Endpoints:      endpointClient,
		Upstreams:      upstreamClient,
		UpstreamGroups: upstreamGroupClient,
		Proxies:        proxyClient,
		Secrets:        secretClient,
		Artifacts:      artifactClient,

		// Gloo Enterprise resources
		AuthConfigs:       authConfigClient,
		RateLimitConfigs:  rateLimitClient,
		RateLimitReporter: rateLimitReporterClient,
		GraphQLApis:       graphqlApiClient,
	}

	typedClientset = TypedClientset{
		KubeClient:        kubeClient,
		KubeServiceClient: kubeServiceClient,
		KubeCoreCache:     kubeCoreCache,
		ConsulWatcher:     consulWatcher,
	}

	return resourceClientset, typedClientset, nil
}
