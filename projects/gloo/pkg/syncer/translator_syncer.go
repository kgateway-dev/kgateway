package syncer

import (
	"context"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	"github.com/solo-io/go-utils/contextutils"

	"github.com/hashicorp/go-multierror"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

type translatorSyncer struct {
	translator translator.Translator
	sanitizer  sanitizer.XdsSanitizer
	xdsCache   envoycache.SnapshotCache
	xdsHasher  *xds.ProxyKeyHasher
	reporter   reporter.Reporter
	// used for debugging purposes only
	latestSnap *v1.ApiSnapshot
	extensions []TranslatorSyncerExtension
	// used to track which envoy node IDs exist without belonging to a proxy
	extensionKeys map[string]struct{}
	settings      *v1.Settings
}

type TranslatorSyncerExtensionParams struct {
	Reports                  reporter.ResourceReports
	RateLimitServiceSettings ratelimit.ServiceSettings
}

type TranslatorSyncerExtensionFactory func(context.Context, TranslatorSyncerExtensionParams) (TranslatorSyncerExtension, error)

type UpgradeableTranslatorSyncerExtension interface {
	ExtensionName() string
	IsUpgrade() bool
}

type TranslatorSyncerExtension interface {
	Sync(ctx context.Context, snap *v1.ApiSnapshot, xdsCache envoycache.SnapshotCache) (string, error)
}

func NewTranslatorSyncer(translator translator.Translator, xdsCache envoycache.SnapshotCache, xdsHasher *xds.ProxyKeyHasher, sanitizer sanitizer.XdsSanitizer, reporter reporter.Reporter, devMode bool, extensions []TranslatorSyncerExtension, settings *v1.Settings) v1.ApiSyncer {
	s := &translatorSyncer{
		translator: translator,
		xdsCache:   xdsCache,
		xdsHasher:  xdsHasher,
		reporter:   reporter,
		extensions: extensions,
		sanitizer:  sanitizer,
		settings:   settings,
	}
	if devMode {
		// TODO(ilackarms): move this somewhere else?
		go func() {
			_ = s.ServeXdsSnapshots()
		}()
	}
	return s
}

func (s *translatorSyncer) Sync(ctx context.Context, snap *v1.ApiSnapshot) error {
	logger := contextutils.LoggerFrom(ctx)
	var multiErr *multierror.Error
	reports, err := s.syncEnvoy(ctx, snap)
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
	}
	s.extensionKeys = map[string]struct{}{}
	for _, extension := range s.extensions {
		nodeID, err := extension.Sync(ctx, snap, s.xdsCache)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
		s.extensionKeys[nodeID] = struct{}{}
	}

	if err := s.reporter.WriteReports(ctx, reports, nil); err != nil {
		logger.Debugf("Failed writing report for proxies: %v", err)
		multiErr = multierror.Append(multiErr, eris.Wrapf(err, "writing reports"))
	}

	return multiErr.ErrorOrNil()
}
