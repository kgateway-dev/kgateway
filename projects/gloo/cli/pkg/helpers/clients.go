package helpers

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options/contextoptions"

	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"

	v1alpha1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"

	kubeconverters "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"

	"github.com/hashicorp/consul/api"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/solo-io/gloo/pkg/listers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	apiexts "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes/fake"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	clientset         *kubernetes.Clientset
	fakeKubeClientset *fake.Clientset
	memResourceClient *factory.MemoryResourceClientFactory
	consulClient      *factory.ConsulResourceClientFactory
	vaultClient       factory.ResourceClientFactory

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
func UseConsulClients(client *api.Client, rootKey string, queryOptions *api.QueryOptions) {
	lock.Lock()
	defer lock.Unlock()
	consulClient = &factory.ConsulResourceClientFactory{
		Consul:       client,
		RootKey:      rootKey,
		QueryOptions: queryOptions,
	}
}

// only applies to secret clients
func UseVaultClients(client *vaultapi.Client, pathPrefix, rootKey string) {
	lock.Lock()
	defer lock.Unlock()
	vaultClient = clients.NewVaultSecretClientFactory(clients.NoopVaultClientInitFunc(client), pathPrefix, rootKey)
}

func MustKubeClient() kubernetes.Interface {
	return MustKubeClientWithKubecontext("")
}

// MustKubeClientWithKubecontext attempts to get a kubeclient given some string that denotes a way to retrieve kubecontext
// Not allowed to fail
func MustKubeClientWithKubecontext(kubecontext string) kubernetes.Interface {
	client, err := KubeClientWithKubecontext(kubecontext)
	if err != nil {
		log.Fatalf("failed to create kube client: %v", err)
	}
	return client
}

// KubeClient retrieves a kubeclient plausibly with some direction from env details
func KubeClient() (kubernetes.Interface, error) {
	return KubeClientWithKubecontext("")
}

// KubeClientWithKubecontext attempts to get a kubeclient given some string that denotes a way to retrieve kubecontext
func KubeClientWithKubecontext(kubecontext string) (kubernetes.Interface, error) {
	if fakeKubeClientset != nil {
		return fakeKubeClientset, nil
	}
	if clientset == nil {
		cfg, err := kubeutils.GetConfigWithContext("", os.Getenv("KUBECONFIG"), kubecontext)
		if err != nil {
			return nil, errors.Wrapf(err, "getting kube config")
		}
		client, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			return nil, errors.Wrapf(err, "creating clientset")
		}
		clientset = client
	}

	return clientset, nil
}

func MustGetNamespaces(ctx context.Context) []string {
	ns, err := GetNamespaces(ctx)
	if err != nil {
		log.Fatalf("failed to list namespaces")
	}
	return ns
}

// Note: requires RBAC permission to list namespaces at the cluster level
func GetNamespaces(ctx context.Context) ([]string, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return []string{"default", defaults.GlooSystem}, nil
	}

	kubecontext := contextoptions.KubecontextFrom(ctx)
	kubeClient, err := GetKubernetesClient(kubecontext)

	if err != nil {
		return nil, errors.Wrapf(err, "getting kube client")
	}
	var namespaces []string
	nsList, err := kubeClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
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

// this namespaceLister implementation requires all implementations to have a context input.
func (namespaceLister) List(ctx context.Context) ([]string, error) {
	return GetNamespaces(ctx)
}

type providedNamespaceLister struct {
	namespaces []string
}

func NewProvidedNamespaceLister(namespaces []string) listers.NamespaceLister {
	return providedNamespaceLister{namespaces: namespaces}
}

func (l providedNamespaceLister) List(ctx context.Context) ([]string, error) {
	return l.namespaces, nil
}

func MustUpstreamClient(ctx context.Context) v1.UpstreamClient {
	return MustNamespacedUpstreamClient(ctx, metav1.NamespaceAll) // will require cluster-scoped permissions
}

func MustNamespacedUpstreamClient(ctx context.Context, ns string) v1.UpstreamClient {
	return MustMultiNamespacedUpstreamClient(ctx, []string{ns})
}

func MustMultiNamespacedUpstreamClient(ctx context.Context, namespaces []string) v1.UpstreamClient {
	client, err := UpstreamClient(ctx, namespaces)
	if err != nil {
		log.Fatalf("failed to create upstream client: %v", err)
	}
	return client
}

