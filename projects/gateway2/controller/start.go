package controller

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gateway2/proxy_syncer"
	"github.com/solo-io/gloo/projects/gateway2/secrets"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

const (
	// AutoProvision controls whether the controller will be responsible for provisioning dynamic
	// infrastructure for the Gateway API.
	AutoProvision = true
)

var (
	gatewayClass = apiv1.ObjectName(wellknown.GatewayClassName)

	setupLog = ctrl.Log.WithName("setup")
)

type StartConfig struct {
	Dev  bool
	Opts bootstrap.Opts

	// ExtensionsFactory is the factory function which will return an extensions.K8sGatewayExtensions
	// This is responsible for producing the extension points that this controller requires
	K8sGatewayExtensions extensions.K8sGatewayExtensions

	// GlooPluginRegistryFactory is the factory function to produce a PluginRegistry
	// The plugins in this registry are used during the conversion of a Proxy resource into an xDS Snapshot
	GlooPluginRegistryFactory plugins.PluginRegistryFactory

	// ProxyClient is the client that writes Proxy resources into an in-memory cache
	// This cache is utilized by the debug.ProxyEndpointServer
	ProxyClient v1.ProxyClient

	InputChannels *proxy_syncer.GatewayInputChannels

	Mgr manager.Manager
}

// Start runs the controllers responsible for processing the K8s Gateway API objects
// It is intended to be run in a goroutine as the function will block until the supplied
// context is cancelled
func Start(ctx context.Context, cfg StartConfig) error {
	// var opts []zap.Opts
	// if cfg.Dev {
	// 	setupLog.Info("starting log in dev mode")
	// 	opts = append(opts, zap.UseDevMode(true))
	// }
	// ctrl.SetLogger(zap.New(opts...))

	mgr := cfg.Mgr
	// TODO: replace this with something that checks that we have xds snapshot ready (or that we don't need one).
	mgr.AddReadyzCheck("ready-ping", healthz.Ping)

	// Create the proxy syncer for the Gateway API resources
	proxySyncer := proxy_syncer.NewProxySyncer(
		wellknown.GatewayControllerName,
		cfg.Opts.WriteNamespace,
		cfg.InputChannels,
		mgr,
		cfg.K8sGatewayExtensions,
		cfg.ProxyClient,
	)
	if err := mgr.Add(proxySyncer); err != nil {
		setupLog.Error(err, "unable to add proxySyncer runnable")
		return err
	}

	gwCfg := GatewayConfig{
		Mgr:            mgr,
		GWClass:        gatewayClass,
		ControllerName: wellknown.GatewayControllerName,
		AutoProvision:  AutoProvision,
		ControlPlane:   cfg.Opts.ControlPlane,
		IstioValues:    cfg.Opts.GlooGateway.IstioValues,
		Kick:           cfg.InputChannels.Kick,
		Extensions:     cfg.K8sGatewayExtensions,
	}
	if err := NewBaseGatewayController(ctx, gwCfg); err != nil {
		setupLog.Error(err, "unable to create controller")
		return err
	}

	if err := secrets.NewSecretsController(ctx, mgr, cfg.InputChannels); err != nil {
		setupLog.Error(err, "unable to create controller")
		return err
	}

	return mgr.Start(ctx)
}
