package controller

import (
	"context"

	"istio.io/istio/pkg/kube/kubetypes"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	internaldeployer "github.com/kgateway-dev/kgateway/v2/internal/kgateway/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

// TODO [danehans]: Refactor so controller config is organized into shared and Gateway/InferencePool-specific controllers.
type GatewayConfig struct {
	Client apiclient.Client
	Mgr    manager.Manager
	// Dev enables development mode for the controller.
	Dev bool
	// ControllerName is the name of the Envoy controller. Any GatewayClass objects
	// managed by this controller must have this name as their ControllerName.
	ControllerName string
	// AgwControllerName is the name of the agentgateway controller. Any GatewayClass objects
	// managed by this controller must have this name as their ControllerName.
	AgwControllerName string
	// ControlPlane sets the default control plane information the deployer will use.
	ControlPlane deployer.ControlPlaneInfo
	// IstioAutoMtlsEnabled enables istio auto mtls mode for the controller,
	// resulting in the deployer to enable istio and sds sidecars on the deployed proxies.
	IstioAutoMtlsEnabled bool
	// ImageInfo sets the default image information the deployer will use.
	ImageInfo *deployer.ImageInfo
	// DiscoveryNamespaceFilter filters namespaced objects based on the discovery namespace filter.
	DiscoveryNamespaceFilter kubetypes.DynamicObjectFilter
	// CommonCollections used to fetch ir.Gateways for the deployer to generate the ports for the proxy service
	CommonCollections *collections.CommonCollections
	// GatewayClassName is the configured gateway class name.
	GatewayClassName string
	// WaypointGatewayClassName is the configured waypoint gateway class name.
	WaypointGatewayClassName string
	// AgentgatewayClassName is the configured agent gateway class name.
	AgentgatewayClassName string
	// Additional GatewayClass definitions to support extending to other well-known gateway classes
	AdditionalGatewayClasses map[string]*deployer.GatewayClassInfo
	// CertWatcher is the shared certificate watcher for xDS TLS
	CertWatcher *certwatcher.CertWatcher
}

type HelmValuesGeneratorOverrideFunc func(inputs *deployer.Inputs) deployer.HelmValuesGenerator

func NewBaseGatewayController(
	ctx context.Context,
	cfg GatewayConfig,
	classInfos map[string]*deployer.GatewayClassInfo,
	helmValuesGeneratorOverride HelmValuesGeneratorOverrideFunc,
	gatewayControllerExtension pluginsdk.GatewayControllerExtension,
) error {
	logger.Info("starting controllers")

	// Initialize Gateway reconciler
	if err := watchGw(cfg, helmValuesGeneratorOverride, gatewayControllerExtension); err != nil {
		return nil
	}

	// Initialize GatewayClass reconciler
	if err := cfg.Mgr.Add(newGatewayClassReconciler(cfg, classInfos)); err != nil {
		return err
	}

	return nil
}

func watchGw(
	cfg GatewayConfig,
	helmValuesGeneratorOverride HelmValuesGeneratorOverrideFunc,
	gatewayControllerExtension pluginsdk.GatewayControllerExtension,
) error {
	logger.Info("creating gateway deployer",
		"ctrlname", cfg.ControllerName, "agwctrlname", cfg.AgwControllerName,
		"server", cfg.ControlPlane.XdsHost, "port", cfg.ControlPlane.XdsPort,
		"agwport", cfg.ControlPlane.AgwXdsPort, "tls", cfg.ControlPlane.XdsTLS,
	)

	inputs := &deployer.Inputs{
		Dev:                        cfg.Dev,
		IstioAutoMtlsEnabled:       cfg.IstioAutoMtlsEnabled,
		ControlPlane:               cfg.ControlPlane,
		ImageInfo:                  cfg.ImageInfo,
		CommonCollections:          cfg.CommonCollections,
		GatewayClassName:           cfg.GatewayClassName,
		WaypointGatewayClassName:   cfg.WaypointGatewayClassName,
		AgentgatewayClassName:      cfg.AgentgatewayClassName,
		AgentgatewayControllerName: cfg.AgwControllerName,
	}

	gwParams := internaldeployer.NewGatewayParameters(cfg.Client, inputs)
	if helmValuesGeneratorOverride != nil {
		gwParams.WithHelmValuesGeneratorOverride(helmValuesGeneratorOverride(inputs))
	}

	d, err := internaldeployer.NewGatewayDeployer(
		cfg.ControllerName,
		cfg.AgwControllerName,
		cfg.AgentgatewayClassName,
		cfg.Mgr.GetScheme(),
		cfg.Client,
		gwParams,
	)
	if err != nil {
		return err
	}

	return cfg.Mgr.Add(NewGatewayReconciler(cfg, d, gwParams, gatewayControllerExtension))
}
