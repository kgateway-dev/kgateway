package setup

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/go-logr/logr"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	"istio.io/istio/pkg/security"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/common"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/admin"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/controller"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/namespaces"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

type Server interface {
	Start(ctx context.Context) error
}

func WithAPIClient(apiClient apiclient.Client) func(*setup) {
	return func(s *setup) {
		s.apiClient = apiClient
	}
}

func WithExtraInformerCacheSyncHandlers(handlers []cache.InformerSynced) func(*setup) {
	return func(s *setup) {
		s.extraInformerCacheSyncHandlers = handlers
	}
}

func WithGatewayControllerExtension(extension sdk.GatewayControllerExtension) func(*setup) {
	return func(s *setup) {
		s.gatewayControllerExtension = extension
	}
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

// WithLeaderElectionID sets the LeaderElectionID for the leader lease.
func WithLeaderElectionID(id string) func(*setup) {
	return func(s *setup) {
		s.leaderElectionID = id
	}
}

func WithHelmValuesGeneratorOverride(helmValuesGeneratorOverride func(inputs *deployer.Inputs) deployer.HelmValuesGenerator) func(*setup) {
	return func(s *setup) {
		s.helmValuesGeneratorOverride = helmValuesGeneratorOverride
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

func WithExtraManagerConfig(mgrConfigFuncs ...func(context.Context, manager.Manager, kubetypes.DynamicObjectFilter) error) func(*setup) {
	return func(s *setup) {
		s.extraManagerConfig = mgrConfigFuncs
	}
}

func WithExtraRunnables(runnables ...func(ctx context.Context, commoncol *collections.CommonCollections, s *apisettings.Settings) (bool, manager.Runnable)) func(*setup) {
	return func(s *setup) {
		s.extraRunnables = runnables
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

func WithCommonCollectionsOptions(commonCollectionsOptions []collections.Option) func(*setup) {
	return func(s *setup) {
		s.commonCollectionsOptions = commonCollectionsOptions
	}
}

func WithStatusSyncerOptions(statusSyncerOptions []proxy_syncer.StatusSyncerOption) func(*setup) {
	return func(s *setup) {
		s.statusSyncerOptions = statusSyncerOptions
	}
}

type setup struct {
	apiClient                      apiclient.Client
	extraInformerCacheSyncHandlers []cache.InformerSynced
	gatewayControllerExtension     sdk.GatewayControllerExtension
	gatewayControllerName          string
	gatewayClassName               string
	waypointClassName              string
	additionalGatewayClasses       map[string]*deployer.GatewayClassInfo
	extraPlugins                   func(ctx context.Context, commoncol *collections.CommonCollections, mergeSettingsJSON string) []sdk.Plugin
	helmValuesGeneratorOverride    func(inputs *deployer.Inputs) deployer.HelmValuesGenerator
	extraXDSCallbacks              xdsserver.Callbacks
	xdsListener                    net.Listener
	restConfig                     *rest.Config
	ctrlMgrOptionsInitFunc         func(context.Context) *ctrl.Options
	// extra controller manager config, like adding registering additional controllers
	extraManagerConfig []func(ctx context.Context, mgr manager.Manager, objectFilter kubetypes.DynamicObjectFilter) error
	// extra Runnable to add to the manager
	extraRunnables   []func(ctx context.Context, commoncol *collections.CommonCollections, settings *apisettings.Settings) (bool, manager.Runnable)
	krtDebugger      *krt.DebugHandler
	globalSettings   *apisettings.Settings
	leaderElectionID string
	validator        validator.Validator

	commonCollectionsOptions []collections.Option
	statusSyncerOptions      []proxy_syncer.StatusSyncerOption
}

var _ Server = &setup{}

// ensure global logger wiring happens once to avoid data races
var setLoggerOnce sync.Once

func New(opts ...func(*setup)) (*setup, error) {
	s := &setup{
		gatewayControllerName: wellknown.DefaultGatewayControllerName,
		gatewayClassName:      wellknown.DefaultGatewayClassName,
		waypointClassName:     wellknown.DefaultWaypointClassName,
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
	if s.apiClient == nil {
		apiClient, err := apiclient.New(s.restConfig)
		if err != nil {
			return nil, fmt.Errorf("error creating API client: %w", err)
		}
		s.apiClient = apiClient
	}

	leaderElectionID := s.leaderElectionID

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
				LeaderElectionNamespace: namespaces.GetPodNamespace(),
				LeaderElection:          !s.globalSettings.DisableLeaderElection,
				LeaderElectionID:        leaderElectionID,
			}
		}
	}

	if s.krtDebugger == nil {
		s.krtDebugger = new(krt.DebugHandler)
	}

	if s.globalSettings.EnableEnvoy && s.xdsListener == nil {
		var err error
		s.xdsListener, err = newXDSListener("0.0.0.0", s.globalSettings.XdsServicePort)
		if err != nil {
			slog.Error("error creating xds listener", "error", err)
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

	if err := schemes.AddToScheme(mgr.GetScheme()); err != nil {
		slog.Error("unable to extend scheme", "error", err)
		return err
	}

	uniqueClientCallbacks, uccBuilder := krtcollections.NewUniquelyConnectedClients(s.extraXDSCallbacks, s.globalSettings.XdsAuth)

	authenticators := []security.Authenticator{
		NewKubeJWTAuthenticator(s.apiClient.Kube()),
	}

	// Create shared certificate watcher if TLS is enabled. This watcher is used by both the xDS server
	// and the Gateway controller to kick reconciliation on cert changes.
	var certWatcher *certwatcher.CertWatcher
	if s.globalSettings.XdsTLS {
		var err error
		certWatcher, err = certwatcher.New(xds.TLSCertPath, xds.TLSKeyPath)
		if err != nil {
			return err
		}
		go func() {
			if err := certWatcher.Start(ctx); err != nil {
				slog.Error("failed to start TLS certificate watcher", "error", err)
			}
			slog.Info("started TLS certificate watcher")
		}()
	}

	// Only create Envoy control plane if Envoy controller is enabled
	var cache envoycache.SnapshotCache
	if s.globalSettings.EnableEnvoy {
		cache = NewControlPlane(ctx, s.xdsListener, uniqueClientCallbacks, authenticators, s.globalSettings.XdsAuth, certWatcher)
	}

	setupOpts := &controller.SetupOpts{
		Cache:          cache,
		KrtDebugger:    s.krtDebugger,
		GlobalSettings: s.globalSettings,
		CertWatcher:    certWatcher,
	}

	slog.Info("creating krt collections")
	krtOpts := krtutil.NewKrtOptions(ctx.Done(), setupOpts.KrtDebugger)

	commoncol, err := collections.NewCommonCollections(
		ctx,
		krtOpts,
		s.apiClient,
		s.gatewayControllerName,
		*s.globalSettings,
		s.commonCollectionsOptions...,
	)
	if err != nil {
		slog.Error("error creating common collections", "error", err)
		return err
	}

	for _, mgrCfgFunc := range s.extraManagerConfig {
		err := mgrCfgFunc(ctx, mgr, s.apiClient.ObjectFilter())
		if err != nil {
			return err
		}
	}

	runnablesRegistry := make(map[string]any)
	for _, runnable := range s.extraRunnables {
		enabled, r := runnable(ctx, commoncol, s.globalSettings)
		if !enabled {
			continue
		}
		if named, ok := r.(common.NamedRunnable); ok {
			runnablesRegistry[named.RunnableName()] = struct{}{}
		}
		if err := mgr.Add(r); err != nil {
			return fmt.Errorf("error adding extra Runnable to manager: %w", err)
		}
	}

	err = s.buildKgatewayWithConfig(ctx, mgr, setupOpts, commoncol, uccBuilder)
	if err != nil {
		return err
	}

	slog.Info("starting admin server")
	go admin.RunAdminServer(ctx, setupOpts)

	slog.Info("starting manager")
	return mgr.Start(ctx)
}

func newXDSListener(ip string, port uint32) (net.Listener, error) {
	bindAddr := net.TCPAddr{IP: net.ParseIP(ip), Port: int(port)}
	return net.Listen(bindAddr.Network(), bindAddr.String())
}

func (s *setup) buildKgatewayWithConfig(
	ctx context.Context,
	mgr manager.Manager,
	setupOpts *controller.SetupOpts,
	commonCollections *collections.CommonCollections,
	uccBuilder krtcollections.UniquelyConnectedClientsBulider,
) error {
	slog.Info("creating krt collections")
	krtOpts := krtutil.NewKrtOptions(ctx.Done(), setupOpts.KrtDebugger)

	augmentedPods, _ := krtcollections.NewPodsCollection(s.apiClient, krtOpts)
	augmentedPodsForUcc := augmentedPods
	if envutils.IsEnvTruthy("DISABLE_POD_LOCALITY_XDS") {
		augmentedPodsForUcc = nil
	}

	ucc := uccBuilder(ctx, krtOpts, augmentedPodsForUcc)

	gatewayClassInfos := controller.GetDefaultClassInfo(
		setupOpts.GlobalSettings,
		s.gatewayClassName,
		s.waypointClassName,
		s.gatewayControllerName,
		s.additionalGatewayClasses,
	)

	slog.Info("initializing controller")
	c, err := controller.NewControllerBuilder(ctx, controller.StartConfig{
		Manager:                     mgr,
		ControllerName:              s.gatewayControllerName,
		GatewayClassName:            s.gatewayClassName,
		WaypointGatewayClassName:    s.waypointClassName,
		AdditionalGatewayClasses:    s.additionalGatewayClasses,
		GatewayClassInfos:           gatewayClassInfos,
		ExtraPlugins:                s.extraPlugins,
		HelmValuesGeneratorOverride: s.helmValuesGeneratorOverride,
		RestConfig:                  s.restConfig,
		SetupOpts:                   setupOpts,
		Client:                      s.apiClient,
		AugmentedPods:               augmentedPods,
		UniqueClients:               ucc,
		Dev:                         logging.MustGetLevel(logging.DefaultComponent) <= logging.LevelTrace,
		KrtOptions:                  krtOpts,
		CommonCollections:           commonCollections,
		Validator:                   s.validator,
		GatewayControllerExtension:  s.gatewayControllerExtension,
		StatusSyncerOptions:         s.statusSyncerOptions,
	})
	if err != nil {
		slog.Error("failed initializing controller: ", "error", err)
		return err
	}

	slog.Info("waiting for cache sync")

	err = c.Build(ctx)
	if err != nil {
		return err
	}

	// RunAndWait must be called AFTER all Informers clients have been created
	s.apiClient.RunAndWait(ctx.Done())

	// Wait for extra Informer caches to sync
	s.apiClient.WaitForCacheSync("extra-informers", ctx.Done(), s.extraInformerCacheSyncHandlers...)

	return nil
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
