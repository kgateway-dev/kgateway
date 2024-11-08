package controller

import (
	"context"

	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/config"

	glooschemes "github.com/solo-io/gloo/pkg/schemes"
	"github.com/solo-io/go-utils/contextutils"

	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	gatewaykubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/deployer"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	ext "github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gateway2/proxy_syncer"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	extauthkubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1/kube/apis/enterprise.gloo.solo.io/v1"
	glookubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	uzap "go.uber.org/zap"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
)

const (
	// AutoProvision controls whether the controller will be responsible for provisioning dynamic
	// infrastructure for the Gateway API.
	AutoProvision = true
)

var setupLog = ctrl.Log.WithName("setup")

type StartConfig struct {
	Dev        bool
	SetupOpts  *bootstrap.SetupOpts
	RestConfig *rest.Config
	// ExtensionsFactory is the factory function which will return an extensions.K8sGatewayExtensions
	// This is responsible for producing the extension points that this controller requires
	ExtensionsFactory extensions.K8sGatewayExtensionsFactory

	// GlooStatusReporter is the shared reporter from setup_syncer that reports as 'gloo',
	// it is used to report on Upstreams and Proxies after xds translation.
	// this is required because various upstream tests expect a certain reporter for Upstreams
	// TODO: remove the other reporter and only use this one, no need for 2 different reporters
	GlooStatusReporter reporter.StatusReporter

	// KubeGwStatusReporter is used within any StatusPlugins that must persist a GE-classic style status
	// TODO: as mentioned above, this should be removed: https://github.com/solo-io/solo-projects/issues/7055
	KubeGwStatusReporter reporter.StatusReporter

	// Translator is an instance of the Gloo translator used to translate Proxy -> xDS Snapshot
	Translator setup.TranslatorFactory

	// SyncerExtensions is a list of extensions, the kube gw controller will use these to get extension-specific
	// errors & warnings for any Proxies it generates
	SyncerExtensions []syncer.TranslatorSyncerExtension

	Client istiokube.Client

	AugmentedPods krt.Collection[krtcollections.LocalityPod]
	UniqueClients krt.Collection[krtcollections.UniqlyConnectedClient]

	InitialSettings *glookubev1.Settings
	Settings        krt.Singleton[glookubev1.Settings]
}

// Start runs the controllers responsible for processing the K8s Gateway API objects
// It is intended to be run in a goroutine as the function will block until the supplied
// context is cancelled
type ControllerBuilder struct {
	proxySyncer     *proxy_syncer.ProxySyncer
	inputChannels   *proxy_syncer.GatewayInputChannels
	cfg             StartConfig
	k8sGwExtensions ext.K8sGatewayExtensions
	mgr             ctrl.Manager
}

