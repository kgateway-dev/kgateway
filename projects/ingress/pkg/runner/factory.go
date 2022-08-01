package runner

import (
	"context"
	"os"

	bootstrap2 "github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/utils/statusutils"

	"github.com/golang/protobuf/ptypes"
	"github.com/solo-io/gloo/pkg/utils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	gloodefaults "github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"knative.dev/pkg/network"
)

var defaultClusterIngressProxyAddress = "clusteringress-proxy." + gloodefaults.GlooSystem + ".svc." + network.GetClusterDomainName()

var defaultKnativeExternalProxyAddress = "knative-external-proxy." + gloodefaults.GlooSystem + ".svc." + network.GetClusterDomainName()
var defaultKnativeInternalProxyAddress = "knative-internal-proxy." + gloodefaults.GlooSystem + ".svc." + network.GetClusterDomainName()

func IngressRunnerFactory(ctx context.Context, kubeCache kube.SharedCache, inMemoryCache memory.InMemoryResourceCache, settings *gloov1.Settings) (bootstrap2.RunFunc, error) {
	var (
		cfg           *rest.Config
		clientset     kubernetes.Interface
		kubeCoreCache cache.KubeCoreCache
	)

	params := bootstrap.NewConfigFactoryParams(
		settings,
		inMemoryCache,
		kubeCache,
		&cfg,
		nil, // no consul client for ingress controller
	)

	proxyFactory, err := bootstrap.ConfigFactoryForSettings(params, gloov1.ProxyCrd)
	if err != nil {
		return nil, err
	}

	upstreamFactory, err := bootstrap.ConfigFactoryForSettings(params, gloov1.UpstreamCrd)
	if err != nil {
		return nil, err
	}

	secretFactory, err := bootstrap.SecretFactoryForSettings(
		ctx,
		settings,
		inMemoryCache,
		&cfg,
		&clientset,
		&kubeCoreCache,
		nil, // ingress client does not support vault config
		gloov1.SecretCrd.Plural,
	)
	if err != nil {
		return nil, err
	}

	refreshRate, err := ptypes.Duration(settings.GetRefreshRate())
	if err != nil {
		return nil, err
	}

	writeNamespace := settings.GetDiscoveryNamespace()
	if writeNamespace == "" {
		writeNamespace = gloodefaults.GlooSystem
	}
	statusReporterNamespace := statusutils.GetStatusReporterNamespaceOrDefault(writeNamespace)

	watchNamespaces := utils.ProcessWatchNamespaces(settings.GetWatchNamespaces(), writeNamespace)

	envTrue := func(name string) bool {
		return os.Getenv(name) == "true" || os.Getenv(name) == "1"
	}

	disableKubeIngress := envTrue("DISABLE_KUBE_INGRESS")
	requireIngressClass := envTrue("REQUIRE_INGRESS_CLASS")
	enableKnative := envTrue("ENABLE_KNATIVE_INGRESS")
	customIngressClass := os.Getenv("CUSTOM_INGRESS_CLASS")
	knativeVersion := os.Getenv("KNATIVE_VERSION")
	ingressProxyLabel := os.Getenv("INGRESS_PROXY_LABEL")

	clusterIngressProxyAddress := defaultClusterIngressProxyAddress
	if settings.GetKnative() != nil && settings.GetKnative().GetClusterIngressProxyAddress() != "" {
		clusterIngressProxyAddress = settings.GetKnative().GetClusterIngressProxyAddress()
	}

	knativeExternalProxyAddress := defaultKnativeExternalProxyAddress
	if settings.GetKnative() != nil && settings.GetKnative().GetKnativeExternalProxyAddress() != "" {
		knativeExternalProxyAddress = settings.GetKnative().GetKnativeExternalProxyAddress()
	}

	knativeInternalProxyAddress := defaultKnativeInternalProxyAddress
	if settings.GetKnative() != nil && settings.GetKnative().GetKnativeInternalProxyAddress() != "" {
		knativeInternalProxyAddress = settings.GetKnative().GetKnativeInternalProxyAddress()
	}

	if len(ingressProxyLabel) == 0 {
		ingressProxyLabel = "ingress-proxy"
	}

	opts := RunOpts{
		ClusterIngressProxyAddress:  clusterIngressProxyAddress,
		KnativeExternalProxyAddress: knativeExternalProxyAddress,
		KnativeInternalProxyAddress: knativeInternalProxyAddress,
		WriteNamespace:              writeNamespace,
		StatusReporterNamespace:     statusReporterNamespace,
		WatchNamespaces:             watchNamespaces,
		Proxies:                     proxyFactory,
		Upstreams:                   upstreamFactory,
		Secrets:                     secretFactory,
		WatchOpts: clients.WatchOpts{
			Ctx:         ctx,
			RefreshRate: refreshRate,
		},
		EnableKnative:       enableKnative,
		KnativeVersion:      knativeVersion,
		DisableKubeIngress:  disableKubeIngress,
		RequireIngressClass: requireIngressClass,
		CustomIngressClass:  customIngressClass,
		IngressProxyLabel:   ingressProxyLabel,
	}

	ingressRunner := func() error {
		return RunIngress(opts)
	}
	return ingressRunner, nil
}
