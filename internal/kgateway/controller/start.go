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

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/inferenceextension/endpointpicker"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/registry"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections/metrics"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/proxy_syncer"
	krtinternal "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	agentgatewayplugins "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	common "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	kgtwschemes "github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/namespaces"
)

const (
	// AutoProvision controls whether the controller will be responsible for provisioning dynamic
	// infrastructure for the Gateway API.
	AutoProvision           = true
	ControllerRuntimeLogger = "controllerruntime"
)

type SetupOpts struct {
	Cache envoycache.SnapshotCache

	KrtDebugger *krt.DebugHandler

	// static set of global Settings
	GlobalSettings *settings.Settings

	PprofBindAddress       string
	HealthProbeBindAddress string
	MetricsBindAddress     string
}

var setupLog = ctrl.Log.WithName("setup")

type StartConfig struct {
	Manager                  manager.Manager
	ControllerName           string
	GatewayClassName         string
	WaypointGatewayClassName string
	AgentGatewayClassName    string

	Dev        bool
	SetupOpts  *SetupOpts
	RestConfig *rest.Config
	// ExtensionsFactory is the factory function which will return an extensions.K8sGatewayExtensions
	// This is responsible for producing the extension points that this controller requires
	ExtraPlugins             func(ctx context.Context, commoncol *common.CommonCollections, mergeSettingsJSON string) []sdk.Plugin
	ExtraAgentgatewayPlugins func(ctx context.Context, agw *agentgatewayplugins.AgwCollections) []agentgatewayplugins.AgentgatewayPlugin
	ExtraGatewayParameters   func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters
	Client                   istiokube.Client

	AgwCollections    *agentgatewayplugins.AgwCollections
	CommonCollections *common.CommonCollections
	AugmentedPods     krt.Collection[krtcollections.LocalityPod]
	UniqueClients     krt.Collection[ir.UniqlyConnectedClient]

	KrtOptions krtinternal.KrtOptions
}

// Start runs the controllers responsible for processing the K8s Gateway API objects
// It is intended to be run in a goroutine as the function will block until the supplied
// context is cancelled
type ControllerBuilder struct {
	proxySyncer *proxy_syncer.ProxySyncer
	agwSyncer   *agentgatewaysyncer.AgentGwSyncer
	cfg         StartConfig
	mgr         ctrl.Manager
	commoncol   *common.CommonCollections

	ready atomic.Bool
}