func NewControllerBuilder(ctx context.Context, cfg StartConfig) (*ControllerBuilder, error) {
	var opts []zap.Opts
	if cfg.Dev {
		setupLog.Info("starting log in dev mode")
		opts = append(opts, zap.UseDevMode(true))
	}
	ctrl.SetLogger(zap.New(opts...))

	mgrOpts := ctrl.Options{
		BaseContext:      func() context.Context { return ctx },
		Scheme:           glooschemes.DefaultScheme(),
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

	virtualHostOptionCollection := proxy_syncer.SetupCollectionDynamic[gatewaykubev1.VirtualHostOption](
		ctx,
		cfg.Client,
		gatewaykubev1.SchemeGroupVersion.WithResource("virtualhostoptions"),
		krt.WithName("VirtualHostOption"))

	routeOptionCollection := proxy_syncer.SetupCollectionDynamic[gatewaykubev1.RouteOption](
		ctx,
		cfg.Client,
		gatewaykubev1.SchemeGroupVersion.WithResource("routeoptions"),
		krt.WithName("RouteOption"))

	authConfigCollection := proxy_syncer.SetupCollectionDynamic[extauthkubev1.AuthConfig](
		ctx,
		cfg.Client,
		gatewaykubev1.SchemeGroupVersion.WithResource("authconfigs"),
		krt.WithName("AuthConfig"))

	inputChannels := proxy_syncer.NewGatewayInputChannels()
	k8sGwExtensions, err := cfg.ExtensionsFactory(ctx, ext.K8sGatewayExtensionsFactoryParameters{
		Mgr:         mgr,
		IstioClient: cfg.Client,
		CoreCollections: ext.CoreCollections{
			AugmentedPods:               cfg.AugmentedPods,
			RouteOptionCollection:       routeOptionCollection,
			VirtualHostOptionCollection: virtualHostOptionCollection,
			AuthConfigCollection:        authConfigCollection,
		},
		StatusReporter: cfg.KubeGwStatusReporter,
		KickXds:        inputChannels.Kick,
	})
	if err != nil {
		setupLog.Error(err, "unable to create k8s gw extensions")
		return nil, err
	}

	// Create the proxy syncer for the Gateway API resources
	proxySyncer := proxy_syncer.NewProxySyncer(
		ctx,
		cfg.InitialSettings,
		cfg.Settings,
		wellknown.GatewayControllerName,
		setup.GetWriteNamespace(&cfg.InitialSettings.Spec),
		inputChannels,
		mgr,
		cfg.Client,
		cfg.AugmentedPods,
		cfg.UniqueClients,
		k8sGwExtensions,
		cfg.Translator,
		cfg.SetupOpts.Cache,
		cfg.SyncerExtensions,
		cfg.GlooStatusReporter,
		cfg.SetupOpts.ProxyReconcileQueue,
	)
	proxySyncer.Init(ctx)

	if err := mgr.Add(proxySyncer); err != nil {
		setupLog.Error(err, "unable to add proxySyncer runnable")
		return nil, err
	}

	return &ControllerBuilder{
		proxySyncer:     proxySyncer,
		inputChannels:   inputChannels,
		cfg:             cfg,
		k8sGwExtensions: k8sGwExtensions,
		mgr:             mgr,
	}, nil
}

func (c *ControllerBuilder) Start(ctx context.Context) error {
	logger := contextutils.LoggerFrom(ctx)
	logger.Info("starting gateway controller")
	// GetXdsAddress waits for gloo-edge to populate the xds address of the server.
	// in the future this logic may move here and be duplicated.
	xdsHost, xdsPort := c.cfg.SetupOpts.GetXdsAddress(ctx)
	if xdsHost == "" {
		return ctx.Err()
	}

	logger.Infow("got xds address for deployer", uzap.String("xds_host", xdsHost), uzap.Int32("xds_port", xdsPort))

	integrationEnabled := c.cfg.InitialSettings.Spec.GetGloo().GetIstioOptions().GetEnableIntegration().GetValue()

	// copy over relevant aws options (if any) from Settings
	var awsInfo *deployer.AwsInfo
	awsOpts := c.cfg.InitialSettings.Spec.GetGloo().GetAwsOptions()
	if awsOpts != nil {
		credOpts := awsOpts.GetServiceAccountCredentials()
		if credOpts != nil {
			awsInfo = &deployer.AwsInfo{
				EnableServiceAccountCredentials: true,
				StsClusterName:                  credOpts.GetCluster(),
				StsUri:                          credOpts.GetUri(),
			}
		} else {
			awsInfo = &deployer.AwsInfo{
				EnableServiceAccountCredentials: false,
			}
		}
	}

	gwCfg := GatewayConfig{
		Mgr:            c.mgr,
		GWClasses:      sets.New(append(c.cfg.SetupOpts.ExtraGatewayClasses, wellknown.GatewayClassName)...),
		ControllerName: wellknown.GatewayControllerName,
		AutoProvision:  AutoProvision,
		ControlPlane: deployer.ControlPlaneInfo{
			XdsHost: xdsHost,
			XdsPort: xdsPort,
		},
		// TODO pass in the settings so that the deloyer can register to it for changes.
		IstioIntegrationEnabled: integrationEnabled,
		Aws:                     awsInfo,
		Kick:                    c.inputChannels.Kick,
		Extensions:              c.k8sGwExtensions,
	}
	if err := NewBaseGatewayController(ctx, gwCfg); err != nil {
		setupLog.Error(err, "unable to create controller")
		return err
	}

	return c.mgr.Start(ctx)
}