// provide "" (metav1.NamespaceAll) to get a cluster-scoped upstream client
func UpstreamClient(ctx context.Context, namespaces []string) (v1.UpstreamClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return v1.NewUpstreamClient(ctx, customFactory)
	}

	kubecontext := contextoptions.KubecontextFrom(ctx)
	cfg, err := kubeutils.GetConfigWithContext("", "", kubecontext)
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(ctx)
	upstreamClient, err := v1.NewUpstreamClient(ctx, &factory.KubeResourceClientFactory{
		Crd:                v1.UpstreamCrd,
		Cfg:                cfg,
		SharedCache:        cache,
		NamespaceWhitelist: namespaces,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating upstreams client")
	}
	if err := upstreamClient.Register(); err != nil {
		return nil, err
	}
	return upstreamClient, nil
}

func MustUpstreamGroupClient(ctx context.Context) v1.UpstreamGroupClient {
	return MustNamespacedUpstreamGroupClient(ctx, metav1.NamespaceAll) // will require cluster-scoped permissions
}

func MustNamespacedUpstreamGroupClient(ctx context.Context, ns string) v1.UpstreamGroupClient {
	return MustMultiNamespacedUpstreamGroupClient(ctx, []string{ns})
}

func MustMultiNamespacedUpstreamGroupClient(ctx context.Context, namespaces []string) v1.UpstreamGroupClient {
	client, err := UpstreamGroupClient(ctx, namespaces)
	if err != nil {
		log.Fatalf("failed to create upstream group client: %v", err)
	}
	return client
}

// provide "" (metav1.NamespaceAll) to get a cluster-scoped upstream group client
func UpstreamGroupClient(ctx context.Context, namespaces []string) (v1.UpstreamGroupClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return v1.NewUpstreamGroupClient(ctx, customFactory)
	}

	kubecontext := contextoptions.KubecontextFrom(ctx)
	cfg, err := kubeutils.GetConfigWithContext("", "", kubecontext)
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(ctx)
	upstreamGroupClient, err := v1.NewUpstreamGroupClient(ctx, &factory.KubeResourceClientFactory{
		Crd:                v1.UpstreamGroupCrd,
		Cfg:                cfg,
		SharedCache:        cache,
		NamespaceWhitelist: namespaces,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating upstream groups client")
	}
	if err := upstreamGroupClient.Register(); err != nil {
		return nil, err
	}
	return upstreamGroupClient, nil
}

func MustProxyClient(ctx context.Context) v1.ProxyClient {
	return MustNamespacedProxyClient(ctx, metav1.NamespaceAll) // will require cluster-scoped permissions
}

func MustNamespacedProxyClient(ctx context.Context, ns string) v1.ProxyClient {
	return MustMultiNamespacedProxyClient(ctx, []string{ns})
}

func MustMultiNamespacedProxyClient(ctx context.Context, namespaces []string) v1.ProxyClient {
	client, err := ProxyClient(ctx, namespaces)
	if err != nil {
		log.Fatalf("failed to create proxy client: %v", err)
	}
	return client
}

// provide "" (metav1.NamespaceAll) to get a cluster-scoped proxy client
func ProxyClient(ctx context.Context, namespaces []string) (v1.ProxyClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return v1.NewProxyClient(ctx, customFactory)
	}
	kubecontext := contextoptions.KubecontextFrom(ctx)
	cfg, err := kubeutils.GetConfigWithContext("", "", kubecontext)
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(ctx)
	proxyClient, err := v1.NewProxyClient(ctx, &factory.KubeResourceClientFactory{
		Crd:                v1.ProxyCrd,
		Cfg:                cfg,
		SharedCache:        cache,
		NamespaceWhitelist: namespaces,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating proxys client")
	}
	if err := proxyClient.Register(); err != nil {
		return nil, err
	}
	return proxyClient, nil
}

func MustGatewayClient(ctx context.Context) gatewayv1.GatewayClient {
	return MustNamespacedGatewayClient(ctx, metav1.NamespaceAll) // will require cluster-scoped permissions
}

func MustNamespacedGatewayClient(ctx context.Context, ns string) gatewayv1.GatewayClient {
	return MustMultiNamespacedGatewayClient(ctx, []string{ns})
}

func MustMultiNamespacedGatewayClient(ctx context.Context, namespaces []string) gatewayv1.GatewayClient {
	client, err := GatewayClient(ctx, namespaces)
	if err != nil {
		log.Fatalf("failed to create gateway client: %v", err)
	}
	return client
}

