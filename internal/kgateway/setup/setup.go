package setup

import (
	"context"
	"log/slog"
	"net"

	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/go-logr/logr"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/admin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/controller"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	agentgatewayplugins "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
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

func WithExtraAgentgatewayPlugins(extraAgentgatewayPlugins func(ctx context.Context, agw *agentgatewayplugins.AgwCollections) []agentgatewayplugins.PolicyPlugin) func(*setup) {
	return func(s *setup) {
		s.extraAgentgatewayPlugins = extraAgentgatewayPlugins
	}
}

// WithLeaderElectionID sets the LeaderElectionID for the leader lease.
func WithLeaderElectionID(id string) func(*setup) {
	return func(s *setup) {
		s.leaderElectionID = id
	}
}

func ExtraGatewayParameters(extraGatewayParameters func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters) func(*setup) {
	return func(s *setup) {
		s.extraGatewayParameters = extraGatewayParameters
	}
}

func WithRestConfig(rc *rest.Config) func(*setup) {
	return func(s *setup) {
		s.restConfig = rc
	}
}

func WithControllerManagerOptions(f func(context.Context) *ctrl.Options) func(*setup) {
	return func(s *setup) {
		s.ctrlMgrOptionsInitFunc = f
	}
}

func WithExtraXDSCallbacks(extraXDSCallbacks xdsserver.Callbacks) func(*setup) {
	return func(s *setup) {
		s.extraXDSCallbacks = extraXDSCallbacks
	}
}

// used for tests only to get access to dynamically assigned port number
func WithXDSListener(l net.Listener) func(*setup) {
	return func(s *setup) {
		s.xdsListener = l
	}
}

func WithExtraManagerConfig(mgrConfigFuncs ...func(ctx context.Context, mgr manager.Manager, objectFilter kubetypes.DynamicObjectFilter) error) func(*setup) {
	return func(s *setup) {
		s.extraManagerConfig = mgrConfigFuncs
	}
}

func WithKrtDebugger(dbg *krt.DebugHandler) func(*setup) {
	return func(s *setup) {
		s.krtDebugger = dbg
	}
}

func WithGlobalSettings(settings *settings.Settings) func(*setup) {
	return func(s *setup) {
		s.globalSettings = settings
	}
}

type setup struct {
	gatewayControllerName    string
	gatewayClassName         string
	waypointClassName        string
	agentGatewayClassName    string
	extraPlugins             func(ctx context.Context, commoncol *common.CommonCollections) []sdk.Plugin
	extraAgentgatewayPlugins func(ctx context.Context, agw *agentgatewayplugins.AgwCollections) []agentgatewayplugins.PolicyPlugin
	extraGatewayParameters   func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters
	extraXDSCallbacks        xdsserver.Callbacks
	xdsListener              net.Listener
	restConfig               *rest.Config
	ctrlMgrOptionsInitFunc   func(context.Context) *ctrl.Options
	// extra controller manager config, like adding registering additional controllers
	extraManagerConfig []func(ctx context.Context, mgr manager.Manager, objectFilter kubetypes.DynamicObjectFilter) error
	krtDebugger        *krt.DebugHandler
	globalSettings     *settings.Settings
	leaderElectionID   string
}

var _ Server = &setup{}

func New(opts ...func(*setup)) (*setup, error) {
	s := &setup{
		gatewayControllerName: wellknown.DefaultGatewayControllerName,
		gatewayClassName:      wellknown.DefaultGatewayClassName,
		waypointClassName:     wellknown.DefaultWaypointClassName,
		agentGatewayClassName: wellknown.DefaultAgentGatewayClassName,
		leaderElectionID:      wellknown.LeaderElectionID,
	}
	for _, opt := range opts {
		opt(s)
	}

	if s.globalSettings == nil {
		var err error
		s.globalSettings, err = settings.BuildSettings()
		if err != nil {
			slog.Error("error loading settings from env", "error", err)
			return nil, err
		}
	}

	if s.restConfig == nil {
		s.restConfig = ctrl.GetConfigOrDie()
	}

	if s.ctrlMgrOptionsInitFunc == nil {
		s.ctrlMgrOptionsInitFunc = func(ctx context.Context) *ctrl.Options {
			return &ctrl.Options{
				BaseContext:      func() context.Context { return ctx },
				Scheme:           runtime.NewScheme(),
				PprofBindAddress: "127.0.0.1:9099",
				// if you change the port here, also change the port "health" in the helmchart.
				HealthProbeBindAddress: ":9093",
				Metrics: metricsserver.Options{
					BindAddress: ":9092",
				},
				LeaderElection:   !s.globalSettings.DisableLeaderElection,
				LeaderElectionID: s.leaderElectionID,
			}
		}
	}

	if s.krtDebugger == nil {
		s.krtDebugger = new(krt.DebugHandler)
	}

	if s.xdsListener == nil {
		var err error
		s.xdsListener, err = newXDSListener("0.0.0.0", s.globalSettings.XdsServicePort)
		if err != nil {
			slog.Error("error creating xds listener", "error", err)
			return nil, err
		}
	}

	return s, nil
}

func (s *setup) Start(ctx context.Context) error {
	slog.Info("starting kgateway")

	SetupLogging(s.globalSettings.LogLevel)

	mgrOpts := s.ctrlMgrOptionsInitFunc(ctx)

	metrics.SetRegistry(s.globalSettings.EnableBuiltinDefaultMetrics, nil)
	metrics.SetActive(!(mgrOpts.Metrics.BindAddress == "" || mgrOpts.Metrics.BindAddress == "0"))

	mgr, err := ctrl.NewManager(s.restConfig, *mgrOpts)
	if err != nil {
		return err
	}

	if err := controller.AddToScheme(mgr.GetScheme()); err != nil {
		slog.Error("unable to extend scheme", "error", err)
		return err
	}

	uniqueClientCallbacks, uccBuilder := krtcollections.NewUniquelyConnectedClients(s.extraXDSCallbacks)
	cache := NewControlPlane(ctx, s.xdsListener, uniqueClientCallbacks)

	setupOpts := &controller.SetupOpts{
		Cache:          cache,
		KrtDebugger:    s.krtDebugger,
		GlobalSettings: s.globalSettings,
	}

	istioClient, err := CreateKubeClient(s.restConfig)
	if err != nil {
		return err
	}

	cli, err := versioned.NewForConfig(s.restConfig)
	if err != nil {
		return err
	}

	slog.Info("creating krt collections")
	krtOpts := krtutil.NewKrtOptions(ctx.Done(), setupOpts.KrtDebugger)

	commoncol, err := collections.NewCommonCollections(
		ctx,
		krtOpts,
		istioClient,
		cli,
		mgr.GetClient(),
		s.gatewayControllerName,
		*s.globalSettings,
	)
	if err != nil {
		slog.Error("error creating common collections", "error", err)
		return err
	}

	agwCollections, err := agentgatewayplugins.NewAgwCollections(
		commoncol,
	)
	if err != nil {
		slog.Error("error creating agw common collections", "error", err)
		return err
	}

	for _, mgrCfgFunc := range s.extraManagerConfig {
		err := mgrCfgFunc(ctx, mgr, commoncol.DiscoveryNamespacesFilter)
		if err != nil {
			return err
		}
	}

	BuildKgatewayWithConfig(
		ctx, mgr, s.gatewayControllerName, s.gatewayClassName, s.waypointClassName,
		s.agentGatewayClassName, setupOpts, s.restConfig, istioClient, commoncol, agwCollections, uccBuilder, s.extraPlugins, s.extraAgentgatewayPlugins, s.extraGatewayParameters)

	slog.Info("starting admin server")
	go admin.RunAdminServer(ctx, setupOpts)

	slog.Info("starting manager")
	return mgr.Start(ctx)
}

func newXDSListener(ip string, port uint32) (net.Listener, error) {
	bindAddr := net.TCPAddr{IP: net.ParseIP(ip), Port: int(port)}
	return net.Listen(bindAddr.Network(), bindAddr.String())
}

func BuildKgatewayWithConfig(
	ctx context.Context,
	mgr manager.Manager,
	gatewayControllerName string,
	gatewayClassName string,
	waypointClassName string,
	agentGatewayClassName string,
	setupOpts *controller.SetupOpts,
	restConfig *rest.Config,
	kubeClient istiokube.Client,
	commonCollections *collections.CommonCollections,
	agwCollections *agentgatewayplugins.AgwCollections,
	uccBuilder krtcollections.UniquelyConnectedClientsBulider,
	extraPlugins func(ctx context.Context, commoncol *common.CommonCollections) []sdk.Plugin,
	extraAgentgatewayPlugins func(ctx context.Context, agw *agentgatewayplugins.AgwCollections) []agentgatewayplugins.PolicyPlugin,
	extraGatewayParameters func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters,
) error {
	slog.Info("creating krt collections")
	krtOpts := krtutil.NewKrtOptions(ctx.Done(), setupOpts.KrtDebugger)

	augmentedPods, _ := krtcollections.NewPodsCollection(kubeClient, krtOpts)
	augmentedPodsForUcc := augmentedPods
	if envutils.IsEnvTruthy("DISABLE_POD_LOCALITY_XDS") {
		augmentedPodsForUcc = nil
	}

	ucc := uccBuilder(ctx, krtOpts, augmentedPodsForUcc)

	slog.Info("initializing controller")
	c, err := controller.NewControllerBuilder(ctx, controller.StartConfig{
		Manager:                  mgr,
		ControllerName:           gatewayControllerName,
		GatewayClassName:         gatewayClassName,
		WaypointGatewayClassName: waypointClassName,
		AgentGatewayClassName:    agentGatewayClassName,
		ExtraPlugins:             extraPlugins,
		ExtraAgentgatewayPlugins: extraAgentgatewayPlugins,
		ExtraGatewayParameters:   extraGatewayParameters,
		RestConfig:               restConfig,
		SetupOpts:                setupOpts,
		Client:                   kubeClient,
		AugmentedPods:            augmentedPods,
		UniqueClients:            ucc,
		Dev:                      logging.MustGetLevel(logging.DefaultComponent) <= logging.LevelTrace,
		KrtOptions:               krtOpts,
		CommonCollections:        commonCollections,
		AgwCollections:           agwCollections,
	})
	if err != nil {
		slog.Error("failed initializing controller: ", "error", err)
		return err
	}

	slog.Info("waiting for cache sync")
	kubeClient.RunAndWait(ctx.Done())

	return c.Build(ctx)
}

// SetupLogging configures the global slog logger
func SetupLogging(levelStr string) {
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
