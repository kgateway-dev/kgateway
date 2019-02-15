package setup

import (
	"context"
	"os"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"

	"github.com/gogo/protobuf/types"
	knativeclientset "github.com/knative/serving/pkg/client/clientset/versioned"
	"github.com/solo-io/gloo/projects/clusteringress/pkg/api/clusteringress"
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
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/utils/errutils"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func Setup(ctx context.Context, kubeCache kube.SharedCache, inMemoryCache memory.InMemoryResourceCache, settings *gloov1.Settings) error {
	var (
		cfg           *rest.Config
		clientset     kubernetes.Interface
		kubeCoreCache cache.KubeCoreCache
	)
	proxyFactory, err := bootstrap.ConfigFactoryForSettings(
		settings,
		inMemoryCache,
		kubeCache,
		gloov1.ProxyCrd,
		&cfg,
	)
	if err != nil {
		return err
	}

	upstreamFactory, err := bootstrap.ConfigFactoryForSettings(
		settings,
		inMemoryCache,
		kubeCache,
		gloov1.UpstreamCrd,
		&cfg,
	)
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
	watchNamespaces := settings.WatchNamespaces
	if len(watchNamespaces) == 0 {
		watchNamespaces, err = bootstrap.ListAllNamespaces(cfg)
		if err != nil {
			return err
		}
	}
	var writeNamespaceProvided bool
	for _, ns := range watchNamespaces {
		if ns == writeNamespace {
			writeNamespaceProvided = true
			break
		}
	}
	if !writeNamespaceProvided {
		watchNamespaces = append(watchNamespaces, writeNamespace)
	}
	disableKubeIngress := os.Getenv("DISABLE_KUBE_INGRESS") == "true" || os.Getenv("DISABLE_KUBE_INGRESS") == "1"
	enableKnative := os.Getenv("ENABLE_KNATIVE_INGRESS") == "true" || os.Getenv("ENABLE_KNATIVE_INGRESS") == "1"

	opts := Opts{
		WriteNamespace:  writeNamespace,
		WatchNamespaces: watchNamespaces,
		Proxies:         proxyFactory,
		Upstreams:       upstreamFactory,
		Secrets:         secretFactory,
		WatchOpts: clients.WatchOpts{
			Ctx:         ctx,
			RefreshRate: refreshRate,
		},
		EnableKnative:      enableKnative,
		DisableKubeIngress: disableKubeIngress,
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
	secretClient, err := gloov1.NewSecretClient(opts.Secrets)
	if err != nil {
		return err
	}

	proxyClient, err := gloov1.NewProxyClient(opts.Proxies)
	if err != nil {
		return err
	}
	if err := proxyClient.Register(); err != nil {
		return err
	}
	upstreamClient, err := gloov1.NewUpstreamClient(opts.Upstreams)
	if err != nil {
		return err
	}
	if err := upstreamClient.Register(); err != nil {
		return err
	}
	writeErrs := make(chan error)

	if !opts.DisableKubeIngress {
		kube, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			return errors.Wrapf(err, "getting kube client")
		}

		baseIngressClient := ingress.NewResourceClient(kube, &v1.Ingress{})
		ingressClient := v1.NewIngressClientWithBase(baseIngressClient)

		translatorEmitter := v1.NewTranslatorEmitter(secretClient, upstreamClient, ingressClient)
		translatorSync := translator.NewSyncer(opts.WriteNamespace, proxyClient, ingressClient, writeErrs)
		translatorEventLoop := v1.NewTranslatorEventLoop(translatorEmitter, translatorSync)
		translatorEventLoopErrs, err := translatorEventLoop.Run(opts.WatchNamespaces, opts.WatchOpts)
		if err != nil {
			return err
		}
		go errutils.AggregateErrs(opts.WatchOpts.Ctx, writeErrs, translatorEventLoopErrs, "ingress_translator_event_loop")

		baseKubeServiceClient := service.NewResourceClient(kube, &v1.KubeService{})
		kubeServiceClient := v1.NewKubeServiceClientWithBase(baseKubeServiceClient)
		// note (ilackarms): we must set the selector correctly here or the status syncer will not work
		// the selector should return exactly 1 service which is our <install-namespace>.ingress-proxy service
		// TODO (ilackarms): make the service labels configurable
		kubeServiceClient = service.NewClientWithSelector(kubeServiceClient, map[string]string{
			"gloo": "ingress-proxy",
		})
		statusEmitter := v1.NewStatusEmitter(kubeServiceClient, ingressClient)
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
		logger.Infof("starting Ingress with KNative (ClusterIngress) support enabled")
		knative, err := knativeclientset.NewForConfig(cfg)
		if err != nil {
			return errors.Wrapf(err, "creating knative clientset")
		}

		baseClient := clusteringress.NewResourceClient(knative, &clusteringressv1.ClusterIngress{})
		ingressClient := clusteringressv1.NewClusterIngressClientWithBase(baseClient)
		clusterIngTranslatorEmitter := clusteringressv1.NewTranslatorEmitter(secretClient, upstreamClient, ingressClient)
		clusterIngTranslatorSync := clusteringresstranslator.NewSyncer(opts.WriteNamespace, proxyClient, ingressClient, writeErrs)
		clusterIngTranslatorEventLoop := clusteringressv1.NewTranslatorEventLoop(clusterIngTranslatorEmitter, clusterIngTranslatorSync)
		clusterIngTranslatorEventLoopErrs, err := clusterIngTranslatorEventLoop.Run(opts.WatchNamespaces, opts.WatchOpts)
		if err != nil {
			return err
		}
		go errutils.AggregateErrs(opts.WatchOpts.Ctx, writeErrs, clusterIngTranslatorEventLoopErrs, "cluster_ingress_translator_event_loop")
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