// provide "" (metav1.NamespaceAll) to get a cluster-scoped gateway client
func GatewayClient(ctx context.Context, namespaces []string) (gatewayv1.GatewayClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return gatewayv1.NewGatewayClient(ctx, customFactory)
	}
	kubecontext := contextoptions.KubecontextFrom(ctx)

	cfg, err := kubeutils.GetConfigWithContext("", "", kubecontext)
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(ctx)
	gatewayClient, err := gatewayv1.NewGatewayClient(ctx, &factory.KubeResourceClientFactory{
		Crd:                gatewayv1.GatewayCrd,
		Cfg:                cfg,
		SharedCache:        cache,
		NamespaceWhitelist: namespaces,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating gateway client")
	}
	if err := gatewayClient.Register(); err != nil {
		return nil, err
	}
	return gatewayClient, nil
}

func MustVirtualServiceClient(ctx context.Context) gatewayv1.VirtualServiceClient {
	return MustNamespacedVirtualServiceClient(ctx, metav1.NamespaceAll) // will require cluster-scoped permissions
}

func MustNamespacedVirtualServiceClient(ctx context.Context, ns string) gatewayv1.VirtualServiceClient {
	return MustMultiNamespacedVirtualServiceClient(ctx, []string{ns})
}

func MustMultiNamespacedVirtualServiceClient(ctx context.Context, namespaces []string) gatewayv1.VirtualServiceClient {
	client, err := VirtualServiceClient(ctx, namespaces)
	if err != nil {
		log.Fatalf("failed to create virtualService client: %v", err)
	}
	return client
}

// provide "" (metav1.NamespaceAll) to get a cluster-scoped virtual service client
func VirtualServiceClient(ctx context.Context, namespaces []string) (gatewayv1.VirtualServiceClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return gatewayv1.NewVirtualServiceClient(ctx, customFactory)
	}
	kubecontext := contextoptions.KubecontextFrom(ctx)

	cfg, err := kubeutils.GetConfigWithContext("", "", kubecontext)
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(ctx)
	virtualServiceClient, err := gatewayv1.NewVirtualServiceClient(ctx, &factory.KubeResourceClientFactory{
		Crd:                gatewayv1.VirtualServiceCrd,
		Cfg:                cfg,
		SharedCache:        cache,
		NamespaceWhitelist: namespaces,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating virtualServices client")
	}
	if err := virtualServiceClient.Register(); err != nil {
		return nil, err
	}
	return virtualServiceClient, nil
}

func MustRouteTableClient(ctx context.Context) gatewayv1.RouteTableClient {
	return MustNamespacedRouteTableClient(ctx, metav1.NamespaceAll) // will require cluster-scoped permissions
}

func MustNamespacedRouteTableClient(ctx context.Context, ns string) gatewayv1.RouteTableClient {
	return MustMultiNamespacedRouteTableClient(ctx, []string{ns})
}

func MustMultiNamespacedRouteTableClient(ctx context.Context, namespaces []string) gatewayv1.RouteTableClient {
	client, err := RouteTableClient(ctx, namespaces)
	if err != nil {
		log.Fatalf("failed to create routeTable client: %v", err)
	}
	return client
}

// provide "" (metav1.NamespaceAll) to get a cluster-scoped route table client
func RouteTableClient(ctx context.Context, namespaces []string) (gatewayv1.RouteTableClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return gatewayv1.NewRouteTableClient(ctx, customFactory)
	}
	kubecontext := contextoptions.KubecontextFrom(ctx)

	cfg, err := kubeutils.GetConfigWithContext("", "", kubecontext)
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(ctx)
	routeTableClient, err := gatewayv1.NewRouteTableClient(ctx, &factory.KubeResourceClientFactory{
		Crd:                gatewayv1.RouteTableCrd,
		Cfg:                cfg,
		SharedCache:        cache,
		NamespaceWhitelist: namespaces,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating routeTables client")
	}
	if err := routeTableClient.Register(); err != nil {
		return nil, err
	}
	return routeTableClient, nil
}

func MustSettingsClient(ctx context.Context) v1.SettingsClient {
	return MustNamespacedSettingsClient(ctx, metav1.NamespaceAll) // will require cluster-scoped permissions
}

func MustNamespacedSettingsClient(ctx context.Context, ns string) v1.SettingsClient {
	return MustMultiNamespacedSettingsClient(ctx, []string{ns})
}

func MustMultiNamespacedSettingsClient(ctx context.Context, namespaces []string) v1.SettingsClient {
	client, err := SettingsClient(ctx, namespaces)
	if err != nil {
		log.Fatalf("failed to create settings client: %v", err)
	}
	return client
}

