package controller

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	istiolog "istio.io/istio/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/inferenceextension/endpointpicker"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/waypoint"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/registry"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections/metrics"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/proxy_syncer"
	agwplugins "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	kgtwschemes "github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/namespaces"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

const (
	// AutoProvision controls whether the controller will be responsible for provisioning dynamic
	// infrastructure for the Gateway API.
	AutoProvision = true
)

type SetupOpts struct {
	Cache envoycache.SnapshotCache

	KrtDebugger *krt.DebugHandler

	// static set of global Settings
	GlobalSettings *apisettings.Settings

	PprofBindAddress       string
	HealthProbeBindAddress string
	MetricsBindAddress     string
}

var setupLog = ctrl.Log.WithName("setup")

type StartConfig struct {
	Manager                  manager.Manager
	ControllerName           string
	AgwControllerName        string
	GatewayClassName         string
	WaypointGatewayClassName string
	AgentgatewayClassName    string
	AdditionalGatewayClasses map[string]*deployer.GatewayClassInfo

	Dev        bool
	SetupOpts  *SetupOpts
	RestConfig *rest.Config
	// ExtensionsFactory is the factory function which will return an extensions.K8sGatewayExtensions
	// This is responsible for producing the extension points that this controller requires
	ExtraPlugins           func(ctx context.Context, commoncol *collections.CommonCollections, mergeSettingsJSON string) []sdk.Plugin
	ExtraAgwPlugins        func(ctx context.Context, agw *agwplugins.AgwCollections) []agwplugins.AgwPlugin
	ExtraGatewayParameters func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters
	Client                 istiokube.Client
	Validator              validator.Validator

	AgwCollections    *agwplugins.AgwCollections
	CommonCollections *collections.CommonCollections
	AugmentedPods     krt.Collection[krtcollections.LocalityPod]
	UniqueClients     krt.Collection[ir.UniqlyConnectedClient]

	KrtOptions                   krtutil.KrtOptions
	ExtraAgwPolicyStatusHandlers map[string]agwplugins.AgwPolicyStatusSyncHandler
}

// Start runs the controllers responsible for processing the K8s Gateway API objects
// It is intended to be run in a goroutine as the function will block until the supplied
// context is cancelled
type ControllerBuilder struct {
	proxySyncer *proxy_syncer.ProxySyncer
	agwSyncer   *agentgatewaysyncer.Syncer
	cfg         StartConfig
	mgr         ctrl.Manager
	commoncol   *collections.CommonCollections

	ready atomic.Bool
}