func NewControllerBuilder(ctx context.Context, cfg StartConfig) (*ControllerBuilder, error) {
	loggingOptions := istiolog.DefaultOptions()
	loggingOptions.JSONEncoding = true
	if cfg.Dev {
		setupLog.Info("starting log in dev mode")
		loggingOptions.SetDefaultOutputLevel(istiolog.OverrideScopeName, istiolog.DebugLevel)
		logging.MustSetLevel(ControllerRuntimeLogger, slog.LevelDebug)
	}
	istiolog.Configure(loggingOptions)

	setupLog.Info("initializing kgateway extensions")
	// Extend the scheme and add the EPP plugin if the inference extension is enabled and the InferencePool CRD exists.
	if cfg.SetupOpts.GlobalSettings.EnableInferExt {
		exists, err := kgtwschemes.AddInferExtV1Scheme(cfg.RestConfig, cfg.Manager.GetScheme())
		switch {
		case err != nil:
			return nil, err
		case exists:
			setupLog.Info("adding endpoint-picker inference extension")

			existingExtraPlugins := cfg.ExtraPlugins
			cfg.ExtraPlugins = func(ctx context.Context, commoncol *common.CommonCollections, mergeSettingsJSON string) []sdk.Plugin {
				var plugins []sdk.Plugin

				// Add the inference extension plugin.
				if plug := endpointpicker.NewPlugin(ctx, commoncol); plug != nil {
					plugins = append(plugins, *plug)
				}

				// If there was an existing ExtraPlugins function, append its plugins too.
				if existingExtraPlugins != nil {
					plugins = append(plugins, existingExtraPlugins(ctx, commoncol, cfg.SetupOpts.GlobalSettings.PolicyMerge)...)
				}

				return plugins
			}
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
		cfg.AgentGatewayClassName,
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
		cfg.AgentGatewayClassName,
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

	var agentGatewaySyncer *agentgatewaysyncer.AgentGwSyncer
	if cfg.SetupOpts.GlobalSettings.EnableAgentGateway {
		agentgatewayMergedPlugins := agentGatewayPluginFactory(cfg)(ctx, cfg.AgwCollections)
		cfg.AgwCollections.InitPlugins(ctx, mergedPlugins, globalSettings)

		agentGatewaySyncer = agentgatewaysyncer.NewAgentGwSyncer(
			cfg.ControllerName,
			cfg.AgentGatewayClassName,
			cfg.Client,
			cfg.Manager,
			cfg.AgwCollections,
			// TODO(npolshak): move away from shared mergedPlugins to agentGatewayPlugins https://github.com/kgateway-dev/kgateway/issues/12052
			mergedPlugins,
			agentgatewayMergedPlugins,
			cfg.SetupOpts.Cache,
			namespaces.GetPodNamespace(),
			cfg.Client.ClusterID().String(),
			cfg.SetupOpts.GlobalSettings.EnableInferExt,
		)
		agentGatewaySyncer.Init(cfg.KrtOptions)

		if err := cfg.Manager.Add(agentGatewaySyncer); err != nil {
			setupLog.Error(err, "unable to add agentGatewaySyncer runnable")
			return nil, err
		}

		agentGatewayStatusSyncer := agentgatewaysyncer.NewAgentGwStatusSyncer(
			cfg.ControllerName,
			cfg.AgentGatewayClassName,
			cfg.Client,
			cfg.Manager,
			agentGatewaySyncer.GatewayReportQueue(),
			agentGatewaySyncer.ListenerSetReportQueue(),
			agentGatewaySyncer.RouteReportQueue(),
			agentGatewaySyncer.CacheSyncs(),
		)
		if err := cfg.Manager.Add(agentGatewayStatusSyncer); err != nil {
			setupLog.Error(err, "unable to add agentGatewayStatusSyncer runnable")
			return nil, err
		}
	}

	setupLog.Info("starting controller builder")
	cb := &ControllerBuilder{
		proxySyncer: proxySyncer,
		agwSyncer:   agentGatewaySyncer,
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
	return func(ctx context.Context, commoncol *common.CommonCollections) sdk.Plugin {
		plugins := registry.Plugins(ctx, commoncol, cfg.WaypointGatewayClassName, *cfg.SetupOpts.GlobalSettings)
		plugins = append(plugins, krtcollections.NewBuiltinPlugin(ctx))
		if cfg.ExtraPlugins != nil {
			plugins = append(plugins, cfg.ExtraPlugins(ctx, commoncol, cfg.SetupOpts.GlobalSettings.PolicyMerge)...)
		}
		return registry.MergePlugins(plugins...)
	}
}

func agentGatewayPluginFactory(cfg StartConfig) func(ctx context.Context, agw *agentgatewayplugins.AgwCollections) agentgatewayplugins.AgentgatewayPlugin {
	return func(ctx context.Context, agw *agentgatewayplugins.AgwCollections) agentgatewayplugins.AgentgatewayPlugin {
		plugins := agentgatewayplugins.Plugins(agw)
		if cfg.ExtraAgentgatewayPlugins != nil {
			plugins = append(plugins, cfg.ExtraAgentgatewayPlugins(ctx, agw)...)
		}
		return agentgatewayplugins.MergePlugins(plugins...)
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

	istioAutoMtlsEnabled := globalSettings.EnableIstioAutoMtls

	gwCfg := GatewayConfig{
		Mgr:            c.mgr,
		ControllerName: c.cfg.ControllerName,
		AutoProvision:  AutoProvision,
		ControlPlane: deployer.ControlPlaneInfo{
			XdsHost: xdsHost,
			XdsPort: xdsPort,
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
		AgentGatewayClassName:    c.cfg.AgentGatewayClassName,
	}

	setupLog.Info("creating gateway class provisioner")
	if err := NewGatewayClassProvisioner(c.mgr, c.cfg.ControllerName, GetDefaultClassInfo(globalSettings, c.cfg.GatewayClassName, c.cfg.WaypointGatewayClassName, c.cfg.AgentGatewayClassName)); err != nil {
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
			poolCfg.InferenceExt = new(deployer.InferenceExtInfo)
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
func GetDefaultClassInfo(globalSettings *settings.Settings, gatewayClassName string, waypointGatewayClassName string, agentGatewayClassName string) map[string]*ClassInfo {
	classInfos := map[string]*ClassInfo{
		gatewayClassName: {
			Description: "Standard class for managing Gateway API ingress traffic.",
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		waypointGatewayClassName: {
			Description: "Specialized class for Istio ambient mesh waypoint proxies.",
			Labels:      map[string]string{},
			Annotations: map[string]string{
				"ambient.istio.io/waypoint-inbound-binding": "PROXY/15088",
			},
		},
	}
	// Only enable agentgateway gateway class if it's enabled in the settings
	if globalSettings.EnableAgentGateway {
		classInfos[agentGatewayClassName] = &ClassInfo{
			Description: "Specialized class for agentgateway.",
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		}
	}
	return classInfos
}
