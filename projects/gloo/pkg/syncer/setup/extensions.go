package setup

import (
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/servers/iosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	xdsserver "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
)

var ErrNilExtension = func(name string) error {
	return errors.Errorf("Extensions.%s must be defined, found nil", name)
}

// Extensions contains the set of extension points for Gloo.
// These are the injectable pieces of code, which we use to define separate
// implementations of our Open Source and Enterprise Control Plane implementations.
// See RunGlooWithExtensions for where this is used.
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

	// SnapshotHistoryFactory is the factory function which will produce a History object
	// This history object is used by the ControlPlane to track internal state
	SnapshotHistoryFactory iosnapshot.HistoryFactory
}

// Validate returns an error if the Extensions are invalid, nil otherwise
func (e Extensions) Validate() error {

	if e.SnapshotHistoryFactory == nil {
		return ErrNilExtension("SnapshotHistoryFactory")
	}
	if e.PluginRegistryFactory == nil {
		return ErrNilExtension("PluginRegistryFactory")
	}
	if e.ApiEmitterChannel == nil {
		return ErrNilExtension("ApiEmitterChannel")
	}
	if e.SyncerExtensions == nil {
		return ErrNilExtension("SyncerExtensions")
	}

	return nil
}
