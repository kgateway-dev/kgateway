package controller

import (
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	api "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

type ControllerConfig struct {
	// The name of the GatewayClass to watch for
	GatewayClassName string
}

func NewScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	for _, f := range []func(*runtime.Scheme) error{
		api.AddToScheme, corev1.AddToScheme,
	} {
		if err := f(scheme); err != nil {
			setupLog.Error(err, "unable to add scheme")
			os.Exit(1)
		}
	}
	return scheme

}

func Start(cfg ControllerConfig) {
	ctrl.SetLogger(zap.New())
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{Scheme: NewScheme()})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	ctx := signals.SetupSignalHandler()

	var gatewayClassName api.ObjectName = api.ObjectName(cfg.GatewayClassName)
	err = newBaseGatewayController(ctx, mgr, gatewayClassName)

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
