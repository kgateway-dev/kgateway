package setup

import (
	"context"
	"os"
	"strconv"
	"strings"

	"knative.dev/pkg/network"

	clusteringressclient "github.com/solo-io/gloo/projects/clusteringress/pkg/api/custom/knative"

	clusteringressv1alpha1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/external/knative"

	knativeclient "github.com/solo-io/gloo/projects/knative/pkg/api/custom/knative"
	knativev1alpha1 "github.com/solo-io/gloo/projects/knative/pkg/api/external/knative"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/utils"
	clusteringressv1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/v1"
	clusteringresstranslator "github.com/solo-io/gloo/projects/clusteringress/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	gloodefaults "github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/ingress/pkg/api/ingress"
	"github.com/solo-io/gloo/projects/ingress/pkg/api/service"
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/gloo/projects/ingress/pkg/status"
	"github.com/solo-io/gloo/projects/ingress/pkg/translator"
	knativev1 "github.com/solo-io/gloo/projects/knative/pkg/api/v1"
	knativetranslator "github.com/solo-io/gloo/projects/knative/pkg/translator"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	knativeclientset "knative.dev/serving/pkg/client/clientset/versioned"
)

var defaultClusterIngressProxyAddress = "clusteringress-proxy." + gloodefaults.GlooSystem + ".svc." + network.GetClusterDomainName()

var defaultKnativeExternalProxyAddress = "knative-external-proxy." + gloodefaults.GlooSystem + ".svc." + network.GetClusterDomainName()
var defaultKnativeInternalProxyAddress = "knative-internal-proxy." + gloodefaults.GlooSystem + ".svc." + network.GetClusterDomainName()

func Setup(ctx context.Context, kubeCache kube.SharedCache, inMemoryCache memory.InMemoryResourceCache, settings *gloov1.Settings) error {
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
		return err
	}

	refreshRate, err := types.DurationFromProto(settings.RefreshRate)
	if err != nil {
		return err
	}

	writeNamespace := settings.DiscoveryNamespace
	if writeNamespace == "" {
		writeNamespace = gloodefaults.GlooSystem
	}
	watchNamespaces := utils.ProcessWatchNamespaces(settings.WatchNamespaces, writeNamespace)

	envTrue := func(name string) bool {
		return os.Getenv(name) == "true" || os.Getenv(name) == "1"
	}

	disableKubeIngress := envTrue("DISABLE_KUBE_INGRESS")
	requireIngressClass := envTrue("REQUIRE_INGRESS_CLASS")
	enableKnative := envTrue("ENABLE_KNATIVE_INGRESS")
	customIngressClass := os.Getenv("CUSTOM_INGRESS_CLASS")
	knativeVersion := os.Getenv("KNATIVE_VERSION")

	clusterIngressProxyAddress := defaultClusterIngressProxyAddress
	if settings.Knative != nil && settings.Knative.ClusterIngressProxyAddress != "" {
		clusterIngressProxyAddress = settings.Knative.ClusterIngressProxyAddress
	}

	knativeExternalProxyAddress := defaultKnativeExternalProxyAddress
	if settings.Knative != nil && settings.Knative.KnativeExternalProxyAddress != "" {
		knativeExternalProxyAddress = settings.Knative.KnativeExternalProxyAddress
	}

	knativeInternalProxyAddress := defaultKnativeInternalProxyAddress
	if settings.Knative != nil && settings.Knative.KnativeInternalProxyAddress != "" {
		knativeInternalProxyAddress = settings.Knative.KnativeInternalProxyAddress
	}

	opts := Opts{
		ClusterIngressProxyAddress:  clusterIngressProxyAddress,
		KnativeExternalProxyAddress: knativeExternalProxyAddress,
		KnativeInternalProxyAddress: knativeInternalProxyAddress,
		WriteNamespace:              writeNamespace,
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

	proxyClient, err := gloov1.NewProxyClient(opts.Proxies)
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

		upstreamClient, err := gloov1.NewUpstreamClient(opts.Upstreams)
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
		translatorSync := translator.NewSyncer(opts.WriteNamespace, proxyClient, ingressClient, writeErrs, opts.RequireIngressClass, opts.CustomIngressClass)
		translatorEventLoop := v1.NewTranslatorEventLoop(translatorEmitter, translatorSync)
		translatorEventLoopErrs, err := translatorEventLoop.Run(opts.WatchNamespaces, opts.WatchOpts)
		if err != nil {
			return err
		}
		go errutils.AggregateErrs(opts.WatchOpts.Ctx, writeErrs, translatorEventLoopErrs, "ingress_translator_event_loop")

		// note (ilackarms): we must set the selector correctly here or the status syncer will not work
		// the selector should return exactly 1 service which is our <install-namespace>.ingress-proxy service
		// TODO (ilackarms): make the service labels configurable
		ingressServiceClient := service.NewClientWithSelector(kubeServiceClient, map[string]string{
			"gloo": "ingress-proxy",
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
			clusterIngTranslatorSync := clusteringresstranslator.NewSyncer(
				opts.ClusterIngressProxyAddress,
				opts.WriteNamespace,
				proxyClient,
				knative.NetworkingV1alpha1(),
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
			knativeTranslatorSync := knativetranslator.NewSyncer(
				opts.KnativeExternalProxyAddress,
				opts.KnativeInternalProxyAddress,
				opts.WriteNamespace,
				proxyClient,
				knative.NetworkingV1alpha1(),
				writeErrs,
				opts.RequireIngressClass,
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
