package setup

import (
	"context"

	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	core "github.com/kgateway-dev/kgateway/v2/internal/kgateway/setup"
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
	ExtraGatewayParameters   func(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters
	AddToScheme              func(s *runtime.Scheme) error
	ExtraXDSCallbacks        xdsserver.Callbacks
}

func New(opts Options) core.Server {
	// internal setup already accepted functional-options; we wrap only extras.
	return core.New(
		core.WithExtraPlugins(opts.ExtraPlugins),
		core.ExtraGatewayParameters(opts.ExtraGatewayParameters),
		core.WithGatewayControllerName(opts.GatewayControllerName),
		core.WithGatewayClassName(opts.GatewayClassName),
		core.WithWaypointClassName(opts.WaypointGatewayClassName),
		core.WithAgentGatewayClassName(opts.AgentGatewayClassName),
		core.AddToScheme(opts.AddToScheme),
		core.WithExtraXDSCallbacks(opts.ExtraXDSCallbacks),
	)
}
