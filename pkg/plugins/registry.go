package plugins

import (
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/log"
)

var defaultRegistry = &registry{}

type EndpointDiscoveryInitFunc func(options bootstrap.Options) (endpointdiscovery.Interface, error)

func Register(plugin TranslatorPlugin, startEndpointDiscovery EndpointDiscoveryInitFunc) {
	if plugin == nil {
		log.Fatalf("plugin can not be nil")
	}
	defaultRegistry.plugins = append(defaultRegistry.plugins, plugin)
	if startEndpointDiscovery != nil {
		defaultRegistry.endpointDiscoveries = append(defaultRegistry.endpointDiscoveries, startEndpointDiscovery)
	}
}

func RegisteredPlugins() []TranslatorPlugin {
	return defaultRegistry.plugins
}

func EndpointDiscoveryInitializers() []EndpointDiscoveryInitFunc {
	return defaultRegistry.endpointDiscoveries
}

type registry struct {
	plugins             []TranslatorPlugin
	endpointDiscoveries []EndpointDiscoveryInitFunc
}
