package setup

import (
	"context"

	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"istio.io/istio/pkg/kube/kubetypes"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	agwplugins "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/agentgatewaysyncer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/setup"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

type Options struct {
	APIClient                      apiclient.Client
	ExtraInformerCacheSyncHandlers []cache.InformerSynced
	GatewayControllerExtension     sdk.GatewayControllerExtension

	GatewayControllerName      string
	AgentgatewayControllerName string
	GatewayClassName           string
	WaypointGatewayClassName   string
	AgentgatewayClassName      string
	AdditionalGatewayClasses   map[string]*deployer.GatewayClassInfo
	ExtraPlugins               func(ctx context.Context, commoncol *collections.CommonCollections, mergeSettingsJSON string) []sdk.Plugin
	ExtraAgwPlugins            func(ctx context.Context, agw *agwplugins.AgwCollections) []agwplugins.AgwPlugin
	// HelmValuesGeneratorOverride allows replacing the default helm values generation logic.
	// When set, this generator will be used instead of the built-in GatewayParameters-based generator
	// for all Gateways. This is a 1:1 replacement - you provide one generator that handles everything.
	HelmValuesGeneratorOverride func(inputs *deployer.Inputs) deployer.HelmValuesGenerator
	ExtraXDSCallbacks           xdsserver.Callbacks
	RestConfig                  *rest.Config
	CtrlMgrOptions              func(context.Context) *ctrl.Options
	// extra controller manager config, like registering additional controllers
	ExtraManagerConfig []func(context.Context, manager.Manager, kubetypes.DynamicObjectFilter) error
	// ExtraRunnables are additional runnables to add to the manager
	ExtraRunnables []func(ctx context.Context, commoncol *collections.CommonCollections, agw *agwplugins.AgwCollections, s *apisettings.Settings) (bool, manager.Runnable)
	// Validator is the validator to use for the controller.
	Validator validator.Validator
	// ExtraAgwResourceStatusHandlers maps resource kinds to their status sync handlers for AgentGateway
	ExtraAgwResourceStatusHandlers map[schema.GroupVersionKind]agwplugins.AgwResourceStatusSyncHandler

	CommonCollectionsOptions  []collections.Option
	StatusSyncerOptions       []proxy_syncer.StatusSyncerOption
	AgentGatewaySyncerOptions []agentgatewaysyncer.AgentgatewaySyncerOption
}

func New(opts Options) (setup.Server, error) {
	// internal setup already accepted functional-options; we wrap only extras.
	return setup.New(
		setup.WithAPIClient(opts.APIClient),
		setup.WithExtraInformerCacheSyncHandlers(opts.ExtraInformerCacheSyncHandlers),
		setup.WithGatewayControllerExtension(opts.GatewayControllerExtension),
		setup.WithExtraPlugins(opts.ExtraPlugins),
		setup.WithExtraAgwPlugins(opts.ExtraAgwPlugins),
		setup.WithHelmValuesGeneratorOverride(opts.HelmValuesGeneratorOverride),
		setup.WithGatewayControllerName(opts.GatewayControllerName),
		setup.WithAgwControllerName(opts.AgentgatewayControllerName),
		setup.WithGatewayClassName(opts.GatewayClassName),
		setup.WithWaypointClassName(opts.WaypointGatewayClassName),
		setup.WithAgentgatewayClassName(opts.AgentgatewayClassName),
		setup.WithAdditionalGatewayClasses(opts.AdditionalGatewayClasses),
		setup.WithExtraXDSCallbacks(opts.ExtraXDSCallbacks),
		setup.WithRestConfig(opts.RestConfig),
		setup.WithControllerManagerOptions(opts.CtrlMgrOptions),
		setup.WithExtraManagerConfig(opts.ExtraManagerConfig...),
		setup.WithExtraRunnables(opts.ExtraRunnables...),
		setup.WithValidator(opts.Validator),
		setup.WithExtraAgwResourceStatusHandlers(opts.ExtraAgwResourceStatusHandlers),
		setup.WithCommonCollectionsOptions(opts.CommonCollectionsOptions),
		setup.WithStatusSyncerOptions(opts.StatusSyncerOptions),
		setup.WithAgentgatewaySyncerOptions(opts.AgentGatewaySyncerOptions),
	)
}
