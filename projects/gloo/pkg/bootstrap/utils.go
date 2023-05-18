package bootstrap

import (
	"context"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"

	consulapi "github.com/hashicorp/consul/api"
	vaultapi "github.com/hashicorp/vault/api"
	kubeconverters "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/service"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	DefaultK8sQPS   = 50     // 10x the k8s-recommended default; gloo gets busy writing status updates
	DefaultK8sBurst = 100    // 10x the k8s-recommended default; gloo gets busy writing status updates
	DefaultRootKey  = "gloo" // used for vault and consul key-value storage
)

// default query options for consul
var DefaultQueryOptions = &consulapi.QueryOptions{RequireConsistent: true, AllowStale: false}

type ConfigFactoryParams struct {
	settings *v1.Settings
	memory   configFactoryParamsMemory
	kube     configFactoryParamsKube
	consul   configFactoryParamsConsul
}

func NewConfigFactoryParams(settings *v1.Settings,
	sharedCache memory.InMemoryResourceCache,
	cache kube.SharedCache,
	cfg **rest.Config,
	consulClient *consulapi.Client) ConfigFactoryParams {
	return ConfigFactoryParams{
		settings: settings,
		memory: configFactoryParamsMemory{
			sharedCache: sharedCache,
		},
		kube: configFactoryParamsKube{
			kubeCache: cache,
			restCfg:   cfg,
		},
		consul: configFactoryParamsConsul{
			consulClient: consulClient,
		},
	}
}

type configFactoryParamsMemory struct {
	sharedCache memory.InMemoryResourceCache
}

type configFactoryParamsKube struct {
	kubeCache kube.SharedCache
	restCfg   **rest.Config
}

type configFactoryParamsConsul struct {
	consulClient *consulapi.Client
}

// sharedCache, resourceCrd+cfg OR consulClient must be non-nil
func ConfigFactoryForSettings(params ConfigFactoryParams, resourceCrd crd.Crd) (factory.ResourceClientFactory, error) {
	settings := params.settings

	if settings.GetConfigSource() == nil {
		sharedCache := params.memory.sharedCache
		if sharedCache == nil {
			return nil, errors.Errorf("internal error: shared cache cannot be nil")
		}
		return &factory.MemoryResourceClientFactory{
			Cache: sharedCache,
		}, nil
	}

	switch source := settings.GetConfigSource().(type) {
	// this is at trick to reuse the same cfg across multiple clients
	case *v1.Settings_KubernetesConfigSource:
		kubeCache := params.kube.kubeCache
		cfg := params.kube.restCfg
		if *cfg == nil {
			c, err := kubeutils.GetConfig("", "")
			if err != nil {
				return nil, err
			}

			c.QPS = DefaultK8sQPS
			c.Burst = DefaultK8sBurst
			if kubeSettingsConfig := settings.GetKubernetes(); kubeSettingsConfig != nil {
				if rl := kubeSettingsConfig.GetRateLimits(); rl != nil {
					c.QPS = rl.GetQPS()
					c.Burst = int(rl.GetBurst())
				}
			}
			*cfg = c
		}

		return &factory.KubeResourceClientFactory{
			Crd:                resourceCrd,
			Cfg:                *cfg,
			SharedCache:        kubeCache,
			NamespaceWhitelist: settings.GetWatchNamespaces(),
		}, nil
	case *v1.Settings_ConsulKvSource:
		consulClient := params.consul.consulClient
		rootKey := source.ConsulKvSource.GetRootKey()
		if rootKey == "" {
			rootKey = DefaultRootKey
		}
		return &factory.ConsulResourceClientFactory{
			Consul:       consulClient,
			RootKey:      rootKey,
			QueryOptions: DefaultQueryOptions,
		}, nil
	case *v1.Settings_DirectoryConfigSource:
		return &factory.FileResourceClientFactory{
			RootDir: filepath.Join(source.DirectoryConfigSource.GetDirectory(), resourceCrd.Plural),
		}, nil
	}
	return nil, errors.Errorf("invalid config source type")
}

