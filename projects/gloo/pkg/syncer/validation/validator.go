package validation

import (
	"context"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

// NewValidator will create a new validator for the purpose of validating extensions.
func NewValidator(extensions []syncer.TranslatorSyncerExtension, settings *v1.Settings) *validator {
	return &validator{
		extensions:     extensions,
		settings:       settings,
		snapshotSetter: &syncer.NoOpSnapshotSetter{},
	}
}

type Validator interface {
	// Validate will validate resources from a snapshot, and return the resource reports corresponding to the
	// resources in the snapshot.
	Validate(ctx context.Context, snapshot *gloosnapshot.ApiSnapshot) reporter.ResourceReports
}

// validator will validate the extension resources of a snapshot.
type validator struct {
	extensions     []syncer.TranslatorSyncerExtension
	settings       *v1.Settings
	snapshotSetter syncer.SnapshotSetter
}

// Validate will sync the extensions with the snapshot. This uses a no-op snapshot setter, so that no changes to the
// snapshot occur.
func (v *validator) Validate(ctx context.Context, snapshot *gloosnapshot.ApiSnapshot) reporter.ResourceReports {
	reports := reporter.ResourceReports{}
	for _, ex := range v.extensions {
		intermediateReports := make(reporter.ResourceReports)
		ex.Sync(ctx, snapshot, v.settings, v.snapshotSetter, intermediateReports)
		reports.Merge(intermediateReports)
	}
	return reports
}