func NewControllerBuilder(ctx context.Context, cfg StartConfig) (*ControllerBuilder, error) {
	loggingOptions := istiolog.DefaultOptions()
	loggingOptions.JSONEncoding = true
	if cfg.Dev {
		setupLog.Info("starting log in dev mode")
		loggingOptions.SetDefaultOutputLevel(istiolog.OverrideScopeName, istiolog.DebugLevel)
	}
	istiolog.Configure(loggingOptions)

	setupLog.Info("initializing kgateway extensions")

	var gatedPlugins []sdk.Plugin
	// Extend the scheme and add the EPP plugin if the inference extension is enabled and the InferencePool CRD exists.
	if cfg.SetupOpts.GlobalSettings.EnableInferExt {
		exists, err := kgtwschemes.AddInferExtV1Scheme(cfg.RestConfig, cfg.Manager.GetScheme())
		switch {
		case err != nil:
			return nil, err
		case exists:
			setupLog.Info("adding the endpoint-picker inference extension")
			gatedPlugins = append(gatedPlugins, endpointpicker.NewPlugin(ctx, cfg.CommonCollections))
		}
	}
	// Add the waypoint plugin if enabled
	if cfg.SetupOpts.GlobalSettings.EnableWaypoint {
		setupLog.Info("adding the waypoint plugin")
		gatedPlugins = append(gatedPlugins, waypoint.NewPlugin(ctx, cfg.CommonCollections, cfg.WaypointGatewayClassName))
	}
	// Append the gatedPlugins to the ExtraPlugins
	if len(gatedPlugins) != 0 {
		existingExtraPlugins := cfg.ExtraPlugins
		cfg.ExtraPlugins = func(ctx context.Context, commoncol *collections.CommonCollections, mergeSettingsJSON string) []sdk.Plugin {
			if existingExtraPlugins != nil {
				return append(gatedPlugins, existingExtraPlugins(ctx, commoncol, cfg.SetupOpts.GlobalSettings.PolicyMerge)...)
			}
			return gatedPlugins
		}
	}

	globalSettings := *cfg.SetupOpts.GlobalSettings
	mergedPlugins := pluginFactoryWithBuiltin(cfg)(ctx, cfg.CommonCollections)
	cfg.CommonCollections.InitPlugins(ctx, mergedPlugins, globalSettings)

	// Begin background processing of resource sync metrics.
	// This only effects metrics in the resources subsystem and is not required for other metrics.
	metrics.StartResourceSyncMetricsProcessing(ctx)

	// Create the proxy syncer for the Gateway API resources
	setupLog.Info("initializing proxy syncer")
	proxySyncer := proxy_syncer.NewProxySyncer(
		ctx,
		cfg.ControllerName,
		cfg.Manager,
		cfg.Client,
		cfg.UniqueClients,
		mergedPlugins,
		cfg.CommonCollections,
		cfg.SetupOpts.Cache,
		cfg.AgentgatewayClassName,
		cfg.Validator,
	)
	proxySyncer.Init(ctx, cfg.KrtOptions)
	if err := cfg.Manager.Add(proxySyncer); err != nil {
		setupLog.Error(err, "unable to add proxySyncer runnable")
		return nil, err
	}

	statusSyncer := proxy_syncer.NewStatusSyncer(
		cfg.Manager,
		mergedPlugins,
		cfg.ControllerName,
		cfg.AgentgatewayClassName,
		cfg.Client,
		cfg.CommonCollections,
		proxySyncer.ReportQueue(),
		proxySyncer.BackendPolicyReportQueue(),
		proxySyncer.CacheSyncs(),
	)
	if err := cfg.Manager.Add(statusSyncer); err != nil {
		setupLog.Error(err, "unable to add statusSyncer runnable")
		return nil, err
	}

	var agwSyncer *agentgatewaysyncer.Syncer
	if cfg.SetupOpts.GlobalSettings.EnableAgentgateway {
		agwMergedPlugins := agwPluginFactory(cfg)(ctx, cfg.AgwCollections)

		agwSyncer = agentgatewaysyncer.NewAgwSyncer(
			cfg.AgwControllerName,
			cfg.AgentgatewayClassName,
			cfg.Client,
			cfg.Manager,
			cfg.AgwCollections,
			agwMergedPlugins,
			cfg.SetupOpts.Cache,
			cfg.SetupOpts.GlobalSettings.EnableInferExt,
		)
		agwSyncer.Init(cfg.KrtOptions)

		if err := cfg.Manager.Add(agwSyncer); err != nil {
			setupLog.Error(err, "unable to add agentgateway Syncer runnable")
			return nil, err
		}

		agwStatusSyncer := agentgatewaysyncer.NewAgwStatusSyncer(
			cfg.AgwControllerName,
			cfg.AgentgatewayClassName,
			cfg.Client,
			cfg.Manager,
			agwSyncer.GatewayReportQueue(),
			agwSyncer.ListenerSetReportQueue(),
			agwSyncer.RouteReportQueue(),
			agwSyncer.PolicyStatusQueue(),
			agwSyncer.CacheSyncs(),
			cfg.ExtraAgwPolicyStatusHandlers,
		)
		if err := cfg.Manager.Add(agwStatusSyncer); err != nil {
			setupLog.Error(err, "unable to add agentgateway StatusSyncer runnable")
			return nil, err
		}
	}

	setupLog.Info("starting controller builder")
	cb := &ControllerBuilder{
		proxySyncer: proxySyncer,
		agwSyncer:   agwSyncer,
		cfg:         cfg,
		mgr:         cfg.Manager,
		commoncol:   cfg.CommonCollections,
	}

	// wait for the ControllerBuilder to Start
	// as well as its subcomponents (mainly ProxySyncer) before marking ready
	if err := cfg.Manager.AddReadyzCheck("ready-ping", func(_ *http.Request) error {
		if !cb.HasSynced() {
			return errors.New("not synced")
		}
		return nil
	}); err != nil {
		setupLog.Error(err, "failed setting up healthz")
	}

	return cb, nil
}

