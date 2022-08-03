package runner

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/enterprise_warning"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	extauthExt "github.com/solo-io/gloo/projects/gloo/pkg/syncer/extauth"
	ratelimitExt "github.com/solo-io/gloo/projects/gloo/pkg/syncer/ratelimit"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
)

// A PluginRegistryFactory generates a PluginRegistry
// It is executed each translation loop, ensuring we have up to date configuration of all plugins
type PluginRegistryFactory func(ctx context.Context, opts registry.PluginOpts) plugins.PluginRegistry

type RunExtensions struct {
	PluginRegistryFactory PluginRegistryFactory
	SyncerExtensions      []syncer.TranslatorSyncerExtensionFactory
	XdsCallbacks          server.Callbacks
	ApiEmitterChannel     chan struct{}
}

func DefaultRunExtensions() *RunExtensions {
	return &RunExtensions{
		PluginRegistryFactory: GlooPluginRegistryFactory,
		SyncerExtensions: []syncer.TranslatorSyncerExtensionFactory{
			ratelimitExt.NewTranslatorSyncerExtension,
			extauthExt.NewTranslatorSyncerExtension,
		},
		ApiEmitterChannel: make(chan struct{}),
		XdsCallbacks:      nil,
	}
}

func GlooPluginRegistryFactory(_ context.Context, opts registry.PluginOpts) plugins.PluginRegistry {
	availablePlugins := registry.Plugins(opts)

	// To improve the UX, load a plugin that warns users if they are attempting to use enterprise configuration
	availablePlugins = append(availablePlugins, enterprise_warning.NewPlugin())
	return registry.NewPluginRegistry(availablePlugins)
}
