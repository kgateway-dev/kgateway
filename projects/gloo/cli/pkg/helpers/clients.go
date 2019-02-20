package helpers

import (
	"context"
	"fmt"
	"time"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/utils/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var memoryResourceClient *factory.MemoryResourceClientFactory

func UseMemoryClients() {
	memoryResourceClient = &factory.MemoryResourceClientFactory{
		Cache: memory.NewInMemoryResourceCache(),
	}
}

func MustGetNamespaces() []string {
	ns, err := GetNamespaces()
	if err != nil {
		log.Fatalf("failed to list namespaces")
	}
	return ns
}

// Note: requires RBAC permission to list namespaces at the cluster level
func GetNamespaces() ([]string, error) {
	if memoryResourceClient != nil {
		return []string{"default", defaults.GlooSystem}, nil
	}

	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube client")
	}
	var namespaces []string
	nsList, err := kubeClient.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, ns := range nsList.Items {
		namespaces = append(namespaces, ns.Name)
	}
	return namespaces, nil
}

func MustUpstreamClient() v1.UpstreamClient {
	client, err := UpstreamClient()
	if err != nil {
		log.Fatalf("failed to create upstream client: %v", err)
	}
	return client
}

func UpstreamClient() (v1.UpstreamClient, error) {
	if memoryResourceClient != nil {
		return v1.NewUpstreamClient(memoryResourceClient)
	}

	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(context.TODO())
	upstreamClient, err := v1.NewUpstreamClient(&factory.KubeResourceClientFactory{
		Crd:         v1.UpstreamCrd,
		Cfg:         cfg,
		SharedCache: cache,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating upstreams client")
	}
	if err := upstreamClient.Register(); err != nil {
		return nil, err
	}
	return upstreamClient, nil
}

func MustProxyClient() v1.ProxyClient {
	client, err := ProxyClient()
	if err != nil {
		log.Fatalf("failed to create proxy client: %v", err)
	}
	return client
}

func ProxyClient() (v1.ProxyClient, error) {
	if memoryResourceClient != nil {
		return v1.NewProxyClient(memoryResourceClient)
	}

	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(context.TODO())
	proxyClient, err := v1.NewProxyClient(&factory.KubeResourceClientFactory{
		Crd:         v1.ProxyCrd,
		Cfg:         cfg,
		SharedCache: cache,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating proxys client")
	}
	if err := proxyClient.Register(); err != nil {
		return nil, err
	}
	return proxyClient, nil
}

func MustVirtualServiceClient() gatewayv1.VirtualServiceClient {
	client, err := VirtualServiceClient()
	if err != nil {
		log.Fatalf("failed to create virtualService client: %v", err)
	}
	return client
}

func VirtualServiceClient() (gatewayv1.VirtualServiceClient, error) {
	if memoryResourceClient != nil {
		return gatewayv1.NewVirtualServiceClient(memoryResourceClient)
	}

	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(context.TODO())
	virtualServiceClient, err := gatewayv1.NewVirtualServiceClient(&factory.KubeResourceClientFactory{
		Crd:         gatewayv1.VirtualServiceCrd,
		Cfg:         cfg,
		SharedCache: cache,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating virtualServices client")
	}
	if err := virtualServiceClient.Register(); err != nil {
		return nil, err
	}
	return virtualServiceClient, nil
}

func MustSettingsClient() v1.SettingsClient {
	client, err := SettingsClient()
	if err != nil {
		log.Fatalf("failed to create settings client: %v", err)
	}
	return client
}

func SettingsClient() (v1.SettingsClient, error) {
	if memoryResourceClient != nil {
		return v1.NewSettingsClient(memoryResourceClient)
	}

	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(context.TODO())
	settingsClient, err := v1.NewSettingsClient(&factory.KubeResourceClientFactory{
		Crd:         v1.SettingsCrd,
		Cfg:         cfg,
		SharedCache: cache,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating settings client")
	}
	if err := settingsClient.Register(); err != nil {
		return nil, err
	}
	return settingsClient, nil
}

func MustSecretClient() v1.SecretClient {
	client, err := secretClient()
	if err != nil {
		log.Fatalf("failed to create Secret client: %v", err)
	}
	return client
}

func secretClient() (v1.SecretClient, error) {
	if memoryResourceClient != nil {
		return v1.NewSecretClient(memoryResourceClient)
	}

	clientset, err := getKubernetesClient()
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	secretClient, err := v1.NewSecretClient(&factory.KubeSecretClientFactory{
		Clientset: clientset,
		Cache:     cache.NewKubeCoreCache(context.TODO(), clientset),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating Secrets client")
	}
	if err := secretClient.Register(); err != nil {
		return nil, err
	}
	return secretClient, nil
}

func getKubernetesClient() (*kubernetes.Clientset, error) {
	config, err := getKubernetesConfig(0)
	if err != nil {
		return nil, err
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return kubeClient, nil
}

func getKubernetesConfig(timeout time.Duration) (*rest.Config, error) {
	config, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, fmt.Errorf("Error retrieving Kubernetes configuration: %v \n", err)
	}
	config.Timeout = timeout
	return config, nil
}