func pluginFactoryWithBuiltin(cfg StartConfig) extensions2.K8sGatewayExtensionsFactory {
	return func(ctx context.Context, commoncol *collections.CommonCollections) sdk.Plugin {
		plugins := registry.Plugins(
			ctx,
			commoncol,
			cfg.WaypointGatewayClassName,
			*cfg.SetupOpts.GlobalSettings,
			cfg.Validator,
		)
		plugins = append(plugins, krtcollections.NewBuiltinPlugin(ctx))
		if cfg.ExtraPlugins != nil {
			plugins = append(plugins, cfg.ExtraPlugins(ctx, commoncol, cfg.SetupOpts.GlobalSettings.PolicyMerge)...)
		}
		return registry.MergePlugins(plugins...)
	}
}

func agwPluginFactory(cfg StartConfig) func(ctx context.Context, agw *agwplugins.AgwCollections) agwplugins.AgwPlugin {
	return func(ctx context.Context, agw *agwplugins.AgwCollections) agwplugins.AgwPlugin {
		plugins := agwplugins.Plugins(agw)
		if cfg.ExtraAgwPlugins != nil {
			plugins = append(plugins, cfg.ExtraAgwPlugins(ctx, agw)...)
		}
		return agwplugins.MergePlugins(plugins...)
	}
}

func (c *ControllerBuilder) Build(ctx context.Context) error {
	slog.Info("creating gateway controllers")

	globalSettings := c.cfg.SetupOpts.GlobalSettings

	xdsHost := globalSettings.XdsServiceHost
	if xdsHost == "" {
		xdsHost = kubeutils.ServiceFQDN(metav1.ObjectMeta{
			Name:      globalSettings.XdsServiceName,
			Namespace: namespaces.GetPodNamespace(),
		})
	}

	xdsPort := globalSettings.XdsServicePort
	slog.Info("got xds address for deployer", "xds_host", xdsHost, "xds_port", xdsPort)

	agwXdsPort := globalSettings.AgentgatewayXdsServicePort
	slog.Info("got agentgateway xds address for deployer", "agw_xds_host", xdsHost, "agw_xds_port", agwXdsPort)

	istioAutoMtlsEnabled := globalSettings.EnableIstioAutoMtls

	gwCfg := GatewayConfig{
		Mgr:               c.mgr,
		ControllerName:    c.cfg.ControllerName,
		AgwControllerName: c.cfg.AgwControllerName,
		AutoProvision:     AutoProvision,
		ControlPlane: deployer.ControlPlaneInfo{
			XdsHost:    xdsHost,
			XdsPort:    xdsPort,
			AgwXdsPort: agwXdsPort,
		},
		IstioAutoMtlsEnabled: istioAutoMtlsEnabled,
		ImageInfo: &deployer.ImageInfo{
			Registry:   globalSettings.DefaultImageRegistry,
			Tag:        globalSettings.DefaultImageTag,
			PullPolicy: globalSettings.DefaultImagePullPolicy,
		},
		DiscoveryNamespaceFilter: c.cfg.Client.ObjectFilter(),
		CommonCollections:        c.commoncol,
		GatewayClassName:         c.cfg.GatewayClassName,
		WaypointGatewayClassName: c.cfg.WaypointGatewayClassName,
		AgentgatewayClassName:    c.cfg.AgentgatewayClassName,
	}

	setupLog.Info("creating gateway class provisioner")
	if err := NewGatewayClassProvisioner(c.mgr, c.cfg.ControllerName,
		GetDefaultClassInfo(globalSettings, c.cfg.GatewayClassName, c.cfg.WaypointGatewayClassName,
			c.cfg.AgentgatewayClassName, c.cfg.ControllerName, c.cfg.AgwControllerName, c.cfg.AdditionalGatewayClasses)); err != nil {
		setupLog.Error(err, "unable to create gateway class provisioner")
		return err
	}

	setupLog.Info("creating base gateway controller")
	if err := NewBaseGatewayController(ctx, gwCfg, c.cfg.ExtraGatewayParameters); err != nil {
		setupLog.Error(err, "unable to create gateway controller")
		return err
	}

	setupLog.Info("creating inferencepool controller")
	// Create the InferencePool controller if the inference extension feature is enabled and the API group is registered.
	if globalSettings.EnableInferExt &&
		c.mgr.GetScheme().IsGroupRegistered(inf.GroupVersion.Group) {
		poolCfg := &InferencePoolConfig{
			Mgr: c.mgr,
			// TODO read this from globalSettings
			ControllerName: c.cfg.ControllerName,
		}
		// Enable the inference extension deployer if set.
		if globalSettings.InferExtAutoProvision {
			setupLog.Info("inference extension auto-provisioning is deprecated in v2.1 and will be removed in v2.2.")
			poolCfg.InferenceExt = new(deployer.InferenceExtInfo)
		}
		if !globalSettings.EnableAgentgateway {
			setupLog.Info("using inference extension without agentgateway is deprecated in v2.1 and will not be supported in v2.2.")
		}
		if err := NewBaseInferencePoolController(ctx, poolCfg, &gwCfg, c.cfg.ExtraGatewayParameters); err != nil {
			setupLog.Error(err, "unable to create inferencepool controller")
			return err
		}
	}

	// TODO (dmitri-d) don't think c.ready field is used anywhere and can be removed
	// mgr WaitForCacheSync is part of proxySyncer's HasSynced
	// so we can mark ready here before we call mgr.Start
	c.ready.Store(true)
	return nil
}

