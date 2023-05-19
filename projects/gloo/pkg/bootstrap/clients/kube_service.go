package clients

import (
	"context"

	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/service"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

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
