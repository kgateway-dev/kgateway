package plugins

import (
	"github.com/solo-io/gloo/pkg/log"
)

var defaultRegistry = &registry{}

func Register(plugin TranslatorPlugin) {
	if plugin == nil {
		log.Fatalf("plugin can not be nil")
	}
	defaultRegistry.plugins = append(defaultRegistry.plugins, plugin)
}

func RegisteredPlugins() []TranslatorPlugin {
	return defaultRegistry.plugins
}

type registry struct {
	plugins             []TranslatorPlugin
}
