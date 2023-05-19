package clients

import (
	"context"
	"path/filepath"

	consulapi "github.com/hashicorp/consul/api"
	errors "github.com/rotisserie/eris"
	kubeconverters "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// sharedCach OR resourceCrd+cfg must be non-nil
func ArtifactFactoryForSettings(ctx context.Context,
	settings *v1.Settings,
	sharedCache memory.InMemoryResourceCache,
	cfg **rest.Config,
	clientset *kubernetes.Interface,
	kubeCoreCache *cache.KubeCoreCache,
	consulClient *consulapi.Client,
	pluralName string) (factory.ResourceClientFactory, error) {
	if settings.GetArtifactSource() == nil {
		if sharedCache == nil {
			return nil, errors.Errorf("internal error: shared cache cannot be nil")
		}
		return &factory.MemoryResourceClientFactory{
			Cache: sharedCache,
		}, nil
	}

	switch source := settings.GetArtifactSource().(type) {
	case *v1.Settings_KubernetesArtifactSource:
		if err := initializeForKube(ctx, cfg, clientset, kubeCoreCache, settings.GetRefreshRate(), settings.GetWatchNamespaces()); err != nil {
			return nil, errors.Wrapf(err, "initializing kube cfg clientset and core cache")
		}
		return &factory.KubeConfigMapClientFactory{
			Clientset:       *clientset,
			Cache:           *kubeCoreCache,
			CustomConverter: kubeconverters.NewArtifactConverter(),
		}, nil
	case *v1.Settings_DirectoryArtifactSource:
		return &factory.FileResourceClientFactory{
			RootDir: filepath.Join(source.DirectoryArtifactSource.GetDirectory(), pluralName),
		}, nil
	case *v1.Settings_ConsulKvArtifactSource:
		rootKey := source.ConsulKvArtifactSource.GetRootKey()
		if rootKey == "" {
			rootKey = DefaultRootKey
		}
		return &factory.ConsulResourceClientFactory{
			Consul:       consulClient,
			RootKey:      rootKey,
			QueryOptions: DefaultConsulQueryOptions,
		}, nil
	}
	return nil, errors.Errorf("invalid config source type")
}
