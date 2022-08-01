package runner

import (
	"strconv"
	"strings"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/statusutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
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
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"k8s.io/client-go/kubernetes"

	clusteringressclient "github.com/solo-io/gloo/projects/clusteringress/pkg/api/custom/knative"
	clusteringressv1alpha1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/external/knative"
	clusteringressv1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/v1"
	clusteringresstranslator "github.com/solo-io/gloo/projects/clusteringress/pkg/translator"
	knativeclientset "knative.dev/networking/pkg/client/clientset/versioned"
)

type RunOpts struct {
	ClusterIngressProxyAddress  string
	KnativeExternalProxyAddress string
	KnativeInternalProxyAddress string
	WriteNamespace              string
	StatusReporterNamespace     string
	WatchNamespaces             []string
	Proxies                     factory.ResourceClientFactory
	Upstreams                   factory.ResourceClientFactory
	Secrets                     factory.ResourceClientFactory
	WatchOpts                   clients.WatchOpts
	EnableKnative               bool
	KnativeVersion              string
	DisableKubeIngress          bool
	RequireIngressClass         bool
	CustomIngressClass          string
	IngressProxyLabel           string
}

func RunIngress(opts RunOpts) error {
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
