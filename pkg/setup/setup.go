package setup

import (
	"context"

	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"istio.io/istio/pkg/kube/kubetypes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	ctrl "sigs.k8s.io/controller-runtime"

	core "github.com/kgateway-dev/kgateway/v2/internal/kgateway/setup"
	agentgatewayplugins "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	common "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

type Options struct {
	GatewayControllerName    string
	GatewayClassName         string
	WaypointGatewayClassName string
	AgentGatewayClassName    string
	ExtraPlugins             func(ctx context.Context, commoncol *common.CommonCollections) []sdk.Plugin
	ExtraAgentgatewayPlugins func(ctx context.Context, agw *agentgatewayplugins.AgwCollections) []agentgatewayplugins.PolicyPlugin
	ExtraGatewayParameters   func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters
	ExtraXDSCallbacks        xdsserver.Callbacks
	RestConfig               *rest.Config
	CtrlMgrOptions           func(context.Context) *ctrl.Options
	// extra controller manager config, like registering additional controllers
	ExtraManagerConfig []func(ctx context.Context, mgr manager.Manager, objectFilter kubetypes.DynamicObjectFilter) error
}

func New(opts Options) (core.Server, error) {
	// internal setup already accepted functional-options; we wrap only extras.
	return core.New(
		core.WithExtraPlugins(opts.ExtraPlugins),
		core.WithExtraAgentgatewayPlugins(opts.ExtraAgentgatewayPlugins),
		core.ExtraGatewayParameters(opts.ExtraGatewayParameters),
		core.WithGatewayControllerName(opts.GatewayControllerName),
		core.WithGatewayClassName(opts.GatewayClassName),
		core.WithWaypointClassName(opts.WaypointGatewayClassName),
		core.WithAgentGatewayClassName(opts.AgentGatewayClassName),
		core.WithExtraXDSCallbacks(opts.ExtraXDSCallbacks),
		core.WithRestConfig(opts.RestConfig),
		core.WithControllerManagerOptions(opts.CtrlMgrOptions),
		core.WithExtraManagerConfig(opts.ExtraManagerConfig...),
	)
}