func KubeServiceClientForSettings(ctx context.Context,
	settings *v1.Settings,
	sharedCache memory.InMemoryResourceCache,
	cfg **rest.Config,
	clientset *kubernetes.Interface,
	kubeCoreCache *cache.KubeCoreCache) (skkube.ServiceClient, error) {

	// We are running in kubernetes
	switch settings.GetConfigSource().(type) {
	case *v1.Settings_KubernetesConfigSource:
		if err := initializeForKube(ctx, cfg, clientset, kubeCoreCache, settings.GetRefreshRate(), settings.GetWatchNamespaces()); err != nil {
			return nil, errors.Wrapf(err, "initializing kube cfg clientset and core cache")
		}
		return service.NewServiceClient(*clientset, *kubeCoreCache), nil
	}

	// In all other cases, run in memory
	if sharedCache == nil {
		return nil, errors.Errorf("internal error: shared cache cannot be nil")
	}
	memoryRcFactory := &factory.MemoryResourceClientFactory{Cache: sharedCache}
	inMemoryClient, err := memoryRcFactory.NewResourceClient(ctx, factory.NewResourceClientParams{
		ResourceType: &skkube.Service{},
	})
	if err != nil {
		return nil, err
	}
	return skkube.NewServiceClientWithBase(inMemoryClient), nil
}

// SecretFactoryForSettings creates a resource client factory for provided config.
// Implemented as multiSecretSourceResourceClient regardless if there is just one
// source configured.
// sharedCache OR resourceCrd+cfg must be non-nil
func SecretFactoryForSettings(ctx context.Context,
	settings *v1.Settings,
	sharedCache memory.InMemoryResourceCache,
	cfg **rest.Config,
	clientset *kubernetes.Interface,
	kubeCoreCache *cache.KubeCoreCache,
	vaultClient *vaultapi.Client,
	pluralName string) (factory.ResourceClientFactory, error) {

	if settings.GetSecretSource() == nil && settings.GetSecretOptions() == nil {
		if sharedCache == nil {
			return nil, errors.Errorf("internal error: shared cache cannot be nil")
		}
		return &factory.MemoryResourceClientFactory{
			Cache: sharedCache,
		}, nil
	}

	secretOpts := settings.GetSecretOptions()

	// Check for secretSource API config and translate it to secretOptions API config.
	// Log a warning and use new API if both are defined.
	if deprecatedApiSource := settings.GetSecretSource(); deprecatedApiSource != nil {
		if secretOpts != nil {
			contextutils.LoggerFrom(ctx).Warn("secretOptions API and deprecated secretSource API both configured; using secretOptions")
		} else {
			secretOpts = &v1.Settings_SecretOptions{
				SecretSources: []*v1.Settings_SecretOptions_Source{},
			}
			newApiSource := &v1.Settings_SecretOptions_Source{}
			switch src := deprecatedApiSource.(type) {
			case *v1.Settings_DirectorySecretSource:
				{
					newApiSource.Source = &v1.Settings_SecretOptions_Source_Directory{
						Directory: src.DirectorySecretSource,
					}
				}
			case *v1.Settings_VaultSecretSource:
				{
					newApiSource.Source = &v1.Settings_SecretOptions_Source_Vault{
						Vault: src.VaultSecretSource,
					}
				}
			case *v1.Settings_KubernetesSecretSource:
				{
					newApiSource.Source = &v1.Settings_SecretOptions_Source_Kubernetes{
						Kubernetes: src.KubernetesSecretSource,
					}
				}
			}
			secretOpts.SecretSources = []*v1.Settings_SecretOptions_Source{newApiSource}
		}
	}
	if secretOpts != nil {
		return NewMultiSecretSourceResourceClientFactory(secretOpts.GetSecretSources(), sharedCache, cfg, clientset, kubeCoreCache, vaultClient)
	}
	return nil, errors.Errorf("invalid config source type")
}

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
			QueryOptions: DefaultQueryOptions,
		}, nil
	}
	return nil, errors.Errorf("invalid config source type")
}

func initializeForKube(ctx context.Context,
	cfg **rest.Config,
	clientset *kubernetes.Interface,
	kubeCoreCache *cache.KubeCoreCache,
	refreshRate *duration.Duration,
	nsToWatch []string,
) error {
	if cfg == nil {
		c, err := kubeutils.GetConfig("", "")
		if err != nil {
			return err
		}
		*cfg = c
	}

	if *clientset == nil {
		cs, err := kubernetes.NewForConfig(*cfg)
		if err != nil {
			return err
		}
		*clientset = cs
	}

	if *kubeCoreCache == nil {
		duration := 12 * time.Hour
		if refreshRate != nil {
			duration = prototime.DurationFromProto(refreshRate)
		}
		coreCache, err := cache.NewKubeCoreCacheWithOptions(ctx, *clientset, duration, nsToWatch)
		if err != nil {
			return err
		}
		*kubeCoreCache = coreCache
	}

	return nil

}

func GetWriteNamespace(settings *v1.Settings) string {
	writeNamespace := settings.GetDiscoveryNamespace()
	if writeNamespace == "" {
		writeNamespace = defaults.GlooSystem
	}

	return writeNamespace
}
