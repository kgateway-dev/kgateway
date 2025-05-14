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
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/internal/version"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
)

const (
	componentName = "kgateway"
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
	setupLogging(ctx, componentName)
	return StartKgateway(ctx, s.extraPlugins)
}

func StartKgateway(
	ctx context.Context,
	extraPlugins func(ctx context.Context, commoncol *common.CommonCollections) []sdk.Plugin,
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
		// TODO: why do we plumb this through if it's wellknown?
		ControllerName: wellknown.GatewayControllerName,
		ExtraPlugins:   extraPlugins,
		RestConfig:     restConfig,
		SetupOpts:      setupOpts,
		Client:         kubeClient,
		AugmentedPods:  augmentedPods,
		UniqueClients:  ucc,

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

// setupLogging sets up controller-runtime logging
func setupLogging(ctx context.Context, loggerName string) {
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

func createKubeClient(restConfig *rest.Config) (istiokube.Client, error) {
	restCfg := istiokube.NewClientConfigForRestConfig(restConfig)
	client, err := istiokube.NewClient(restCfg, "")
	if err != nil {
		return nil, err
	}
	istiokube.EnableCrdWatcher(client)
	return client, nil
}
