package controller

import (
	"context"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/config"

	glooschemes "github.com/solo-io/gloo/pkg/schemes"

	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	ext "github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gateway2/proxy_syncer"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

const (
	// AutoProvision controls whether the controller will be responsible for provisioning dynamic
	// infrastructure for the Gateway API.
	AutoProvision = true
)

var setupLog = ctrl.Log.WithName("setup")

type StartConfig struct {
	Dev  bool
	Opts bootstrap.Opts

	// ExtensionsFactory is the factory function which will return an extensions.K8sGatewayExtensions
	// This is responsible for producing the extension points that this controller requires
	ExtensionsFactory extensions.K8sGatewayExtensionsFactory

	// GlooPluginRegistryFactory is the factory function to produce a PluginRegistry
	// The plugins in this registry are used during the conversion of a Proxy resource into an xDS Snapshot
	GlooPluginRegistryFactory plugins.PluginRegistryFactory

	// ProxyClient is the client that writes Proxy resources into an in-memory cache
	// This cache ultimately populates the ApiSnapshot (important for extensions such as Rate Limit to work correctly)
	// and is also utilized by the debug.ProxyEndpointServer
	ProxyClient v1.ProxyClient

	// AuthConfigClient is the client used for retrieving AuthConfig objects within the Portal Plugin
	AuthConfigClient extauthv1.AuthConfigClient

	// RouteOptionClient is the client used for retrieving RouteOption objects within the RouteOptionsPlugin
	RouteOptionClient gatewayv1.RouteOptionClient

	// VirtualHostOptionClient is the client used for retrieving VirtualHostOption objects within the VirtualHostOptionsPlugin
	VirtualHostOptionClient gatewayv1.VirtualHostOptionClient

	// SecretClient is used for converting from kube Secrets to gloov1 Secrets
	SecretClient v1.SecretClient

	// GlooStatusReporter is the shared reporter from setup_syncer that reports as 'gloo',
	// it is used to report on Upstreams and Proxies after xds translation.
	// this is required because various upstream tests expect a certain reporter for Upstreams
	// TODO: remove the other reporter and only use this one, no need for 2 different reporters
	GlooStatusReporter reporter.StatusReporter

	// KubeGwStatusReporter is used within any StatusPlugins that must persist a GE-classic style status
	// TODO: as mentioned above, this should be removed: https://github.com/solo-io/solo-projects/issues/7055
	KubeGwStatusReporter reporter.StatusReporter

	// Translator is an instance of the Gloo translator used to translate Proxy -> xDS Snapshot
	Translator translator.Translator

	// SyncerExtensions is a list of extensions, the kube gw controller will use these to get extension-specific
	// errors & warnings for any Proxies it generates
	SyncerExtensions []syncer.TranslatorSyncerExtension
}

// Start runs the controllers responsible for processing the K8s Gateway API objects
// It is intended to be run in a goroutine as the function will block until the supplied
// context is cancelled
func Start(ctx context.Context, cfg StartConfig) error {
	var opts []zap.Opts
	if cfg.Dev {
		setupLog.Info("starting log in dev mode")
		opts = append(opts, zap.UseDevMode(true))
	}
	ctrl.SetLogger(zap.New(opts...))

	mgrOpts := ctrl.Options{
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
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return err
	}

	// TODO: replace this with something that checks that we have xds snapshot ready (or that we don't need one).
	mgr.AddReadyzCheck("ready-ping", healthz.Ping)

	inputChannels := proxy_syncer.NewGatewayInputChannels()
	k8sGwExtensions, err := cfg.ExtensionsFactory(ctx, ext.K8sGatewayExtensionsFactoryParameters{
		Mgr:                     mgr,
		RouteOptionClient:       cfg.RouteOptionClient,
		VirtualHostOptionClient: cfg.VirtualHostOptionClient,
		StatusReporter:          cfg.KubeGwStatusReporter,
		AuthConfigClient:        cfg.AuthConfigClient,
		KickXds:                 inputChannels.Kick,
	})
	if err != nil {
		setupLog.Error(err, "unable to create k8s gw extensions")
		return err
	}
	// Create the proxy syncer for the Gateway API resources
	proxySyncer := proxy_syncer.NewProxySyncer(
		wellknown.GatewayControllerName,
		cfg.Opts.WriteNamespace,
		inputChannels,
		mgr,
		k8sGwExtensions,
		cfg.ProxyClient,
		cfg.Translator,
		cfg.Opts.ControlPlane.SnapshotCache,
		cfg.Opts.Settings,
		cfg.SyncerExtensions,
		cfg.SecretClient,
		cfg.GlooStatusReporter,
	)

	if err := mgr.Add(proxySyncer); err != nil {
		setupLog.Error(err, "unable to add proxySyncer runnable")
		return err
	}

	gwCfg := GatewayConfig{
		Mgr:            mgr,
		GWClasses:      sets.New(append(cfg.Opts.ExtraGatewayClasses, wellknown.GatewayClassName)...),
		ControllerName: wellknown.GatewayControllerName,
		AutoProvision:  AutoProvision,
		ControlPlane:   cfg.Opts.ControlPlane, // TODO(Law) type seems FAR too broad, only used to provide the xds addr
		IstioValues:    cfg.Opts.GlooGateway.IstioValues,
		Kick:           inputChannels.Kick,
		Extensions:     k8sGwExtensions,
	}
	if err := NewBaseGatewayController(ctx, gwCfg); err != nil {
		setupLog.Error(err, "unable to create controller")
		return err
	}

	return mgr.Start(ctx)
}
