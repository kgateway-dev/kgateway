package syncer

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

// SnapshotSetter sets a response snapshot for a node.
// It exposes only the Set functionality for a SnapshotCache
type SnapshotSetter interface {
	SetSnapshot(node string, snapshot envoycache.Snapshot)
}

// NoOpSnapshotSetter does nothing in it's interface
type NoOpSnapshotSetter struct{}

// SetSnapshot does nothing, it is a no-op function
func (n *NoOpSnapshotSetter) SetSnapshot(node string, snapshot envoycache.Snapshot) {}

// TranslatorSyncerExtension represents a custom sync behavior that updates an entry in the SnapshotCache
type TranslatorSyncerExtension interface {
	// ID returns the unique identifier for this TranslatorSyncerExtension
	// This represents the Key in the SnapshotCache where Sync() will store results
	ID() string

	// Sync processes an ApiSnapshot and updates reports with Errors/Warnings that it encounters
	// and updates the SnapshotCache entry if possible
	Sync(
		ctx context.Context,
		snap *v1snap.ApiSnapshot,
		settings *v1.Settings,
		snapshotSetter SnapshotSetter,
		reports reporter.ResourceReports)
}

type TranslatorSyncerExtensionParams struct {
	Hasher                   func(resources []envoycache.Resource) (uint64, error)
	RateLimitServiceSettings *ratelimit.ServiceSettings
}

// TranslatorSyncerExtensionFactory generates TranslatorSyncerExtensions
type TranslatorSyncerExtensionFactory func(context.Context, TranslatorSyncerExtensionParams) TranslatorSyncerExtension
