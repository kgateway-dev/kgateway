package setup

import (
	"context"
	"log/slog"
	"net"
	"sync"

	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/go-logr/logr"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	"istio.io/istio/pkg/security"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/namespaces"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/admin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/controller"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	agwplugins "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

type Server interface {
	Start(ctx context.Context) error
}

func WithGatewayControllerName(name string) func(*setup) {
	return func(s *setup) {
		s.gatewayControllerName = name
	}
}

func WithAgwControllerName(name string) func(*setup) {
	return func(s *setup) {
		s.agwControllerName = name
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

func WithAgentgatewayClassName(name string) func(*setup) {
	return func(s *setup) {
		s.agentgatewayClassName = name
	}
}

func WithAdditionalGatewayClasses(classes map[string]*deployer.GatewayClassInfo) func(*setup) {
	return func(s *setup) {
		s.additionalGatewayClasses = classes
	}
}

func WithExtraPlugins(extraPlugins func(ctx context.Context, commoncol *collections.CommonCollections, mergeSettingsJSON string) []sdk.Plugin) func(*setup) {
	return func(s *setup) {
		s.extraPlugins = extraPlugins
	}
}

func WithExtraAgwPlugins(extraAgwPlugins func(ctx context.Context, agw *agwplugins.AgwCollections) []agwplugins.AgwPlugin) func(*setup) {
	return func(s *setup) {
		s.extraAgwPlugins = extraAgwPlugins
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

// used for tests only to get access to dynamically assigned port number
func WithAgwXDSListener(l net.Listener) func(*setup) {
	return func(s *setup) {
		s.agwXdsListener = l
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

func WithGlobalSettings(settings *apisettings.Settings) func(*setup) {
	return func(s *setup) {
		s.globalSettings = settings
	}
}

func WithValidator(v validator.Validator) func(*setup) {
	return func(s *setup) {
		s.validator = v
	}
}

func WithExtraAgwPolicyStatusHandlers(handlers map[string]agwplugins.AgwPolicyStatusSyncHandler) func(*setup) {
	return func(s *setup) {
		s.extraAgwPolicyStatusHandlers = handlers
	}
}

type setup struct {
	gatewayControllerName    string
	agwControllerName        string
	gatewayClassName         string
	waypointClassName        string
	agentgatewayClassName    string
	additionalGatewayClasses map[string]*deployer.GatewayClassInfo
	extraPlugins             func(ctx context.Context, commoncol *collections.CommonCollections, mergeSettingsJSON string) []sdk.Plugin
	extraAgwPlugins          func(ctx context.Context, agw *agwplugins.AgwCollections) []agwplugins.AgwPlugin
	extraGatewayParameters   func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters
	extraXDSCallbacks        xdsserver.Callbacks
	xdsListener              net.Listener
	agwXdsListener           net.Listener
	restConfig               *rest.Config
	ctrlMgrOptionsInitFunc   func(context.Context) *ctrl.Options
	// extra controller manager config, like adding registering additional controllers
	extraManagerConfig           []func(ctx context.Context, mgr manager.Manager, objectFilter kubetypes.DynamicObjectFilter) error
	krtDebugger                  *krt.DebugHandler
	globalSettings               *apisettings.Settings
	leaderElectionID             string
	validator                    validator.Validator
	extraAgwPolicyStatusHandlers map[string]agwplugins.AgwPolicyStatusSyncHandler
}

var _ Server = &setup{}

// ensure global logger wiring happens once to avoid data races
var setLoggerOnce sync.Once

func New(opts ...func(*setup)) (*setup, error) {
	s := &setup{
		gatewayControllerName: wellknown.DefaultGatewayControllerName,
		agwControllerName:     wellknown.DefaultAgwControllerName,
		gatewayClassName:      wellknown.DefaultGatewayClassName,
		waypointClassName:     wellknown.DefaultWaypointClassName,
		agentgatewayClassName: wellknown.DefaultAgwClassName,
		leaderElectionID:      wellknown.LeaderElectionID,
	}
	for _, opt := range opts {
		opt(s)
	}

	if s.globalSettings == nil {
		var err error
		s.globalSettings, err = apisettings.BuildSettings()
		if err != nil {
			slog.Error("error loading settings from env", "error", err)
			return nil, err
		}
	}

	SetupLogging(s.globalSettings.LogLevel)

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

	if s.agwXdsListener == nil {
		var err error
		s.agwXdsListener, err = newXDSListener("0.0.0.0", s.globalSettings.AgentgatewayXdsServicePort)
		if err != nil {
			slog.Error("error creating agw xds listener", "error", err)
			return nil, err
		}
	}

	if s.validator == nil {
		s.validator = validator.NewBinary()
	}

	return s, nil
}

func (s *setup) Start(ctx context.Context) error {
	slog.Info("starting kgateway")

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

	uniqueClientCallbacks, uccBuilder := krtcollections.NewUniquelyConnectedClients(s.extraXDSCallbacks, s.globalSettings.XdsAuth)

	istioClient, err := CreateKubeClient(s.restConfig)
	if err != nil {
		return err
	}

	cli, err := versioned.NewForConfig(s.restConfig)
	if err != nil {
		return err
	}

	authenticators := []security.Authenticator{
		NewKubeJWTAuthenticator(istioClient.Kube()),
	}

	cache := NewControlPlane(ctx, s.xdsListener, s.agwXdsListener, uniqueClientCallbacks, authenticators, s.globalSettings.XdsAuth)

	setupOpts := &controller.SetupOpts{
		Cache:          cache,
		KrtDebugger:    s.krtDebugger,
		GlobalSettings: s.globalSettings,
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

	agwCollections, err := agwplugins.NewAgwCollections(
		commoncol,
		s.agwControllerName,
		// control plane system namespace (default is kgateway-system)
		namespaces.GetPodNamespace(),
		istioClient.ClusterID().String(),
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
		ctx, mgr, s.gatewayControllerName, s.agwControllerName, s.gatewayClassName, s.waypointClassName,
		s.agentgatewayClassName, s.additionalGatewayClasses, setupOpts, s.restConfig,
		istioClient, commoncol, agwCollections, uccBuilder, s.extraPlugins, s.extraAgwPlugins,
		s.extraGatewayParameters,
		s.validator,
		s.extraAgwPolicyStatusHandlers,
	)

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
	agwControllerName string,
	gatewayClassName string,
	waypointClassName string,
	agentgatewayClassName string,
	additionalGatewayClasses map[string]*deployer.GatewayClassInfo,
	setupOpts *controller.SetupOpts,
	restConfig *rest.Config,
	kubeClient istiokube.Client,
	commonCollections *collections.CommonCollections,
	agwCollections *agwplugins.AgwCollections,
	uccBuilder krtcollections.UniquelyConnectedClientsBulider,
	extraPlugins func(ctx context.Context, commoncol *collections.CommonCollections, mergeSettingsJSON string) []sdk.Plugin,
	extraAgwPlugins func(ctx context.Context, agw *agwplugins.AgwCollections) []agwplugins.AgwPlugin,
	extraGatewayParameters func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters,
	validator validator.Validator,
	extraAgwPolicyStatusHandlers map[string]agwplugins.AgwPolicyStatusSyncHandler,
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
		Manager:                      mgr,
		ControllerName:               gatewayControllerName,
		AgwControllerName:            agwControllerName,
		GatewayClassName:             gatewayClassName,
		WaypointGatewayClassName:     waypointClassName,
		AgentgatewayClassName:        agentgatewayClassName,
		AdditionalGatewayClasses:     additionalGatewayClasses,
		ExtraPlugins:                 extraPlugins,
		ExtraAgwPlugins:              extraAgwPlugins,
		ExtraGatewayParameters:       extraGatewayParameters,
		RestConfig:                   restConfig,
		SetupOpts:                    setupOpts,
		Client:                       kubeClient,
		AugmentedPods:                augmentedPods,
		UniqueClients:                ucc,
		Dev:                          logging.MustGetLevel(logging.DefaultComponent) <= logging.LevelTrace,
		KrtOptions:                   krtOpts,
		CommonCollections:            commonCollections,
		AgwCollections:               agwCollections,
		Validator:                    validator,
		ExtraAgwPolicyStatusHandlers: extraAgwPolicyStatusHandlers,
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
	level, err := logging.ParseLevel(levelStr)
	if err != nil {
		slog.Error("failed to parse log level, defaulting to info", "error", err)
		level = slog.LevelInfo
	}
	// set all loggers to the specified level
	logging.Reset(level)
	// set controller-runtime and klog loggers only once to avoid data races with concurrent readers
	setLoggerOnce.Do(func() {
		controllerLogger := logr.FromSlogHandler(logging.New("controller-runtime").Handler())
		ctrl.SetLogger(controllerLogger)
		klogLogger := logr.FromSlogHandler(logging.New("klog").Handler())
		klog.SetLogger(klogLogger)
	})
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
