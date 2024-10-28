package proxy_syncer

import (
	"context"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/hashutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"istio.io/istio/pkg/kube/krt"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	"github.com/solo-io/gloo/pkg/utils/syncutil"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"

	"go.uber.org/zap/zapcore"
)

// buildXdsSnapshot will translate from a gloov1.Proxy to xdsSnapshot using the supplied api snapshot.
// This method returns the generated xdsSnapshot along with a combined report of proxy->xds translation and extension processing on the Proxy.
// NOTE: Extensions are NOT actually synced here as use a NoOp snapshot when running the extension syncers.
// The actual syncing of the extensions and the status of the extension resources (e.g. AuthConfigs, RLCs) is still handled by the legacy syncer.
func (s *ProxyTranslator) buildXdsSnapshot(
	kctx krt.HandlerContext,
	ctx context.Context,
	proxy *v1.Proxy,
	snap *v1snap.ApiSnapshot,
) (cache.Snapshot, reporter.ResourceReports, *validation.ProxyReport) {
	metaKey := xds.SnapshotCacheKey(proxy)

	ctx = contextutils.WithLogger(ctx, "kube-gateway-xds-snapshot")
	logger := contextutils.LoggerFrom(ctx).With("proxy", metaKey)

	logger.Infof("build xds snapshot for proxy %v (%d upstreams, %d endpoints, %d secrets, %d artifacts, %d auth configs, %d rate limit configs)",
		metaKey, len(snap.Upstreams), len(snap.Endpoints), len(snap.Secrets), len(snap.Artifacts), len(snap.AuthConfigs), len(snap.Ratelimitconfigs))
	snapHash := hashutils.MustHash(snap)
	defer logger.Infof("end sync %v", snapHash)

	// Reports used to aggregate results from xds and extension translation.
	// Will contain reports only `Gloo` components (i.e. Proxies, Upstreams, AuthConfigs, etc.)
	allReports := make(reporter.ResourceReports)

	// we need to track and report upstreams, even though this is possibly duplicate work with the legacy syncer
	// the reason for this is because we need to set Upstream status even if no edge proxies are being translated
	// here we Accept() upstreams in snap so we can report accepted status (without this we wouldn't report on positive case)
	allReports.Accept(snap.Upstreams.AsInputResources()...)
	ksettings := krt.FetchOne(kctx, s.settings.AsCollection())
	settings := &ksettings.Spec

	ctx = settingsutil.WithSettings(ctx, settings)

	params := plugins.Params{
		Ctx:      ctx,
		Settings: settings,
		Snapshot: snap,
		Messages: map[*core.ResourceRef][]string{},
	}

	xdsSnapshot, reports, proxyReport := s.translator.NewTranslator(ctx, settings).Translate(params, proxy)

	// Messages are aggregated during translation, and need to be added to reports
	for _, messages := range params.Messages {
		reports.AddMessages(proxy, messages...)
	}

	allReports.Merge(reports)

	// run through extensions to get extension reports and updated Proxy reports
	for _, syncerExtension := range s.syncerExtensions {
		intermediateReports := make(reporter.ResourceReports)
		// we use the no-op setter here as we don't actually sync the extensions here,
		// that is classic edge syncer's job [see: projects/gloo/pkg/syncer/translator_syncer.go#Sync(...)]
		// all we care about is getting the reports, as our `Proxies` will get reports for errors/warns
		// related to the extension processing
		syncerExtension.Sync(ctx, snap, settings, s.noopSnapSetter, intermediateReports)
		allReports.Merge(intermediateReports)
	}

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
	logger.Infof("begin kube gw sync for proxy %s (%v listeners, %v clusters, %v routes, %v endpoints)",
		proxyKey, len(snap.Listeners.Items), len(snap.Clusters.Items), len(snap.Routes.Items), len(snap.Endpoints.Items))
	defer logger.Infof("end kube gw sync for proxy %s", proxyKey)

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	if contextutils.GetLogLevel() == zapcore.DebugLevel {
		logger.Debugw(syncutil.StringifySnapshot(snap), "proxyKey", proxyKey)
	}

	// if the snapshot is not consistent, make it so
	snap.MakeConsistent()
	s.xdsCache.SetSnapshot(proxyKey, snap)

	// TODO: only leaders should write status (https://github.com/solo-io/solo-projects/issues/6367)
	logger.Debugf("gloo reports for proxy %s to be written: %v", proxyKey, reports)
	if err := s.glooReporter.WriteReports(ctx, reports, nil); err != nil {
		logger.Errorf("Failed writing gloo reports for proxy %s: %v", proxyKey, err)
		return err
	}
	return nil
}
