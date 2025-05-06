package setup

import (
	"context"
	"log/slog"
	"net"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
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
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/logging"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
)

func Main(customCtx context.Context) error {
	// Logging will be set up inside StartKgateway after settings are loaded.
	return StartKgateway(customCtx, nil)
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
	// load global settings
	st, err := settings.BuildSettings()
	if err != nil {
		slog.Error("error loading settings from env", slog.Any("error", err))
	}

	SetupLogging(st.LogLevel)
	slog.Info("global settings loaded", "settings", *st)

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
	slog.Info("starting kgateway")

	kubeClient, err := createKubeClient(restConfig)
	if err != nil {
		return err
	}

	slog.Info("creating krt collections")
	krtOpts := krtutil.NewKrtOptions(ctx.Done(), setupOpts.KrtDebugger)

	augmentedPods := krtcollections.NewPodsCollection(kubeClient, krtOpts)
	augmentedPodsForUcc := augmentedPods
	if envutils.IsEnvTruthy("DISABLE_POD_LOCALITY_XDS") {
		augmentedPodsForUcc = nil
	}

	ucc := uccBuilder(ctx, krtOpts, augmentedPodsForUcc)

	slog.Info("initializing controller")
	c, err := controller.NewControllerBuilder(ctx, controller.StartConfig{
		// TODO: why do we plumb this through if it's wellknown?
		ControllerName: wellknown.GatewayControllerName,
		ExtraPlugins:   extraPlugins,
		RestConfig:     restConfig,
		SetupOpts:      setupOpts,
		Client:         kubeClient,
		AugmentedPods:  augmentedPods,
		UniqueClients:  ucc,
		Dev:            setupOpts.GlobalSettings.LogLevel == "debug",
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
func SetupLogging(levelStr string) {
	baseLogger := logging.NewWithOptions("", logging.Options{
		Format: logging.JSONFormat,
	})
	if levelStr != "" {
		level, err := logging.ParseLevel(levelStr)
		if err != nil {
			slog.Error("failed to parse log level, defaulting to info", slog.Any("error", err))
		}
		logging.SetLevel("", level)
	}
	slog.SetDefault(baseLogger)
}
