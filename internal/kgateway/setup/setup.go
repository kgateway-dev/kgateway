package setup

import (
	"context"
	"log/slog"
	"net"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/go-logr/logr"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/admin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/controller"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
)

type Server interface {
	Start(ctx context.Context) error
}

func WithExtraPlugins(extraPlugins func(ctx context.Context, commoncol *common.CommonCollections) []sdk.Plugin) func(*setup) {
	return func(s *setup) {
		s.extraPlugins = extraPlugins
	}
}

type setup struct {
	extraPlugins func(ctx context.Context, commoncol *common.CommonCollections) []sdk.Plugin
}

var _ Server = &setup{}

func New(opts ...func(*setup)) *setup {
	s := &setup{}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *setup) Start(ctx context.Context) error {
	return StartKgateway(ctx, s.extraPlugins)
}

func StartKgateway(
	ctx context.Context,
	extraPlugins func(ctx context.Context, commoncol *common.CommonCollections) []sdk.Plugin,
) error {
	// load global settings
	st, err := settings.BuildSettings()
	if err != nil {
		slog.Error("error loading settings from env", "error", err)
	}

	setupLogging(st.LogLevel)
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
	extraPlugins func(ctx context.Context, commoncol *common.CommonCollections) []sdk.Plugin,
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
		Dev:            logging.MustGetLevel(logging.DefaultComponent) <= logging.LevelTrace,
		KrtOptions:     krtOpts,
	})
	if err != nil {
		slog.Error("failed initializing controller: ", "error", err)
		return err
	}

	slog.Info("waiting for cache sync")
	kubeClient.RunAndWait(ctx.Done())

	slog.Info("starting admin server")
	go admin.RunAdminServer(ctx, setupOpts)

	slog.Info("starting controller")
	return c.Start(ctx)
}

// setupLogging configures the global slog logger
func setupLogging(levelStr string) {
	if levelStr == "" {
		return
	}
	level, err := logging.ParseLevel(levelStr)
	if err != nil {
		slog.Error("failed to parse log level, defaulting to info", "error", err)
		return
	}
	// set all loggers to the specified level
	logging.Reset(level)
	// set controller-runtime logger
	controllerLogger := logr.FromSlogHandler(logging.New("controller-runtime").Handler())
	ctrl.SetLogger(controllerLogger)
	// set klog logger
	klogLogger := logr.FromSlogHandler(logging.New("klog").Handler())
	klog.SetLogger(klogLogger)
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
