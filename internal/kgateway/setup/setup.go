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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/admin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/controller"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
)

type Server interface {
	Start(ctx context.Context) error
}

func WithGatewayControllerName(name string) func(*setup) {
	return func(s *setup) {
		s.gatewayControllerName = name
	}
}

func WithGatewayClassName(name string) func(*setup) {
	return func(s *setup) {
		s.gatewayClassName = name
	}
}

func WithWaypointClassName(name string) func(*setup) {
	return func(s *setup) {
		s.waypointClassName = name
	}
}

func WithAgentGatewayClassName(name string) func(*setup) {
	return func(s *setup) {
		s.agentGatewayClassName = name
	}
}

func WithExtraPlugins(extraPlugins func(ctx context.Context, commoncol *common.CommonCollections) []sdk.Plugin) func(*setup) {
	return func(s *setup) {
		s.extraPlugins = extraPlugins
	}
}

func ExtraGatewayParameters(extraGatewayParameters func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters) func(*setup) {
	return func(s *setup) {
		s.extraGatewayParameters = extraGatewayParameters
	}
}

func AddToScheme(addToScheme func(s *runtime.Scheme) error) func(s *setup) {
	return func(s *setup) {
		s.addToScheme = addToScheme
	}
}

func WithExtraXDSCallbacks(extraXDSCallbacks xdsserver.Callbacks) func(*setup) {
	return func(s *setup) {
		s.extraXDSCallbacks = extraXDSCallbacks
	}
}

type setup struct {
	gatewayControllerName  string
	gatewayClassName       string
	waypointClassName      string
	agentGatewayClassName  string
	extraPlugins           func(ctx context.Context, commoncol *common.CommonCollections) []sdk.Plugin
	extraGatewayParameters func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters
	addToScheme            func(s *runtime.Scheme) error
	extraXDSCallbacks      xdsserver.Callbacks
}

var _ Server = &setup{}

func New(opts ...func(*setup)) *setup {
	s := &setup{
		gatewayControllerName: wellknown.DefaultGatewayControllerName,
		gatewayClassName:      wellknown.DefaultGatewayClassName,
		waypointClassName:     wellknown.DefaultWaypointClassName,
		agentGatewayClassName: wellknown.DefaultAgentGatewayClassName,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *setup) Start(ctx context.Context) error {
	if s.extraXDSCallbacks != nil {
		return StartKgatewayWithXDSCallbacks(ctx, s.gatewayControllerName, s.gatewayClassName, s.waypointClassName, s.agentGatewayClassName, s.extraPlugins, s.extraGatewayParameters, s.addToScheme, s.extraXDSCallbacks)
	}

	return StartKgateway(ctx, s.gatewayControllerName, s.gatewayClassName, s.waypointClassName, s.agentGatewayClassName, s.extraPlugins, s.extraGatewayParameters, s.addToScheme)
}

func StartKgateway(
	ctx context.Context,
	gatewayControllerName string,
	gatewayClassName string,
	waypointClassName string,
	agentGatewayClassName string,
	extraPlugins func(ctx context.Context, commoncol *common.CommonCollections) []sdk.Plugin,
	extraGatewayParameters func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters,
	addToScheme func(s *runtime.Scheme) error,
) error {
	return StartKgatewayWithXDSCallbacks(ctx, gatewayControllerName, gatewayClassName, waypointClassName, agentGatewayClassName, extraPlugins, extraGatewayParameters, addToScheme, nil)
}

func StartKgatewayWithXDSCallbacks(ctx context.Context,
	gatewayControllerName string,
	gatewayClassName string,
	waypointClassName string,
	agentGatewayClassName string,
	extraPlugins func(ctx context.Context, commoncol *common.CommonCollections) []sdk.Plugin,
	extraGatewayParameters func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters,
	addToScheme func(s *runtime.Scheme) error,
	extraXDSCallbacks xdsserver.Callbacks,
) error {
	// load global settings
	st, err := settings.BuildSettings()
	if err != nil {
		slog.Error("error loading settings from env", "error", err)
	}

	setupLogging(st.LogLevel)
	slog.Info("global settings loaded", "settings", *st)

	uniqueClientCallbacks, uccBuilder := krtcollections.NewUniquelyConnectedClients(extraXDSCallbacks)
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
	return StartKgatewayWithConfig(
		ctx,
		gatewayControllerName,
		gatewayClassName,
		waypointClassName,
		agentGatewayClassName,
		setupOpts,
		restConfig,
		uccBuilder,
		extraPlugins,
		extraGatewayParameters,
		addToScheme,
	)
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
	gatewayControllerName string,
	gatewayClassName string,
	waypointClassName string,
	agentGatewayClassName string,
	setupOpts *controller.SetupOpts,
	restConfig *rest.Config,
	uccBuilder krtcollections.UniquelyConnectedClientsBulider,
	extraPlugins func(ctx context.Context, commoncol *common.CommonCollections) []sdk.Plugin,
	extraGatewayParameters func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters,
	addToScheme func(s *runtime.Scheme) error,
) error {
	slog.Info("starting kgateway")

	kubeClient, err := CreateKubeClient(restConfig)
	if err != nil {
		return err
	}

	metrics.SetRegistry(setupOpts.GlobalSettings.EnableBuiltinDefaultMetrics, nil)
	metrics.SetActive(!(setupOpts.MetricsBindAddress == "" || setupOpts.MetricsBindAddress == "0"))

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
		ControllerName:           gatewayControllerName,
		GatewayClassName:         gatewayClassName,
		WaypointGatewayClassName: waypointClassName,
		AgentGatewayClassName:    agentGatewayClassName,
		ExtraPlugins:             extraPlugins,
		ExtraGatewayParameters:   extraGatewayParameters,
		AddToScheme:              addToScheme,
		RestConfig:               restConfig,
		SetupOpts:                setupOpts,
		Client:                   kubeClient,
		AugmentedPods:            augmentedPods,
		UniqueClients:            ucc,
		Dev:                      logging.MustGetLevel(logging.DefaultComponent) <= logging.LevelTrace,
		KrtOptions:               krtOpts,
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

func CreateKubeClient(restConfig *rest.Config) (istiokube.Client, error) {
	restCfg := istiokube.NewClientConfigForRestConfig(restConfig)
	client, err := istiokube.NewClient(restCfg, "")
	if err != nil {
		return nil, err
	}
	istiokube.EnableCrdWatcher(client)
	return client, nil
}
