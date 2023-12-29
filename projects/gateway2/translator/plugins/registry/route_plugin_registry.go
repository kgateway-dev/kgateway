package registry

import (
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/headermodifier"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/mirror"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/redirect"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/urlrewrite"
)

type RoutePluginRegistry struct {
	routePlugins []plugins.RoutePlugin
}

func (h *RoutePluginRegistry) GetRoutePlugins() []plugins.RoutePlugin {
	return h.routePlugins
}

func NewRoutePluginRegistry(
	queries query.GatewayQueries,
) *RoutePluginRegistry {
	return &RoutePluginRegistry{
		routePlugins: []plugins.RoutePlugin{
			headermodifier.NewPlugin(),
			mirror.NewPlugin(queries),
			redirect.NewPlugin(),
			urlrewrite.NewPlugin(),
		},
	}
}
