package cachingservice

import (
	"context"
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	"github.com/rotisserie/eris"

	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
)

// Compile-time assertion that this extension fulfills the intended interface.
var (
	_ syncer.TranslatorSyncerExtension            = new(TranslatorSyncerExtension)
	_ syncer.UpgradeableTranslatorSyncerExtension = new(TranslatorSyncerExtension)
)

const (
	Name       = "cachingservice"
	ServerRole = "caching-service"
)

// TranslatorSyncerExtension serves to satisfy the syncer interface.
// Holds nothing of importance is merely used for its functions.
type TranslatorSyncerExtension struct{}

// ExtensionName returns the string descriptor for this extension.
func (s *TranslatorSyncerExtension) ExtensionName() string {
	return Name
}

// IsUpgrade denotes whether this plugin overrides another extension.
func (s *TranslatorSyncerExtension) IsUpgrade() bool {
	return false
}

// NewTranslatorSyncerExtension never errors and does not do anything with that which is passed in.
func NewTranslatorSyncerExtension(_ context.Context, _ syncer.TranslatorSyncerExtensionParams) 
									(syncer.TranslatorSyncerExtension, error) {
	return &TranslatorSyncerExtension{}, nil
}

// Sync 
func (s *TranslatorSyncerExtension) Sync(
	ctx context.Context,
	snap *gloov1snap.ApiSnapshot,
	settings *gloov1.Settings,
	xdsCache envoycache.SnapshotCache,
	reports reporter.ResourceReports,
) (string, error) {
	ctx = contextutils.WithLogger(ctx, "cachingServiceTranslatorSyncer")
	logger := contextutils.LoggerFrom(ctx)


	// TODO: Decide on the exact configuration options to translate
	// grpc caching service has a set of toggles but full configuration 
	// is not fully stable.
	return ServerRole, nil

}

func createErrorMsg(feature string) string {
	return fmt.Sprintf("The Gloo Advanced Rate limit API feature '%s' is enterprise-only, please upgrade or use the Envoy rate-limit API instead", feature)
}
