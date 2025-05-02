package setup

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/go-logr/logr"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/admin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/controller"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	controllerlog "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	componentName = "kgateway"
)

func Main(customCtx context.Context) error {
	SetupLogging(componentName)
	return startSetupLoop(customCtx)
}

func startSetupLoop(ctx context.Context) error {
	return StartKgateway(ctx, nil)
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

func StartKgateway(
	ctx context.Context,
	extraPlugins func(ctx context.Context, commoncol *common.CommonCollections) []extensionsplug.Plugin,
) error {
	slog.Info(fmt.Sprintf("starting %s", componentName))

	// load global settings
	st, err := settings.BuildSettings()
	if err != nil {
		slog.Error("got err while parsing Settings from env", slog.Any("error", err))
	}
	slog.Info(fmt.Sprintf("got settings from env: %+v", *st))

	uniqueClientCallbacks, uccBuilder := krtcollections.NewUniquelyConnectedClients()
	cache, err := startControlPlane(ctx, st.XdsServicePort, uniqueClientCallbacks)
	if err != nil {
		return err
	}

	setupOpts := &controller.SetupOpts{
		Cache:                  cache,
		KrtDebugger:            new(krt.DebugHandler),
		GlobalSettings:         st,
		PprofBindAddress:       "127.0.0.1:9099",
		HealthProbeBindAddress: ":9093",
		MetricsBindAddress:     ":9092",
	}

	restConfig := ctrl.GetConfigOrDie()
	return StartKgatewayWithConfig(ctx, setupOpts, restConfig, uccBuilder, extraPlugins)
}

func startControlPlane(
	ctx context.Context,
	port uint32,
	callbacks xdsserver.Callbacks,
) (envoycache.SnapshotCache, error) {
	return NewControlPlane(ctx, &net.TCPAddr{IP: net.IPv4zero, Port: int(port)}, callbacks)
}

func StartKgatewayWithConfig(
	ctx context.Context,
	setupOpts *controller.SetupOpts,
	restConfig *rest.Config,
	uccBuilder krtcollections.UniquelyConnectedClientsBulider,
	extraPlugins func(ctx context.Context, commoncol *common.CommonCollections) []extensionsplug.Plugin,
) error {
	k8sLogger := slog.With(slog.String("component", "k8s"))
	k8sLogger.Info(fmt.Sprintf("starting %s", componentName))

	kubeClient, err := createKubeClient(restConfig)
	if err != nil {
		return err
	}

	k8sLogger.Info("creating krt collections")
	krtOpts := krtutil.NewKrtOptions(ctx.Done(), setupOpts.KrtDebugger)

	augmentedPods := krtcollections.NewPodsCollection(kubeClient, krtOpts)
	augmentedPodsForUcc := augmentedPods
	if envutils.IsEnvTruthy("DISABLE_POD_LOCALITY_XDS") {
		augmentedPodsForUcc = nil
	}

	ucc := uccBuilder(ctx, krtOpts, augmentedPodsForUcc)

	k8sLogger.Info("initializing controller")
	c, err := controller.NewControllerBuilder(ctx, controller.StartConfig{
		// TODO: why do we plumb this through if it's wellknown?
		ControllerName: wellknown.GatewayControllerName,
		ExtraPlugins:   extraPlugins,
		RestConfig:     restConfig,
		SetupOpts:      setupOpts,
		Client:         kubeClient,
		AugmentedPods:  augmentedPods,
		UniqueClients:  ucc,
		Dev:            os.Getenv("LOG_LEVEL") == "debug",
		KrtOptions:     krtOpts,
	})
	if err != nil {
		slog.Error("failed initializing controller: ", slog.Any("error", err))
		return err
	}

	slog.Info("waiting for cache sync")
	kubeClient.RunAndWait(ctx.Done())

	slog.Info("starting admin server")
	go admin.RunAdminServer(ctx, setupOpts)

	slog.Info("starting controller")
	return c.Start(ctx)
}

// SetupLogging configures the global slog logger
func SetupLogging(loggerName string) {
	// pick level from env or default to INFO
	var levelVar slog.LevelVar
	levelVar.Set(slog.LevelInfo)

	if l := os.Getenv("LOG_LEVEL"); l != "" {
		var level slog.Level
		if err := level.UnmarshalText([]byte(strings.ToLower(l))); err != nil {
			fmt.Fprintf(os.Stderr, "unknown LOG_LEVEL %q, defaulting to INFO: %v\n", l, err)
			level = slog.LevelInfo
		}
		levelVar.Set(level)
	}

	handlerOpts := &slog.HandlerOptions{
		AddSource: false,
		Level:     &levelVar,
	}

	handler := slog.NewJSONHandler(os.Stdout, handlerOpts)

	baseLogger := slog.New(handler)
	slog.SetDefault(baseLogger)

	// set controller-runtime logger
	controllerLogger := baseLogger.With(slog.String("component", loggerName))
	logrSink := logr.FromSlogHandler(controllerLogger.Handler())
	controllerlog.SetLogger(logrSink)
}