func (c *ControllerBuilder) HasSynced() bool {
	var hasSynced bool
	if c.agwSyncer != nil {
		hasSynced = c.proxySyncer.HasSynced() && c.agwSyncer.HasSynced()
	} else {
		hasSynced = c.proxySyncer.HasSynced()
	}
	return hasSynced
}

// GetDefaultClassInfo returns the default GatewayClass for the kgateway controller.
// Exported for testing.
func GetDefaultClassInfo(globalSettings *apisettings.Settings,
	gatewayClassName, waypointGatewayClassName, agwClassName, controllerName, agwControllerName string,
	additionalClassInfos map[string]*deployer.GatewayClassInfo) map[string]*deployer.GatewayClassInfo {
	classInfos := map[string]*deployer.GatewayClassInfo{
		gatewayClassName: {
			Description:    "Standard class for managing Gateway API ingress traffic.",
			Labels:         map[string]string{},
			Annotations:    map[string]string{},
			ControllerName: controllerName,
		},
	}
	// Only enable waypoint gateway class if it's enabled in the settings
	classInfos[waypointGatewayClassName] = &deployer.GatewayClassInfo{
		Description: "Specialized class for Istio ambient mesh waypoint proxies.",
		Labels:      map[string]string{},
		Annotations: map[string]string{
			"ambient.istio.io/waypoint-inbound-binding": "PROXY/15088",
		},
		ControllerName: controllerName,
	}
	// Only enable agentgateway gateway class if it's enabled in the settings
	if globalSettings.EnableAgentgateway {
		classInfos[agwClassName] = &deployer.GatewayClassInfo{
			Description:    "Specialized class for agentgateway.",
			Labels:         map[string]string{},
			Annotations:    map[string]string{},
			ControllerName: agwControllerName,
		}
	}
	for class, classInfo := range additionalClassInfos {
		classInfos[class] = classInfo
	}
	return classInfos
}
