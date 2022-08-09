package utils

import (
	"context"

	vaultapi "github.com/hashicorp/vault/api"
	errors "github.com/rotisserie/eris"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ResourceClientset struct {
	Upstreams   gloov1.UpstreamClient
	Secrets     gloov1.SecretClient
	GraphQLApis v1beta1.GraphQLApiClient
}

type TypedClientset struct {
	// Kubernetes clients
	KubeClient    kubernetes.Interface
	KubeCoreCache corecache.KubeCoreCache

	// Consul clients
	ConsulWatcher consul.ConsulWatcher
}

// GenerateDiscoveryClientsets returns the set of clients used to power the FDS component
func GenerateDiscoveryClientsets(ctx context.Context, settings *gloov1.Settings, kubeCache kube.SharedCache, memCache memory.InMemoryResourceCache) (ResourceClientset, TypedClientset, error) {

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

	upstreamFactory, err := bootstrap.ConfigFactoryForSettings(params, gloov1.UpstreamCrd)
	if err != nil {
		return failedToConstruct(errors.Wrap(err, "creating config source from settings"))
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

	graphqlApiFactory, err := bootstrap.ConfigFactoryForSettings(params, v1beta1.GraphQLApiCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	upstreamClient, err := gloov1.NewUpstreamClient(ctx, upstreamFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := upstreamClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	secretClient, err := gloov1.NewSecretClient(ctx, secretFactory)
	if err != nil {
		return failedToConstruct(err)
	}

	graphqlApiClient, err := v1beta1.NewGraphQLApiClient(ctx, graphqlApiFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := graphqlApiClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	resourceClientset = ResourceClientset{
		Upstreams:   upstreamClient,
		Secrets:     secretClient,
		GraphQLApis: graphqlApiClient,
	}

	typedClientset = TypedClientset{
		KubeClient:    kubeClient,
		KubeCoreCache: kubeCoreCache,
		ConsulWatcher: consulWatcher,
	}

	return resourceClientset, typedClientset, nil
}
