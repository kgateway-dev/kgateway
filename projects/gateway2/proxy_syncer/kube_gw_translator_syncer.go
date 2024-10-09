package proxy_syncer

import (
	"context"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/hashutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	syncerstats "github.com/solo-io/gloo/projects/gloo/pkg/syncer/stats"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.uber.org/zap/zapcore"
)

var (
	envoySnapshotOut   = stats.Int64("api.gloo.solo.io/translator/resources", "The number of resources in the snapshot in", "1")
	resourceNameKey, _ = tag.NewKey("resource")

	envoySnapshotOutView = &view.View{
		Name:        "api.gloo.solo.io/translator/resources",
		Measure:     envoySnapshotOut,
		Description: "The number of resources in the snapshot for envoy",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{syncerstats.ProxyNameKey, resourceNameKey},
	}
)

func init() {
	_ = view.Register(envoySnapshotOutView)
}

func measureResource(ctx context.Context, resource string, length int) {
	if ctxWithTags, err := tag.New(ctx, tag.Insert(resourceNameKey, resource)); err == nil {
		stats.Record(ctxWithTags, envoySnapshotOut.M(int64(length)))
	}
}

// buildXdsSnapshot will translate from a gloov1.Proxy to xdsSnapshot using the supplied api snapshot.
// This method also merges in reports from extension syncing. Note that extensions are NOT actually synced,
// as use a NoOp snapshot when running the extension syncers; we only care about getting extensions errors and warnings
// related to the Proxy being processed.
// The actual syncing of the extension server's AND the status of the extension resources is handled by the legacy syncer.
func (s *ProxyTranslator) buildXdsSnapshot(
	ctx context.Context,
	proxy *v1.Proxy,
	snap *v1snap.ApiSnapshot,
) (cache.Snapshot, reporter.ResourceReports, *validation.ProxyReport) {
	ctx = contextutils.WithLogger(ctx, "kube-gateway-xds-snapshot")
	logger := contextutils.LoggerFrom(ctx)
	snapHash := hashutils.MustHash(snap)
	metaKey := xds.SnapshotCacheKey(proxy)
	logger.Infof("build xds snapshot for proxy %v (%v upstreams, %v endpoints, %v secrets, %v artifacts, %v auth configs, %v rate limit configs)",
		metaKey, len(snap.Upstreams), len(snap.Endpoints), len(snap.Secrets), len(snap.Artifacts), len(snap.AuthConfigs), len(snap.Ratelimitconfigs))
	defer logger.Infof("end sync %v", snapHash)

	proxyCtx := ctx
	if ctxWithTags, err := tag.New(proxyCtx, tag.Insert(syncerstats.ProxyNameKey, metaKey)); err == nil {
		proxyCtx = ctxWithTags
	}

	// Reports used to aggregate results from xds and extension translation.
	// Will contain reports only `Gloo` components (i.e. Proxies, Upstreams, AuthConfigs, etc.)
	allReports := make(reporter.ResourceReports)

	// we need to track and report upstreams, even though this is possibly duplicate work with the latest syncer
	// the reason for this is because we need to set Upstream status in lieu of an edge proxy being translated
	// accept upstreams in snap so we can report accepted status (without this we wouldn't know to report on positive)
	allReports.Accept(snap.Upstreams.AsInputResources()...)
	// TODO: deprecate UsGroups -- not used in GW API! (and not really in edge either...)
	// allReports.Accept(snap.UpstreamGroups.AsInputResources()...)

	params := plugins.Params{
		Ctx:      proxyCtx,
		Settings: s.settings,
		Snapshot: snap,
		Messages: map[*core.ResourceRef][]string{},
	}

	xdsSnapshot, reports, proxyReport := s.translator.Translate(params, proxy)

	// Messages are aggregated during translation, and need to be added to reports
	for _, messages := range params.Messages {
		reports.AddMessages(proxy, messages...)
	}

	allReports.Merge(reports)

	s.syncExtensions(ctx, snap, allReports)

	return xdsSnapshot, allReports, proxyReport
}

func (s *ProxyTranslator) syncXdsAndStatus(
	ctx context.Context,
	snap *xds.EnvoySnapshot,
	proxyKey string,
	reports reporter.ResourceReports,
) error {
	ctx = contextutils.WithLogger(ctx, "kube-gateway-xds-syncer")
	logger := contextutils.LoggerFrom(ctx)
	// snapHash := hashutils.MustHash(snap)
	// TODO: add versions?
	logger.Infof("begin kube gw sync for proxy %v (%v listeners, %v clusters, %v routes, %v endpoints)",
		proxyKey, len(snap.Listeners.Items), len(snap.Clusters.Items), len(snap.Routes.Items), len(snap.Endpoints.Items))
	// defer logger.Infof("end sync %v", snapHash)

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	if contextutils.GetLogLevel() == zapcore.DebugLevel {
		// logger.Debug(syncutil.StringifySnapshot(snap))
	}

	// if the snapshot is not consistent, make it so
	snap.MakeConsistent()
	s.xdsCache.SetSnapshot(proxyKey, snap)

	return s.syncStatus(ctx, reports)
}

func (s *ProxyTranslator) syncStatus(ctx context.Context, reports reporter.ResourceReports) error {
	// leftover from translator_syncer's statusSyncer
	// analyze our plan for concurrency, data ownership, do we need locks, etc.?

	// s.reportsLock.RLock()
	// // deep copy the reports so we can release the lock
	// reports := make(reporter.ResourceReports, len(s.latestReports))
	// for k, v := range s.latestReports {
	// 	reports[k] = v
	// }
	// s.reportsLock.RUnlock()

	logger := contextutils.LoggerFrom(ctx)
	logger.Debugf("gloo reports to be written: %v", reports)
	// if s.identity.IsLeader() {
	if err := s.glooReporter.WriteReports(ctx, reports, nil); err != nil {
		logger.Debugf("Failed writing report for proxies: %v", err)
		return err
	}
	return nil
}

// syncExtensions executes each of the TranslatorSyncerExtensions
// we do not actually set the xds cache for these extensions here, we only aggregate the reports
// from a NoOp sync into the provided reports
func (s *ProxyTranslator) syncExtensions(
	ctx context.Context,
	snap *v1snap.ApiSnapshot,
	reports reporter.ResourceReports,
) {
	for _, syncerExtension := range s.syncerExtensions {
		intermediateReports := make(reporter.ResourceReports)
		// we use the no-op setter here as we don't actually sync the extensions here,
		// that is classic edge syncer's job [see: projects/gloo/pkg/syncer/translator_syncer.go#Sync(...)]
		// all we care about is getting the reports, as our `Proxies` will get reports for errors/warns
		// related to the extension processing
		syncerExtension.Sync(ctx, snap, s.settings, s.noopSnapSetter, intermediateReports)
		reports.Merge(intermediateReports)
	}
}
