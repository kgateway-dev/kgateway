package controller

import (
	"context"

	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/config"

	"github.com/solo-io/go-utils/contextutils"

	glooschemes "github.com/kgateway-dev/kgateway/v2/pkg/schemes"

	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/healthz"
	czap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	uzap "go.uber.org/zap"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	istiolog "istio.io/istio/pkg/log"
	infextv1a1 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha1"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/deployer"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/registry"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/proxy_syncer"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
)

const (
	// AutoProvision controls whether the controller will be responsible for provisioning dynamic
	// infrastructure for the Gateway API.
	AutoProvision = true
)

type SetupOpts struct {
	Cache               envoycache.SnapshotCache
	ExtraGatewayClasses []string

	KrtDebugger *krt.DebugHandler

	XdsHost string
	XdsPort int32
}

var setupLog = ctrl.Log.WithName("setup")

type StartConfig struct {
	Dev        bool
	SetupOpts  *SetupOpts
	RestConfig *rest.Config
	// ExtensionsFactory is the factory function which will return an extensions.K8sGatewayExtensions
	// This is responsible for producing the extension points that this controller requires
	ExtraPlugins []extensionsplug.Plugin

	Client istiokube.Client

	AugmentedPods krt.Collection[krtcollections.LocalityPod]
	UniqueClients krt.Collection[ir.UniqlyConnectedClient]

	KrtOptions krtutil.KrtOptions
}

// Start runs the controllers responsible for processing the K8s Gateway API objects
// It is intended to be run in a goroutine as the function will block until the supplied
// context is cancelled
type ControllerBuilder struct {
	proxySyncer *proxy_syncer.ProxySyncer
	cfg         StartConfig
	mgr         ctrl.Manager
	isOurGw     func(gw *apiv1.Gateway) bool
	settings    settings.Settings
}

func NewControllerBuilder(ctx context.Context, cfg StartConfig) (*ControllerBuilder, error) {
	var opts []czap.Opts
	loggingOptions := istiolog.DefaultOptions()

	if cfg.Dev {
		setupLog.Info("starting log in dev mode")
		opts = append(opts, czap.UseDevMode(true))
		loggingOptions.SetDefaultOutputLevel(istiolog.OverrideScopeName, istiolog.DebugLevel)
	}
	ctrl.SetLogger(czap.New(opts...))
	istiolog.Configure(loggingOptions)

	scheme := DefaultScheme()

	// Extend the scheme if the TCPRoute CRD exists.
	if err := glooschemes.AddGatewayV1A2Scheme(cfg.RestConfig, scheme); err != nil {
		return nil, err
	}

	// Extend the scheme if the InferencePool CRD exists.
	if _, err := glooschemes.AddInferExtV1A1Scheme(cfg.RestConfig, scheme); err != nil {
		return nil, err
	}

	mgrOpts := ctrl.Options{
		BaseContext:      func() context.Context { return ctx },
		Scheme:           scheme,
		PprofBindAddress: "127.0.0.1:9099",
		// if you change the port here, also change the port "health" in the helmchart.
		HealthProbeBindAddress: ":9093",
		Metrics: metricsserver.Options{
			BindAddress: ":9092",
		},
		Controller: config.Controller{
			// see https://github.com/kubernetes-sigs/controller-runtime/issues/2937
			// in short, our tests reuse the same name (reasonably so) and the controller-runtime
			// package does not reset the stack of controller names between tests, so we disable
			// the name validation here.
			SkipNameValidation: ptr.To(true),
		},
	}
	mgr, err := ctrl.NewManager(cfg.RestConfig, mgrOpts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return nil, err
	}

	// TODO: replace this with something that checks that we have xds snapshot ready (or that we don't need one).
	mgr.AddReadyzCheck("ready-ping", healthz.Ping)

	setupLog.Info("initializing kgateway extensions")
	cli, err := versioned.NewForConfig(cfg.RestConfig)
	if err != nil {
		return nil, err
	}
	commoncol := common.NewCommonCollections(
		cfg.KrtOptions,
		cfg.Client,
		cli,
		setupLog,
	)
	gwClasses := sets.New(append(cfg.SetupOpts.ExtraGatewayClasses, wellknown.GatewayClassName)...)
	isOurGw := func(gw *apiv1.Gateway) bool {
		return gwClasses.Has(string(gw.Spec.GatewayClassName))
	}
	// Create the proxy syncer for the Gateway API resources
	setupLog.Info("initializing proxy syncer")
	proxySyncer := proxy_syncer.NewProxySyncer(
		ctx,
		wellknown.GatewayControllerName,
		mgr,
		cfg.Client,
		cfg.UniqueClients,
		pluginFactoryWithBuiltin(cfg.ExtraPlugins),
		commoncol,
		cfg.SetupOpts.Cache,
	)
	proxySyncer.Init(ctx, isOurGw, cfg.KrtOptions)

	if err := mgr.Add(proxySyncer); err != nil {
		setupLog.Error(err, "unable to add proxySyncer runnable")
		return nil, err
	}

	setupLog.Info("starting controller builder", "GatewayClasses", sets.List(gwClasses))
	return &ControllerBuilder{
		proxySyncer: proxySyncer,
		cfg:         cfg,
		mgr:         mgr,
		isOurGw:     isOurGw,
		settings:    commoncol.Settings,
	}, nil
}

