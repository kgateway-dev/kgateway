package setup

import (
	"context"
	"fmt"
	"net"
	"os"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/go-logr/zapr"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	zaputil "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/admin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/controller"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/version"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
)

const (
	componentName = "kgateway"
)

func Main(customCtx context.Context) error {
	SetupLogging(customCtx, componentName)
	return startSetupLoop(customCtx)
}

func startSetupLoop(ctx context.Context) error {
	return StartGGv2(ctx, nil, nil)
}

func createKubeClient(restConfig *rest.Config) (istiokube.Client, error) {
	restCfg := istiokube.NewClientConfigForRestConfig(restConfig)
	client, err := istiokube.NewClient(restCfg, "")
	if err != nil {
		return nil, err
	}
	istiokube.EnableCrdWatcher(client)
	return client, nil
}

func StartGGv2(
	ctx context.Context,
	extraPlugins []extensionsplug.Plugin,
	extraGwClasses []string, // TODO: we can remove this and replace with something that watches all GW classes with our controller name
) error {
	logger := contextutils.LoggerFrom(ctx)

	// load global settings
	st, err := settings.BuildSettings()
	if err != nil {
		logger.Error(err, "got err while parsing Settings from env")
	}
	logger.Info(fmt.Sprintf("got settings from env: %+v", *st))

	uniqueClientCallbacks, uccBuilder := krtcollections.NewUniquelyConnectedClients()
	cache, err := startControlPlane(ctx, st.XdsServicePort, uniqueClientCallbacks)
	if err != nil {
		return err
	}

	setupOpts := &controller.SetupOpts{
		Cache:                  cache,
		KrtDebugger:            new(krt.DebugHandler),
		ExtraGatewayClasses:    extraGwClasses,
		GlobalSettings:         st,
		PprofBindAddress:       "127.0.0.1:9099",
		HealthProbeBindAddress: ":9093",
		MetricsBindAddress:     ":9092",
	}

	restConfig := ctrl.GetConfigOrDie()
	return StartGGv2WithConfig(ctx, setupOpts, restConfig, uccBuilder, extraPlugins, nil)
}

func startControlPlane(
	ctx context.Context,
	port uint32,
	callbacks xdsserver.Callbacks,
) (envoycache.SnapshotCache, error) {
	return NewControlPlane(ctx, &net.TCPAddr{IP: net.IPv4zero, Port: int(port)}, callbacks)
}

func StartGGv2WithConfig(
	ctx context.Context,
	setupOpts *controller.SetupOpts,
	restConfig *rest.Config,
	uccBuilder krtcollections.UniquelyConnectedClientsBulider,
	extraPlugins []extensionsplug.Plugin,
	extraGwClasses []string, // TODO: we can remove this and replace with something that watches all GW classes with our controller name
) error {
	ctx = contextutils.WithLogger(ctx, "k8s")
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("starting %s", componentName)

	kubeClient, err := createKubeClient(restConfig)
	if err != nil {
		return err
	}

	logger.Info("creating krt collections")
	krtOpts := krtutil.NewKrtOptions(ctx.Done(), setupOpts.KrtDebugger)

	augmentedPods := krtcollections.NewPodsCollection(kubeClient, krtOpts)
	augmentedPodsForUcc := augmentedPods
	if envutils.IsEnvTruthy("DISABLE_POD_LOCALITY_XDS") {
		augmentedPodsForUcc = nil
	}

	ucc := uccBuilder(ctx, krtOpts, augmentedPodsForUcc)

	logger.Info("initializing controller")
	c, err := controller.NewControllerBuilder(ctx, controller.StartConfig{
		ExtraPlugins:  extraPlugins,
		RestConfig:    restConfig,
		SetupOpts:     setupOpts,
		Client:        kubeClient,
		AugmentedPods: augmentedPods,
		UniqueClients: ucc,

		// Dev flag may be useful for development purposes; not currently tied to any user-facing API
		Dev:        os.Getenv("LOG_LEVEL") == "debug",
		KrtOptions: krtOpts,
	})
	if err != nil {
		logger.Error("failed initializing controller: ", err)
		return err
	}
	/// no collections after this point

	logger.Info("waiting for cache sync")
	kubeClient.RunAndWait(ctx.Done())

	logger.Info("starting admin server")
	go admin.RunAdminServer(ctx, setupOpts)

	logger.Info("starting controller")
	return c.Start(ctx)
}

// SetupLogging sets up controller-runtime logging
func SetupLogging(ctx context.Context, loggerName string) {
	level := zapcore.InfoLevel
	// if log level is set in env, use that
	if envLogLevel := os.Getenv(contextutils.LogLevelEnvName); envLogLevel != "" {
		if err := (&level).Set(envLogLevel); err != nil {
			contextutils.LoggerFrom(ctx).Infof("Could not set log level from env %s=%s, available levels "+
				"can be found here: https://pkg.go.dev/go.uber.org/zap/zapcore?tab=doc#Level",
				contextutils.LogLevelEnvName,
				envLogLevel,
				zap.Error(err),
			)
		}
	}
	atomicLevel := zap.NewAtomicLevelAt(level)

	baseLogger := zaputil.NewRaw(
		zaputil.Level(&atomicLevel),
		zaputil.RawZapOpts(zap.Fields(zap.String("version", version.Version))),
	).Named(loggerName)

	// controller-runtime
	log.SetLogger(zapr.NewLogger(baseLogger))
}
