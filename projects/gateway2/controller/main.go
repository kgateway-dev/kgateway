package controller

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/projects/gateway2/controller/scheme"
	"github.com/solo-io/gloo/projects/gateway2/discovery"
	"github.com/solo-io/gloo/projects/gateway2/secrets"
	"github.com/solo-io/gloo/projects/gateway2/xds"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ControllerConfig struct {
	Ctx context.Context

	// The name of the GatewayClass to watch for
	GatewayClassName      string
	GatewayControllerName string
	Release               string
	AutoProvision         bool

	ControlPlane bootstrap.ControlPlane
}

// Start runs the controllers responsible for processing the K8s Gateway API objects
// It is intended to be run in a goroutine as the function will block until the supplied
// context is cancelled
func Start(cfg ControllerConfig) error {
	ctx := cfg.Ctx
	logger := contextutils.LoggerFrom(ctx)

	mgrOpts := ctrl.Options{
		Scheme:           scheme.NewScheme(),
		PprofBindAddress: "127.0.0.1:9099",
		// if you change the port here, also change the port "health" in the helmchart.
		HealthProbeBindAddress: ":9093",
		Metrics: metricsserver.Options{
			BindAddress: ":9092",
		},
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		logger.Error(err, "unable to start manager")
		return err
	}

	// TODO: replace this with something that checks that we have xds snapshot ready (or that we don't need one).
	mgr.AddReadyzCheck("ready-ping", healthz.Ping)

	glooTranslator := newGlooTranslator(ctx)
	var sanz sanitizer.XdsSanitizers
	inputChannels := xds.NewXdsInputChannels()
	xdsSyncer := xds.NewXdsSyncer(
		cfg.GatewayControllerName,
		glooTranslator,
		sanz,
		cfg.ControlPlane.SnapshotCache,
		false,
		inputChannels,
		mgr.GetClient(),
		mgr.GetScheme(),
	)
	if err := mgr.Add(xdsSyncer); err != nil {
		logger.Error(err, "unable to add xdsSyncer runnable")
		return err
	}

	var gatewayClassName = apiv1.ObjectName(cfg.GatewayClassName)

	gwCfg := GatewayConfig{
		Mgr:            mgr,
		GWClass:        gatewayClassName,
		ControllerName: cfg.GatewayControllerName,
		AutoProvision:  cfg.AutoProvision,
		ControlPlane:   cfg.ControlPlane,
		Kick:           inputChannels.Kick,
	}
	if err = NewBaseGatewayController(ctx, gwCfg); err != nil {
		logger.Error(err, "unable to create controller")
		return err
	}

	if err = discovery.NewDiscoveryController(ctx, mgr, inputChannels); err != nil {
		logger.Error(err, "unable to create controller")
		return err
	}

	if err = secrets.NewSecretsController(ctx, mgr, inputChannels); err != nil {
		logger.Error(err, "unable to create controller")
		return err
	}

	logger.Debugf("Starting controller-runtime.Manager")
	return mgr.Start(ctx)
}
