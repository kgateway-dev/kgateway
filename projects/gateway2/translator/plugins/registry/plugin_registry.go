package registry

import (
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/headermodifier"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/mirror"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/redirect"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/routeoptions"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/urlrewrite"
)

// PluginRegistry is used to provide Plugins to relevant translators using
// during the conversion of K8s Gateway resources into a Gloo Proxy resource.
// The registry is a concrete type because we only have one implementation for a
// registry to provide these plugins. This can still support Enterprise-only functionality
// because the registry initialized with the list of plugins that it will manage.
type PluginRegistry struct {
	initialized            bool // initialized is used to track whether a PluginRegistry instance has been created
	routePlugins           []plugins.RoutePlugin
	postTranslationPlugins []plugins.PostTranslationPlugin
}

// IsNil returns true if the PluginRegistry has not been initialized.
func (p *PluginRegistry) IsNil() bool {
	return !p.initialized
}

func (p *PluginRegistry) GetRoutePlugins() []plugins.RoutePlugin {
	return p.routePlugins
}

func (p *PluginRegistry) GetPostTranslationPlugins() []plugins.PostTranslationPlugin {
	return p.postTranslationPlugins
}

func NewPluginRegistry(allPlugins []plugins.Plugin) PluginRegistry {
	var (
		routePlugins           []plugins.RoutePlugin
		postTranslationPlugins []plugins.PostTranslationPlugin
	)

	for _, plugin := range allPlugins {
		if routePlugin, ok := plugin.(plugins.RoutePlugin); ok {
			routePlugins = append(routePlugins, routePlugin)
		}
		if postTranslationPlugin, ok := plugin.(plugins.PostTranslationPlugin); ok {
			postTranslationPlugins = append(postTranslationPlugins, postTranslationPlugin)
		}
	}
	return PluginRegistry{
		initialized:            true,
		routePlugins:           routePlugins,
		postTranslationPlugins: postTranslationPlugins,
	}
}

// BuildPlugins returns the full set of plugins to be registered.
// New plugins should be added to this list (and only this list).
// If modification of this list is needed for testing etc,
// we can add a new registry constructor that accepts this function
func BuildPlugins(queries query.GatewayQueries) []plugins.Plugin {
	return []plugins.Plugin{
		headermodifier.NewPlugin(),
		mirror.NewPlugin(queries),
		redirect.NewPlugin(),
		routeoptions.NewPlugin(queries),
		urlrewrite.NewPlugin(),
	}
}
