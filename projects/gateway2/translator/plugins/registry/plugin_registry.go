package registry

import (
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/directresponse"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/headermodifier"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/httplisteneroptions"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/listeneroptions"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/mirror"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/redirect"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/routeoptions"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/urlrewrite"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/virtualhostoptions"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PluginRegistry is used to provide Plugins to the K8s Gateway translator.
// These plugins either operate during the conversion of K8s Gateway resources
// into a Gloo Proxy resource, or during the post-processing of that conversion.
type PluginRegistry struct {
	routePlugins           []plugins.RoutePlugin
	listenerPlugins        []plugins.ListenerPlugin
	postTranslationPlugins []plugins.PostTranslationPlugin
	statusPlugins          []plugins.StatusPlugin
}

func (p *PluginRegistry) GetRoutePlugins() []plugins.RoutePlugin {
	return p.routePlugins
}

func (p *PluginRegistry) GetListenerPlugins() []plugins.ListenerPlugin {
	return p.listenerPlugins
}

func (p *PluginRegistry) GetPostTranslationPlugins() []plugins.PostTranslationPlugin {
	return p.postTranslationPlugins
}

func (p *PluginRegistry) GetStatusPlugins() []plugins.StatusPlugin {
	return p.statusPlugins
}

func NewPluginRegistry(allPlugins []plugins.Plugin) PluginRegistry {
	var (
		routePlugins           []plugins.RoutePlugin
		listenerPlugins        []plugins.ListenerPlugin
		postTranslationPlugins []plugins.PostTranslationPlugin
		statusPlugins          []plugins.StatusPlugin
	)

	for _, plugin := range allPlugins {
		if routePlugin, ok := plugin.(plugins.RoutePlugin); ok {
			routePlugins = append(routePlugins, routePlugin)
		}
		if listenerPlugin, ok := plugin.(plugins.ListenerPlugin); ok {
			listenerPlugins = append(listenerPlugins, listenerPlugin)
		}
		if postTranslationPlugin, ok := plugin.(plugins.PostTranslationPlugin); ok {
			postTranslationPlugins = append(postTranslationPlugins, postTranslationPlugin)
		}
		if statusPlugin, ok := plugin.(plugins.StatusPlugin); ok {
			statusPlugins = append(statusPlugins, statusPlugin)
		}
	}
	return PluginRegistry{
		routePlugins,
		listenerPlugins,
		postTranslationPlugins,
		statusPlugins,
	}
}

// BuildPlugins returns the full set of plugins to be registered.
// New plugins should be added to this list (and only this list).
// If modification of this list is needed for testing etc,
// we can add a new registry constructor that accepts this function
func BuildPlugins(
	queries gwquery.GatewayQueries,
	client client.Client,
	routeOptionClient gatewayv1.RouteOptionClient,
	vhostOptionClient gatewayv1.VirtualHostOptionClient,
	statusReporter reporter.StatusReporter,
) []plugins.Plugin {
	return []plugins.Plugin{
		headermodifier.NewPlugin(),
		mirror.NewPlugin(queries),
		redirect.NewPlugin(),
		routeoptions.NewPlugin(queries, client, routeOptionClient, statusReporter),
		virtualhostoptions.NewPlugin(queries, client, vhostOptionClient, statusReporter),
		httplisteneroptions.NewPlugin(queries, client),
		listeneroptions.NewPlugin(queries, client),
		urlrewrite.NewPlugin(),
		directresponse.NewPlugin(queries), // direct response needs to run after any plugin that might set an action
	}
}
