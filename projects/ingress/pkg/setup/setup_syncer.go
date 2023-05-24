package setup

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"

	"github.com/solo-io/gloo/pkg/utils/statusutils"

	"github.com/golang/protobuf/ptypes"
	"github.com/solo-io/gloo/pkg/utils"
	clusteringressclient "github.com/solo-io/gloo/projects/clusteringress/pkg/api/custom/knative"
	clusteringressv1alpha1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/external/knative"
	clusteringressv1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/v1"
	clusteringresstranslator "github.com/solo-io/gloo/projects/clusteringress/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	bootstrap "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"
	gloodefaults "github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/ingress/pkg/api/ingress"
	"github.com/solo-io/gloo/projects/ingress/pkg/api/service"
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/gloo/projects/ingress/pkg/status"
	"github.com/solo-io/gloo/projects/ingress/pkg/translator"
	knativeclient "github.com/solo-io/gloo/projects/knative/pkg/api/custom/knative"
	knativev1alpha1 "github.com/solo-io/gloo/projects/knative/pkg/api/external/knative"
	knativev1 "github.com/solo-io/gloo/projects/knative/pkg/api/v1"
	knativetranslator "github.com/solo-io/gloo/projects/knative/pkg/translator"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	knativeclientset "knative.dev/networking/pkg/client/clientset/versioned"
	"knative.dev/pkg/network"
)

var defaultClusterIngressProxyAddress = "clusteringress-proxy." + gloodefaults.GlooSystem + ".svc." + network.GetClusterDomainName()

var defaultKnativeExternalProxyAddress = "knative-external-proxy." + gloodefaults.GlooSystem + ".svc." + network.GetClusterDomainName()
var defaultKnativeInternalProxyAddress = "knative-internal-proxy." + gloodefaults.GlooSystem + ".svc." + network.GetClusterDomainName()

