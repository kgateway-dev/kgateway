package clients

import (
	"path/filepath"

	consulapi "github.com/hashicorp/consul/api"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"k8s.io/client-go/rest"
)

type (
	configFactoryParamsMemory struct {
		sharedCache memory.InMemoryResourceCache
	}
	configFactoryParamsKube struct {
		kubeCache kube.SharedCache
		restCfg   **rest.Config
	}
	configFactoryParamsConsul struct {
		consulClient *consulapi.Client
	}

	ConfigFactoryParams struct {
		settings *v1.Settings
		memory   configFactoryParamsMemory
		kube     configFactoryParamsKube
		consul   configFactoryParamsConsul
	}
)

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
			QueryOptions: DefaultConsulQueryOptions,
		}, nil
	case *v1.Settings_DirectoryConfigSource:
		return &factory.FileResourceClientFactory{
			RootDir: filepath.Join(source.DirectoryConfigSource.GetDirectory(), resourceCrd.Plural),
		}, nil
	}
	return nil, errors.Errorf("invalid config source type")
}
