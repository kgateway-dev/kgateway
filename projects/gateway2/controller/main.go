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

func Start() {
	ctrl.SetLogger(zap.New())
	scheme := runtime.NewScheme()
	for _, f := range []func(*runtime.Scheme) error{
		api.AddToScheme, corev1.AddToScheme,
	} {
		if err := f(scheme); err != nil {
			setupLog.Error(err, "unable to add scheme")
			os.Exit(1)
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ctx := signals.SetupSignalHandler()

	var gatewayClassName api.ObjectName = "gloo-edge"
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
