package helpers

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	kubeconverters "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"

	"github.com/hashicorp/consul/api"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/solo-io/gloo/pkg/listers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	apiexts "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes/fake"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewayv2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	fakeKubeClientset *fake.Clientset
	memResourceClient *factory.MemoryResourceClientFactory
	consulClient      *factory.ConsulResourceClientFactory
	vaultClient       *factory.VaultSecretClientFactory

	lock sync.Mutex
)

// iterates over all the factory overrides, returning the first non-nil
// mem > consul
// if none set, return nil (callers will default to Kube CRD)
func getConfigClientFactory() factory.ResourceClientFactory {
	lock.Lock()
	defer lock.Unlock()
	if memResourceClient != nil {
		return memResourceClient
	}
	if consulClient != nil {
		return consulClient
	}
	return nil
}

// iterates over all the factory overrides, returning the first non-nil
// mem > vault
// if none set, return nil (callers will default to Kube Secret)
func getSecretClientFactory() factory.ResourceClientFactory {
	lock.Lock()
	defer lock.Unlock()
	if memResourceClient != nil {
		return memResourceClient
	}
	if vaultClient != nil {
		return vaultClient
	}
	return nil
}

// wipes all the client helper overrides
func UseDefaultClients() {
	lock.Lock()
	defer lock.Unlock()
	fakeKubeClientset = nil
	memResourceClient = nil
	consulClient = nil
	vaultClient = nil
}

func UseMemoryClients() {
	lock.Lock()
	defer lock.Unlock()
	memResourceClient = &factory.MemoryResourceClientFactory{
		Cache: memory.NewInMemoryResourceCache(),
	}
	fakeKubeClientset = fake.NewSimpleClientset()
}

// only applies to Config and Artifact clients
func UseConsulClients(client *api.Client, rootKey string) {
	lock.Lock()
	defer lock.Unlock()
	consulClient = &factory.ConsulResourceClientFactory{
		Consul:  client,
		RootKey: rootKey,
	}
}

// only applies to secret clients
func UseVaultClients(client *vaultapi.Client, rootKey string) {
	lock.Lock()
	defer lock.Unlock()
	vaultClient = &factory.VaultSecretClientFactory{
		Vault:   client,
		RootKey: rootKey,
	}
}

func MustKubeClient() kubernetes.Interface {
	client, err := KubeClient()
	if err != nil {
		log.Fatalf("failed to create kube client: %v", err)
	}
	return client
}

