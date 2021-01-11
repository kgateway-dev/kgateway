package extauth

import (
	"context"
	"github.com/rotisserie/eris"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var (
	extauthConnectedStateDescription = "zero indicates gloo detected an error with the auth config and did not update its XDS snapshot, check the gloo logs for errors"
	extauthConnectedState            = stats.Int64("glooe.extauth/connected_state", extauthConnectedStateDescription, "1")

	extauthConnectedStateView = &view.View{
		Name:        "glooe.extauth/connected_state",
		Measure:     extauthConnectedState,
		Description: extauthConnectedStateDescription,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{},
	}
)

const (
	Name = "extauth"
)

func init() {
	_ = view.Register(extauthConnectedStateView)
}

type TranslatorSyncerExtension struct {
	reports reporter.ResourceReports
}

func NewTranslatorSyncerExtension(_ context.Context, params syncer.TranslatorSyncerExtensionParams) (syncer.TranslatorSyncerExtension, error) {
	return &TranslatorSyncerExtension{reports: params.Reports}, nil
}

func (s *TranslatorSyncerExtension) Sync(ctx context.Context, snap *gloov1.ApiSnapshot, xdsCache envoycache.SnapshotCache) (string, error) {
	ctx = contextutils.WithLogger(ctx, "extAuthTranslatorSyncer")
	logger := contextutils.LoggerFrom(ctx)
	logger.Error("Could not load extauth plugin - this is an Enterprise feature")
	return "", eris.New("Could not load extauth plugin - this is an Enterprise feature")
}

func ExtensionName() string {
	return Name
}

func IsUpgrade() bool {
	return false
}