func Setup(ctx context.Context, kubeCache kube.SharedCache, inMemoryCache memory.InMemoryResourceCache, settings *gloov1.Settings, _ leaderelector.Identity) error {
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
		return err
	}

	upstreamFactory, err := bootstrap.ConfigFactoryForSettings(params, gloov1.UpstreamCrd)
	if err != nil {
		return err
	}

	secretFactory, err := bootstrap.SecretFactoryForSettingsWithRetry(
		ctx,
		bootstrap.SecretFactoryParams{
			Settings:           settings,
			SharedCache:        inMemoryCache,
			Cfg:                &cfg,
			Clientset:          &clientset,
			KubeCoreCache:      &kubeCoreCache,
			VaultClientInitMap: nil,
			PluralName:         gloov1.SecretCrd.Plural,
		})
	if err != nil {
		return err
	}

	refreshRate, err := ptypes.Duration(settings.GetRefreshRate())
	if err != nil {
		return err
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

	opts := Opts{
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

	return RunIngress(opts)
}

func RunIngress(opts Opts) error {
	opts.WatchOpts = opts.WatchOpts.WithDefaults()
	opts.WatchOpts.Ctx = contextutils.WithLogger(opts.WatchOpts.Ctx, "ingress")

	if opts.DisableKubeIngress && !opts.EnableKnative {
		return errors.Errorf("ingress controller must be enabled for either Knative (clusteringress) or " +
			"basic kubernetes ingress. set DISABLE_KUBE_INGRESS=0 or ENABLE_KNATIVE_INGRESS=1")
	}

	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return errors.Wrapf(err, "getting kube config")
	}

	proxyClient, err := gloov1.NewProxyClient(opts.WatchOpts.Ctx, opts.Proxies)
	if err != nil {
		return err
	}
	if err := proxyClient.Register(); err != nil {
		return err
	}
	writeErrs := make(chan error)

	if !opts.DisableKubeIngress {
		kube, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			return errors.Wrapf(err, "getting kube client")
		}

		upstreamClient, err := gloov1.NewUpstreamClient(opts.WatchOpts.Ctx, opts.Upstreams)
		if err != nil {
			return err
		}
		if err := upstreamClient.Register(); err != nil {
			return err
		}

		baseIngressClient := ingress.NewResourceClient(kube, &v1.Ingress{})
		ingressClient := v1.NewIngressClientWithBase(baseIngressClient)

		baseKubeServiceClient := service.NewResourceClient(kube, &v1.KubeService{})
		kubeServiceClient := v1.NewKubeServiceClientWithBase(baseKubeServiceClient)

		translatorEmitter := v1.NewTranslatorEmitter(upstreamClient, kubeServiceClient, ingressClient)
		statusClient := statusutils.GetStatusClientForNamespace(opts.StatusReporterNamespace)
		translatorSync := translator.NewSyncer(
			opts.WriteNamespace,
			proxyClient,
			ingressClient,
			writeErrs,
			opts.RequireIngressClass,
			opts.CustomIngressClass,
			statusClient)
		translatorEventLoop := v1.NewTranslatorEventLoop(translatorEmitter, translatorSync)
		translatorEventLoopErrs, err := translatorEventLoop.Run(opts.WatchNamespaces, opts.WatchOpts)
		if err != nil {
			return err
		}
		go errutils.AggregateErrs(opts.WatchOpts.Ctx, writeErrs, translatorEventLoopErrs, "ingress_translator_event_loop")

		// note (ilackarms): we must set the selector correctly here or the status syncer will not work
		// the selector should return exactly 1 service which is our <install-namespace>.ingress-proxy service
		ingressServiceClient := service.NewClientWithSelector(kubeServiceClient, map[string]string{
			"gloo": opts.IngressProxyLabel,
		})
		statusEmitter := v1.NewStatusEmitter(ingressServiceClient, ingressClient)
		statusSync := status.NewSyncer(ingressClient)
		statusEventLoop := v1.NewStatusEventLoop(statusEmitter, statusSync)
		statusEventLoopErrs, err := statusEventLoop.Run(opts.WatchNamespaces, opts.WatchOpts)
		if err != nil {
			return err
		}
		go errutils.AggregateErrs(opts.WatchOpts.Ctx, writeErrs, statusEventLoopErrs, "ingress_status_event_loop")
	}

	logger := contextutils.LoggerFrom(opts.WatchOpts.Ctx)

	if opts.EnableKnative {
		knative, err := knativeclientset.NewForConfig(cfg)
		if err != nil {
			return errors.Wrapf(err, "creating knative clientset")
		}

		// if the version of the target knative is < 0.8.0 (or version not provided), use clusteringress
		// else, use the new knative ingress object
		if pre080knativeVersion(opts.KnativeVersion) {
			logger.Infof("starting Ingress with KNative (ClusterIngress) support enabled")
			knativeCache, err := clusteringressclient.NewClusterIngreessCache(opts.WatchOpts.Ctx, knative)
			if err != nil {
				return errors.Wrapf(err, "creating knative cache")
			}
			baseClient := clusteringressclient.NewResourceClient(knative, knativeCache)
			ingressClient := clusteringressv1alpha1.NewClusterIngressClientWithBase(baseClient)
			clusterIngTranslatorEmitter := clusteringressv1.NewTranslatorEmitter(ingressClient)
			statusClient := statusutils.GetStatusClientForNamespace(opts.StatusReporterNamespace)
			clusterIngTranslatorSync := clusteringresstranslator.NewSyncer(
				opts.ClusterIngressProxyAddress,
				opts.WriteNamespace,
				proxyClient,
				knative.NetworkingV1alpha1(),
				statusClient,
				writeErrs,
			)
			clusterIngTranslatorEventLoop := clusteringressv1.NewTranslatorEventLoop(clusterIngTranslatorEmitter, clusterIngTranslatorSync)
			clusterIngTranslatorEventLoopErrs, err := clusterIngTranslatorEventLoop.Run(opts.WatchNamespaces, opts.WatchOpts)
			if err != nil {
				return err
			}
			go errutils.AggregateErrs(opts.WatchOpts.Ctx, writeErrs, clusterIngTranslatorEventLoopErrs, "cluster_ingress_translator_event_loop")
		} else {
			logger.Infof("starting Ingress with KNative (Ingress) support enabled")
			knativeCache, err := knativeclient.NewIngressCache(opts.WatchOpts.Ctx, knative)
			if err != nil {
				return errors.Wrapf(err, "creating knative cache")
			}
			baseClient := knativeclient.NewResourceClient(knative, knativeCache)
			ingressClient := knativev1alpha1.NewIngressClientWithBase(baseClient)
			knativeTranslatorEmitter := knativev1.NewTranslatorEmitter(ingressClient)
			statusClient := statusutils.GetStatusClientForNamespace(opts.StatusReporterNamespace)
			knativeTranslatorSync := knativetranslator.NewSyncer(
				opts.KnativeExternalProxyAddress,
				opts.KnativeInternalProxyAddress,
				opts.WriteNamespace,
				proxyClient,
				knative.NetworkingV1alpha1(),
				writeErrs,
				opts.RequireIngressClass,
				statusClient,
			)
			knativeTranslatorEventLoop := knativev1.NewTranslatorEventLoop(knativeTranslatorEmitter, knativeTranslatorSync)
			knativeTranslatorEventLoopErrs, err := knativeTranslatorEventLoop.Run(opts.WatchNamespaces, opts.WatchOpts)
			if err != nil {
				return err
			}
			go errutils.AggregateErrs(opts.WatchOpts.Ctx, writeErrs, knativeTranslatorEventLoopErrs, "knative_ingress_translator_event_loop")
		}
	}

	go func() {
		for {
			select {
			case err := <-writeErrs:
				logger.Errorf("error: %v", err)
			case <-opts.WatchOpts.Ctx.Done():
				close(writeErrs)
				return
			}
		}
	}()
	return nil
}

// change this to set whether we default to assuming
// knative is pre-0.8.0 in the absence of a valid version parameter
const defaultPre080 = true

func pre080knativeVersion(version string) bool {
	// expected format: 0.8.0
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		// default case is true
		return defaultPre080
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return defaultPre080
	}
	if major > 0 {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return defaultPre080
	}
	if minor >= 8 {
		return false
	}
	return true
}