func pluginFactoryWithBuiltin(extraPlugins []extensionsplug.Plugin) extensions2.K8sGatewayExtensionsFactory {
	return func(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
		plugins := registry.Plugins(ctx, commoncol)
		plugins = append(plugins, krtcollections.NewBuiltinPlugin(ctx))
		plugins = append(plugins, extraPlugins...)
		return registry.MergePlugins(plugins...)
	}
}

func (c *ControllerBuilder) Start(ctx context.Context) error {
	logger := contextutils.LoggerFrom(ctx).Desugar()
	logger.Info("starting gateway controller")
	// GetXdsAddress waits for gloo-edge to populate the xds address of the server.
	// in the future this logic may move here and be duplicated.
	xdsHost, xdsPort := c.cfg.SetupOpts.XdsHost, c.cfg.SetupOpts.XdsPort
	if xdsHost == "" {
		return ctx.Err()
	}

	logger.Info("got xds address for deployer", uzap.String("xds_host", xdsHost), uzap.Int32("xds_port", xdsPort))

	integrationEnabled := c.settings.EnableIstioIntegration

	// copy over relevant aws options (if any) from Settings
	var awsInfo *deployer.AwsInfo
	stsCluster := c.settings.StsClusterName
	stsUri := c.settings.StsUri
	if stsCluster != "" && stsUri != "" {
		awsInfo = &deployer.AwsInfo{
			EnableServiceAccountCredentials: true,
			StsClusterName:                  stsCluster,
			StsUri:                          stsUri,
		}
	} else {
		awsInfo = &deployer.AwsInfo{
			EnableServiceAccountCredentials: false,
		}
	}

	gwCfg := GatewayConfig{
		Mgr:            c.mgr,
		OurGateway:     c.isOurGw,
		ControllerName: wellknown.GatewayControllerName,
		AutoProvision:  AutoProvision,
		ControlPlane: deployer.ControlPlaneInfo{
			XdsHost: xdsHost,
			XdsPort: xdsPort,
		},
		// TODO pass in the settings so that the deloyer can register to it for changes.
		IstioIntegrationEnabled: integrationEnabled,
		Aws:                     awsInfo,
	}

	if err := NewBaseGatewayController(ctx, gwCfg); err != nil {
		setupLog.Error(err, "unable to create gateway controller")
		return err
	}

	// Create the InferencePool controller if the inference extension API group is registered.
	if c.mgr.GetScheme().IsGroupRegistered(infextv1a1.GroupVersion.Group) {
		poolCfg := &InferencePoolConfig{
			Mgr:            c.mgr,
			ControllerName: wellknown.GatewayControllerName,
			InferenceExt:   new(deployer.InferenceExtInfo),
		}
		if err := NewBaseInferencePoolController(ctx, poolCfg, &gwCfg); err != nil {
			setupLog.Error(err, "unable to create inferencepool controller")
			return err
		}
	}

	return c.mgr.Start(ctx)
}
