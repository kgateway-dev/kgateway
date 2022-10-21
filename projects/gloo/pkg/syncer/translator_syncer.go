package syncer

import (
	"context"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils/metrics"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	"github.com/solo-io/go-utils/contextutils"

	"github.com/hashicorp/go-multierror"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
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
	reporter   reporter.StatusReporter
	// used for debugging purposes only
	latestSnap *v1snap.ApiSnapshot
	extensions []TranslatorSyncerExtension
	// used to track which envoy node IDs exist without belonging to a proxy
	extensionKeys map[string]struct{}
	settings      *v1.Settings
	statusMetrics metrics.ConfigStatusMetrics
}

type TranslatorSyncerExtensionParams struct {
	RateLimitServiceSettings ratelimit.ServiceSettings
}

type TranslatorSyncerExtensionFactory func(context.Context, TranslatorSyncerExtensionParams) (TranslatorSyncerExtension, error)

type UpgradeableTranslatorSyncerExtension interface {
	ExtensionName() string
	IsUpgrade() bool
}

// TODO(kdorosh) in follow up PR, update this interface so it can never error
// It is logically invalid for us to return an error here (translation of resources always needs to
// result in a xds snapshot, so we are resilient to pod restarts)
type TranslatorSyncerExtension interface {
	Sync(
		ctx context.Context,
		snap *v1snap.ApiSnapshot,
		settings *v1.Settings,
		xdsCache envoycache.SnapshotCache,
		reports reporter.ResourceReports,
	) (string, error)
}

func NewTranslatorSyncer(
	translator translator.Translator,
	xdsCache envoycache.SnapshotCache,
	xdsHasher *xds.ProxyKeyHasher,
	sanitizer sanitizer.XdsSanitizer,
	reporter reporter.StatusReporter,
	devMode bool,
	extensions []TranslatorSyncerExtension,
	settings *v1.Settings,
	statusMetrics metrics.ConfigStatusMetrics,
) v1snap.ApiSyncer {
	s := &translatorSyncer{
		translator:    translator,
		xdsCache:      xdsCache,
		xdsHasher:     xdsHasher,
		reporter:      reporter,
		extensions:    extensions,
		sanitizer:     sanitizer,
		settings:      settings,
		statusMetrics: statusMetrics,
	}
	if devMode {
		// TODO(ilackarms): move this somewhere else?
		go func() {
			_ = s.ServeXdsSnapshots()
		}()
	}
	return s
}

func (s *translatorSyncer) Sync(ctx context.Context, snap *v1snap.ApiSnapshot) error {
	logger := contextutils.LoggerFrom(ctx)
	var multiErr *multierror.Error
	reports := make(reporter.ResourceReports)
	err := s.syncEnvoy(ctx, snap, reports)
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
	}
	s.extensionKeys = map[string]struct{}{}
	for _, extension := range s.extensions {
		intermediateReports := make(reporter.ResourceReports)
		nodeID, err := extension.Sync(ctx, snap, s.settings, s.xdsCache, intermediateReports)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
		reports.Merge(intermediateReports)
		s.extensionKeys[nodeID] = struct{}{}
	}

	// while tempting to write statuses in parallel to increase performance, we should actually first consider recommending the user tunes k8s qps/burst:
	// https://github.com/solo-io/gloo/blob/a083522af0a4ce22f4d2adf3a02470f782d5a865/projects/gloo/api/v1/settings.proto#L337-L350
	//
	// add TEMPORARY wrap to our WriteReports error that we should remove in Gloo Edge ~v1.16.0+.
	// to get the status performance improvements, we need to make the assumption that the user has the latest CRDs installed.
	// if a user forgets the error message is very confusing (invalid request during kubectl patch);
	// this should help them understand what's going on in case they did not read the changelog.
	if err := s.reporter.WriteReports(ctx, reports, nil); err != nil {
		logger.Debugf("Failed writing report for proxies: %v", err)
		wrappedErr := eris.Wrapf(err, "failed to write reports"+
			"did you make sure your CRDs have been updated since v1.11.41 of open-source? (i.e. `status` and `status.statuses` fields exist on your CR)")
		multiErr = multierror.Append(multiErr, eris.Wrapf(wrappedErr, "writing reports"))
	}

	// Update resource status metrics
	for resource, report := range reports {
		status := s.reporter.StatusFromReport(report, nil)
		s.statusMetrics.SetResourceStatus(ctx, resource, status)
	}

	return multiErr.ErrorOrNil()
}
