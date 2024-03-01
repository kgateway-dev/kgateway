package setup

import (
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	xdsserver "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
)

// Extensions contains the set of extension points for Gloo
type Extensions struct {
	// PluginRegistryFactory is responsible for creating an xDS PluginRegistry
	// This is the set of plugins which are executed when converting a Proxy into an xDS Snapshot
	PluginRegistryFactory plugins.PluginRegistryFactory

	// SyncerExtensions perform additional syncing logic on a given ApiSnapshot
	// These are used to inject the syncers that process Enterprise-only APIs (AuthConfig, RateLimitConfig)
	SyncerExtensions []syncer.TranslatorSyncerExtensionFactory

	// XdsCallbacks are asynchronous callbacks to perform during xds communication
	XdsCallbacks xdsserver.Callbacks

	// ApiEmitterChannel is a channel that forces the API Emitter to emit a new API Snapshot
	ApiEmitterChannel chan struct{}

	K8sGatewayExtensions extensions.ExtensionManagerFactory
}

// Validate returns an error if the Extensions are invalid, nil otherwise
func (e Extensions) Validate() error {
	if e.K8sGatewayExtensions == nil {
		return errors.Errorf("Extensions.K8sGatewayExtension must be defined, found nil")
	}

	if e.PluginRegistryFactory == nil {
		return errors.Errorf("Extensions.PluginRegistryFactory must be defined, found nil")
	}
	if e.ApiEmitterChannel == nil {
		return errors.Errorf("Extensions.ApiEmitterChannel must be defined, found nil")
	}
	if e.SyncerExtensions == nil {
		return errors.Errorf("Extensions.SyncerExtensions must be defined, found nil")
	}

	return nil
}