// provide "" (metav1.NamespaceAll) to get a cluster-scoped settings client
func SettingsClient(ctx context.Context, namespaces []string) (v1.SettingsClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return v1.NewSettingsClient(ctx, customFactory)
	}
	kubecontext := contextoptions.KubecontextFrom(ctx)

	cfg, err := kubeutils.GetConfigWithContext("", "", kubecontext)
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(ctx)
	settingsClient, err := v1.NewSettingsClient(ctx, &factory.KubeResourceClientFactory{
		Crd:                v1.SettingsCrd,
		Cfg:                cfg,
		SharedCache:        cache,
		NamespaceWhitelist: namespaces,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating settings client")
	}
	if err := settingsClient.Register(); err != nil {
		return nil, err
	}
	return settingsClient, nil
}

func MustSecretClient(ctx context.Context) v1.SecretClient {
	return MustSecretClientWithOptions(ctx, 0, nil)
}

func MustSecretClientWithOptions(ctx context.Context, timeout time.Duration, namespaces []string) v1.SecretClient {
	client, err := GetSecretClient(ctx, namespaces)
	if err != nil {
		log.Fatalf("failed to create Secret client: %v", err)
	}
	return client
}

func GetSecretClient(ctx context.Context, namespaces []string) (v1.SecretClient, error) {
	customFactory := getSecretClientFactory()
	if customFactory != nil {
		return v1.NewSecretClient(ctx, customFactory)
	}

	kubecontext := contextoptions.KubecontextFrom(ctx)
	clientset, err := GetKubernetesClient(kubecontext)
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	coreCache, err := cache.NewKubeCoreCacheWithOptions(ctx, clientset, 12*time.Hour, namespaces)
	if err != nil {
		return nil, err
	}

	secretClient, err := v1.NewSecretClient(ctx, &factory.KubeSecretClientFactory{
		Clientset:       clientset,
		Cache:           coreCache,
		SecretConverter: kubeconverters.GlooSecretConverterChain,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating Secrets client")
	}
	if err := secretClient.Register(); err != nil {
		return nil, err
	}
	return secretClient, nil
}

func GetKubernetesClient(kubecontext string) (kubernetes.Interface, error) {
	return GetKubernetesClientWithTimeout(0, kubecontext)
}

func GetKubernetesClientWithTimeout(timeout time.Duration, kubecontext string) (kubernetes.Interface, error) {
	if fakeKubeClientset != nil {
		return fakeKubeClientset, nil
	}
	config, err := getKubernetesConfig(timeout, kubecontext)
	if err != nil {
		return nil, err
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return kubeClient, nil
}

func getKubernetesConfig(timeout time.Duration, kubecontext string) (*rest.Config, error) {
	config, err := kubeutils.GetConfigWithContext("", "", kubecontext)
	if err != nil {
		return nil, fmt.Errorf("Error retrieving Kubernetes configuration: %v \n", err)
	}
	config.Timeout = timeout
	// The burst & QPS values are set at a higher number to enable the triggering of up to this many
	// requests so that the KubeCoreCache that is created does not throttle as it get resources for
	// each watched namespace.
	config.QPS = 50
	config.Burst = 100
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

func MustAuthConfigClient(ctx context.Context) extauth.AuthConfigClient {
	return MustNamespacedAuthConfigClient(ctx, metav1.NamespaceAll) // will require cluster-scoped permissions
}

func MustNamespacedAuthConfigClient(ctx context.Context, ns string) extauth.AuthConfigClient {
	return MustMultiNamespacedAuthConfigClient(ctx, []string{ns})
}

func MustMultiNamespacedAuthConfigClient(ctx context.Context, namespaces []string) extauth.AuthConfigClient {
	client, err := AuthConfigClient(ctx, namespaces)
	if err != nil {
		log.Fatalf("failed to create auth config client: %v", err)
	}
	return client
}

// provide "" (metav1.NamespaceAll) to get a cluster-scoped authConfig client
func AuthConfigClient(ctx context.Context, namespaces []string) (extauth.AuthConfigClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return extauth.NewAuthConfigClient(ctx, customFactory)
	}

	kubecontext := contextoptions.KubecontextFrom(ctx)
	cfg, err := kubeutils.GetConfigWithContext("", "", kubecontext)
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(ctx)
	authConfigClient, err := extauth.NewAuthConfigClient(ctx, &factory.KubeResourceClientFactory{
		Crd:                extauth.AuthConfigCrd,
		Cfg:                cfg,
		SharedCache:        cache,
		NamespaceWhitelist: namespaces,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating auth config client")
	}
	if err := authConfigClient.Register(); err != nil {
		return nil, err
	}
	return authConfigClient, nil
}

func MustNamespacedRateLimitConfigClient(ctx context.Context, ns string) v1alpha1.RateLimitConfigClient {
	return MustMultiNamespacedRateLimitConfigClient(ctx, []string{ns})
}

func MustMultiNamespacedRateLimitConfigClient(ctx context.Context, namespaces []string) v1alpha1.RateLimitConfigClient {
	client, err := RateLimitConfigClient(ctx, namespaces)
	if err != nil {
		log.Fatalf("failed to create rate limit config client: %v", err)
	}
	return client
}

// provide "" (metav1.NamespaceAll) to get a cluster-scoped client
func RateLimitConfigClient(ctx context.Context, namespaces []string) (v1alpha1.RateLimitConfigClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return v1alpha1.NewRateLimitConfigClient(ctx, customFactory)
	}
	kubecontext := contextoptions.KubecontextFrom(ctx)
	cfg, err := kubeutils.GetConfigWithContext("", "", kubecontext)
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	kubeCache := kube.NewKubeCache(ctx)
	rlConfigClient, err := v1alpha1.NewRateLimitConfigClient(ctx, &factory.KubeResourceClientFactory{
		Crd:                v1alpha1.RateLimitConfigCrd,
		Cfg:                cfg,
		SharedCache:        kubeCache,
		NamespaceWhitelist: namespaces,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating rate limit config client")
	}
	if err := rlConfigClient.Register(); err != nil {
		return nil, err
	}
	return rlConfigClient, nil
}

func MustVirtualHostOptionClient(ctx context.Context) gatewayv1.VirtualHostOptionClient {
	return MustNamespacedVirtualHostOptionClient(ctx, metav1.NamespaceAll) // will require cluster-scoped permissions
}

func MustNamespacedVirtualHostOptionClient(ctx context.Context, ns string) gatewayv1.VirtualHostOptionClient {
	return MustMultiNamespacedVirtualHostOptionClient(ctx, []string{ns})
}

func MustMultiNamespacedVirtualHostOptionClient(ctx context.Context, namespaces []string) gatewayv1.VirtualHostOptionClient {
	client, err := VirtualHostOptionClient(ctx, namespaces)
	if err != nil {
		log.Fatalf("failed to create VirtualHostOption client: %v", err)
	}
	return client
}

func VirtualHostOptionClient(ctx context.Context, namespaces []string) (gatewayv1.VirtualHostOptionClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return gatewayv1.NewVirtualHostOptionClient(ctx, customFactory)
	}

	kubecontext := contextoptions.KubecontextFrom(ctx)
	cfg, err := kubeutils.GetConfigWithContext("", "", kubecontext)
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(ctx)
	virtualHostOptClient, err := gatewayv1.NewVirtualHostOptionClient(ctx, &factory.KubeResourceClientFactory{
		Crd:                gatewayv1.VirtualHostOptionCrd,
		Cfg:                cfg,
		SharedCache:        cache,
		NamespaceWhitelist: namespaces,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating VirtualHostOption client")
	}
	if err := virtualHostOptClient.Register(); err != nil {
		return nil, err
	}
	return virtualHostOptClient, nil
}

func MustRouteOptionClient(ctx context.Context) gatewayv1.RouteOptionClient {
	return MustNamespacedRouteOptionClient(ctx, metav1.NamespaceAll) // will require cluster-scoped permissions
}

func MustNamespacedRouteOptionClient(ctx context.Context, ns string) gatewayv1.RouteOptionClient {
	return MustMultiNamespacedRouteOptionClient(ctx, []string{ns})
}

func MustMultiNamespacedRouteOptionClient(ctx context.Context, namespaces []string) gatewayv1.RouteOptionClient {
	client, err := RouteOptionClient(ctx, namespaces)
	if err != nil {
		log.Fatalf("failed to create RouteOption client: %v", err)
	}
	return client
}

func RouteOptionClient(ctx context.Context, namespaces []string) (gatewayv1.RouteOptionClient, error) {
	customFactory := getConfigClientFactory()
	if customFactory != nil {
		return gatewayv1.NewRouteOptionClient(ctx, customFactory)
	}

	kubecontext := contextoptions.KubecontextFrom(ctx)
	cfg, err := kubeutils.GetConfigWithContext("", "", kubecontext)
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	cache := kube.NewKubeCache(ctx)
	routeOptClient, err := gatewayv1.NewRouteOptionClient(ctx, &factory.KubeResourceClientFactory{
		Crd:                gatewayv1.RouteOptionCrd,
		Cfg:                cfg,
		SharedCache:        cache,
		NamespaceWhitelist: namespaces,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating RouteOption client")
	}
	if err := routeOptClient.Register(); err != nil {
		return nil, err
	}
	return routeOptClient, nil
}
