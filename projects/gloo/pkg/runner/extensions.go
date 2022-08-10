package runner

import (
	"context"

	"github.com/go-errors/errors"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/enterprise_warning"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	extauthExt "github.com/solo-io/gloo/projects/gloo/pkg/syncer/extauth"
	ratelimitExt "github.com/solo-io/gloo/projects/gloo/pkg/syncer/ratelimit"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
)

// RunExtensions represent the properties that can be injected into a Gloo Runner
// These properties are the injection point for Enterprise functionality
type RunExtensions struct {
	PluginRegistryFactory PluginRegistryFactory
	SyncerExtensions      []syncer.TranslatorSyncerExtensionFactory
	XdsCallbacks          server.Callbacks
	ApiEmitterChannel     chan struct{}
}

// A PluginRegistryFactory generates a PluginRegistry
// It is executed each translation loop, ensuring we have up to date configuration of all plugins
type PluginRegistryFactory func(ctx context.Context, opts registry.PluginOpts) plugins.PluginRegistry

// DefaultRunExtensions returns the RunExtensions used to power Gloo Edge OSS
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

// GlooPluginRegistryFactory defines the PluginRegistryFactory that powers Gloo Edge OSS
func GlooPluginRegistryFactory(_ context.Context, opts registry.PluginOpts) plugins.PluginRegistry {
	availablePlugins := registry.Plugins(opts)

	// To improve the UX, load a plugin that warns users if they are attempting to use enterprise configuration
	availablePlugins = append(availablePlugins, enterprise_warning.NewPlugin())
	return registry.NewPluginRegistry(availablePlugins)
}

// ValidateRunExtensions returns an error if any of the provided RunExtensions are invalid, nil otherwise
func ValidateRunExtensions(extensions RunExtensions) error {
	if extensions.ApiEmitterChannel == nil {
		return errors.Errorf("RunExtensions.ApiEmitterChannel must be defined, found nil")
	}
	if extensions.PluginRegistryFactory == nil {
		return errors.Errorf("RunExtensions.PluginRegistryFactory must be defined, found nil")
	}
	if extensions.SyncerExtensions == nil {
		return errors.Errorf("RunExtensions.SyncerExtensions must be defined, found nil")
	}
	return nil
}
