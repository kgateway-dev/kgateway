package controller

import (
	"os"

	"github.com/solo-io/gloo/projects/gateway2/discovery"
	"github.com/solo-io/gloo/projects/gateway2/xds"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

type ControllerConfig struct {
	// The name of the GatewayClass to watch for
	GatewayClassName      string
	GatewayControllerName string
	Release               string
	AutoProvision         bool
	XdsServer             string
	XdsPort               uint16
	Dev                   bool
}

func NewScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	for _, f := range []func(*runtime.Scheme) error{
		apiv1.AddToScheme, apiv1beta1.AddToScheme, corev1.AddToScheme, appsv1.AddToScheme,
	} {
		if err := f(scheme); err != nil {
			setupLog.Error(err, "unable to add scheme")
			os.Exit(1)
		}
	}
	return scheme

}

func Start(cfg ControllerConfig) {
	var opts []zap.Opts
	if cfg.Dev {
		setupLog.Info("starting log in dev mode")
		opts = append(opts, zap.UseDevMode(true))
	}
	ctrl.SetLogger(zap.New(opts...))
	mgrOpts := ctrl.Options{
		Scheme:           NewScheme(),
		PprofBindAddress: "127.0.0.1:9099",
		// if you change the port here, also change the port "health" in the helmchart.
		HealthProbeBindAddress: ":9091",
		Metrics: metricsserver.Options{
			BindAddress: ":9090",
		},
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// TODO: replace this with something that checks that we have xds snapshot ready (or that we don't need one).
	mgr.AddReadyzCheck("ready-ping", healthz.Ping)

	ctx := signals.SetupSignalHandler()

	var gatewayClassName apiv1.ObjectName = apiv1.ObjectName(cfg.GatewayClassName)
	err = NewBaseGatewayController(ctx, mgr, gatewayClassName, cfg.Release, cfg.GatewayControllerName, cfg.AutoProvision, cfg.XdsServer, cfg.XdsPort)

	if err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	inputChannels := xds.NewXdsInputChannels()
	err = discovery.NewDiscoveryController(ctx, mgr, inputChannels)
	if err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