func KubeClient() (kubernetes.Interface, error) {
	if fakeKubeClientset != nil {
		return fakeKubeClientset, nil
	}
	cfg, err := kubeutils.GetConfig("", os.Getenv("KUBECONFIG"))
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	return kubernetes.NewForConfig(cfg)
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
	customFactory := getConfigClientFactory()
	if customFactory != nil {
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

type namespaceLister struct{}

var _ listers.NamespaceLister = namespaceLister{}

func NewNamespaceLister() listers.NamespaceLister {
	return namespaceLister{}
}

func (namespaceLister) List() ([]string, error) {
	return GetNamespaces()
}

func MustUpstreamClient() v1.UpstreamClient {
	client, err := UpstreamClient()
	if err != nil {
		log.Fatalf("failed to create upstream client: %v", err)
	}
	return client
}

func UpstreamClient() (v1.UpstreamClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return v1.NewUpstreamClient(customFactory)
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

func MustUpstreamGroupClient() v1.UpstreamGroupClient {
	client, err := UpstreamGroupClient()
	if err != nil {
		log.Fatalf("failed to create upstream group client: %v", err)
	}
	return client
}

func UpstreamGroupClient() (v1.UpstreamGroupClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return v1.NewUpstreamGroupClient(customFactory)
	}

	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(context.TODO())
	upstreamGroupClient, err := v1.NewUpstreamGroupClient(&factory.KubeResourceClientFactory{
		Crd:         v1.UpstreamGroupCrd,
		Cfg:         cfg,
		SharedCache: cache,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating upstream groups client")
	}
	if err := upstreamGroupClient.Register(); err != nil {
		return nil, err
	}
	return upstreamGroupClient, nil
}

func MustProxyClient() v1.ProxyClient {
	client, err := ProxyClient()
	if err != nil {
		log.Fatalf("failed to create proxy client: %v", err)
	}
	return client
}

func ProxyClient() (v1.ProxyClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return v1.NewProxyClient(customFactory)
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

func MustGatewayV2Client() gatewayv2.GatewayClient {
	client, err := GatewayV2Client()
	if err != nil {
		log.Fatalf("failed to create gateway v2 client: %v", err)
	}
	return client
}

func GatewayV2Client() (gatewayv2.GatewayClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return gatewayv2.NewGatewayClient(customFactory)
	}

	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(context.TODO())
	gatewayClient, err := gatewayv2.NewGatewayClient(&factory.KubeResourceClientFactory{
		Crd:         gatewayv2.GatewayCrd,
		Cfg:         cfg,
		SharedCache: cache,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating gateway client")
	}
	if err := gatewayClient.Register(); err != nil {
		return nil, err
	}
	return gatewayClient, nil
}

func MustVirtualServiceClient() gatewayv1.VirtualServiceClient {
	client, err := VirtualServiceClient()
	if err != nil {
		log.Fatalf("failed to create virtualService client: %v", err)
	}
	return client
}

func VirtualServiceClient() (gatewayv1.VirtualServiceClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return gatewayv1.NewVirtualServiceClient(customFactory)
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

func MustRouteTableClient() gatewayv1.RouteTableClient {
	routeTableClient, err := RouteTableClient()
	if err != nil {
		log.Fatalf("failed to create routeTable client: %v", err)
	}
	return routeTableClient
}

func RouteTableClient() (gatewayv1.RouteTableClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return gatewayv1.NewRouteTableClient(customFactory)
	}

	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(context.TODO())
	routeTableClient, err := gatewayv1.NewRouteTableClient(&factory.KubeResourceClientFactory{
		Crd:         gatewayv1.RouteTableCrd,
		Cfg:         cfg,
		SharedCache: cache,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating routeTables client")
	}
	if err := routeTableClient.Register(); err != nil {
		return nil, err
	}
	return routeTableClient, nil
}

func MustSettingsClient() v1.SettingsClient {
	client, err := SettingsClient()
	if err != nil {
		log.Fatalf("failed to create settings client: %v", err)
	}
	return client
}

func SettingsClient() (v1.SettingsClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return v1.NewSettingsClient(customFactory)
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
	customFactory := getSecretClientFactory()
	if customFactory != nil {
		return v1.NewSecretClient(customFactory)
	}

	clientset, err := GetKubernetesClient()
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	coreCache, err := cache.NewKubeCoreCache(context.TODO(), clientset)
	if err != nil {
		return nil, err
	}

	converterChain := kubeconverters.NewSecretConverterChain(
		new(kubeconverters.TLSSecretConverter),
		new(kubeconverters.AwsSecretConverter),
	)

	secretClient, err := v1.NewSecretClient(&factory.KubeSecretClientFactory{
		Clientset:       clientset,
		Cache:           coreCache,
		SecretConverter: converterChain,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating Secrets client")
	}
	if err := secretClient.Register(); err != nil {
		return nil, err
	}
	return secretClient, nil
}

func GetKubernetesClient() (*kubernetes.Clientset, error) {
	return GetKubernetesClientWithTimeout(0)
}

func GetKubernetesClientWithTimeout(timeout time.Duration) (*kubernetes.Clientset, error) {
	config, err := getKubernetesConfig(timeout)
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

func MustApiExtsClient() apiexts.Interface {
	client, err := ApiExtsClient()
	if err != nil {
		log.Fatalf("failed to create api exts client: %v", err)
	}
	return client
}

func ApiExtsClient() (apiexts.Interface, error) {
	cfg, err := kubeutils.GetConfig("", os.Getenv("KUBECONFIG"))
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	return apiexts.NewForConfig(cfg)
}
