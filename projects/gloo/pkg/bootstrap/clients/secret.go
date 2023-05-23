package clients

import (
	"context"
	"path/filepath"

	vaultapi "github.com/hashicorp/vault/api"
	errors "github.com/rotisserie/eris"
	kubeconverters "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// SecretFactoryForSettings constructs a new ResourceClientFactory for Secrets
// using Kubernetes, Directory, or Vault.
// settings.SecretSource or sharedCache must be non-nil
func SecretFactoryForSettings(ctx context.Context,
	settings *v1.Settings,
	sharedCache memory.InMemoryResourceCache,
	cfg **rest.Config,
	clientset *kubernetes.Interface,
	kubeCoreCache *cache.KubeCoreCache,
	vaultClient *vaultapi.Client,
	pluralName string) (factory.ResourceClientFactory, error) {
	if settings.GetSecretSource() == nil {
		if sharedCache == nil {
			return nil, errors.Errorf("internal error: shared cache cannot be nil")
		}
		return &factory.MemoryResourceClientFactory{
			Cache: sharedCache,
		}, nil
	}

	switch source := settings.GetSecretSource().(type) {
	case *v1.Settings_KubernetesSecretSource:
		if err := initializeForKube(ctx, cfg, clientset, kubeCoreCache, settings.GetRefreshRate(), settings.GetWatchNamespaces()); err != nil {
			return nil, errors.Wrapf(err, "initializing kube cfg clientset and core cache")
		}
		return &factory.KubeSecretClientFactory{
			Clientset:       *clientset,
			Cache:           *kubeCoreCache,
			SecretConverter: kubeconverters.GlooSecretConverterChain,
		}, nil
	case *v1.Settings_VaultSecretSource:
		rootKey := source.VaultSecretSource.GetRootKey()
		if rootKey == "" {
			rootKey = DefaultRootKey
		}
		pathPrefix := source.VaultSecretSource.GetPathPrefix()
		if pathPrefix == "" {
			pathPrefix = DefaultPathPrefix
		}
		return NewVaultSecretClientFactory(vaultClient, pathPrefix, rootKey), nil
	case *v1.Settings_DirectorySecretSource:
		return &factory.FileResourceClientFactory{
			RootDir: filepath.Join(source.DirectorySecretSource.GetDirectory(), pluralName),
		}, nil
	}
	return nil, errors.Errorf("invalid config source type")
}
