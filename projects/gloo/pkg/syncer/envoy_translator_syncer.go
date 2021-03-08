package syncer

import (
	"context"
	"fmt"
	"net/http"

	syncerstats "github.com/solo-io/gloo/projects/gloo/pkg/syncer/stats"
	"github.com/solo-io/go-utils/hashutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"

	"github.com/gorilla/mux"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/syncutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.opencensus.io/trace"
	"go.uber.org/zap"
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

// empty resources to give to envoy when a proxy was deleted
const emptyVersionKey = "empty"

var (
	emptyResource = cache.Resources{
		Version: emptyVersionKey,
		Items:   map[string]envoycache.Resource{},
	}
	emptySnapshot = xds.NewSnapshotFromResources(
		emptyResource,
		emptyResource,
		emptyResource,
		emptyResource,
	)
)

func measureResource(ctx context.Context, resource string, len int) {
	if ctxWithTags, err := tag.New(ctx, tag.Insert(resourceNameKey, resource)); err == nil {
		stats.Record(ctxWithTags, envoySnapshotOut.M(int64(len)))
	}
}

func (s *translatorSyncer) syncEnvoy(ctx context.Context, snap *v1.ApiSnapshot, allReports reporter.ResourceReports) error {
	ctx, span := trace.StartSpan(ctx, "gloo.syncer.Sync")
	defer span.End()

	s.latestSnap = snap
	ctx = contextutils.WithLogger(ctx, "envoyTranslatorSyncer")
	logger := contextutils.LoggerFrom(ctx)
	snapHash := hashutils.MustHash(snap)
	logger.Infof("begin sync %v (%v proxies, %v upstreams, %v endpoints, %v secrets, %v artifacts, %v auth configs, %v rate limit configs)", snapHash,
		len(snap.Proxies), len(snap.Upstreams), len(snap.Endpoints), len(snap.Secrets), len(snap.Artifacts), len(snap.AuthConfigs), len(snap.Ratelimitconfigs))
	defer logger.Infof("end sync %v", snapHash)

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	if contextutils.GetLogLevel() == zapcore.DebugLevel {
		logger.Debug(syncutil.StringifySnapshot(snap))
	}

	allReports.Accept(snap.Upstreams.AsInputResources()...)
	allReports.Accept(snap.UpstreamGroups.AsInputResources()...)
	allReports.Accept(snap.Proxies.AsInputResources()...)

	if !s.settings.GetGloo().GetDisableProxyGarbageCollection().GetValue() {
		allKeys := map[string]bool{
			xds.FallbackNodeKey: true,
		}
		// Get all envoy node ID keys
		for _, key := range s.xdsCache.GetStatusKeys() {
			allKeys[key] = false
		}
		// Get all valid node ID keys
		for _, key := range xds.GetValidKeys(snap.Proxies, s.extensionKeys) {
			allKeys[key] = true
		}
		// preserve keys from the current list of proxies, set previous invalid snapshots to empty snapshot
		for key, valid := range allKeys {
			if !valid {
				if err := s.xdsCache.SetSnapshot(key, emptySnapshot); err != nil {
					return err
				}
			}
		}
	}

	for _, proxy := range snap.Proxies {
		proxyCtx := ctx
		if ctxWithTags, err := tag.New(proxyCtx, tag.Insert(syncerstats.ProxyNameKey, proxy.Metadata.Ref().Key())); err == nil {
			proxyCtx = ctxWithTags
		}

		params := plugins.Params{
			Ctx:      proxyCtx,
			Snapshot: snap,
		}

		xdsSnapshot, reports, _, err := s.translator.Translate(params, proxy)
		if err != nil {
			err := eris.Wrapf(err, "translation loop failed")
			logger.DPanicw("", zap.Error(err))
			return err
		}

		if validateErr := reports.ValidateStrict(); validateErr != nil {
			logger.Warnw("Proxy had invalid config", zap.Any("proxy", proxy.Metadata.Ref()), zap.Error(validateErr))
		}

		allReports.Merge(reports)

		key := xds.SnapshotKey(proxy)

		sanitizedSnapshot, err := s.sanitizer.SanitizeSnapshot(ctx, snap, xdsSnapshot, reports)
		if err != nil {
			logger.Warnf("proxy %v was rejected due to invalid config: %v\n"+
				"Attempting to update only EDS information", proxy.Metadata.Ref().Key(), err)

			// If the snapshot is invalid, attempt at least to update the EDS information. This is important because
			// endpoints are relatively ephemeral entities and the previous snapshot Envoy got might be stale by now.
			sanitizedSnapshot, err = s.updateEndpointsOnly(key, xdsSnapshot)
			if err != nil {
				logger.Warnf("endpoint update failed. xDS snapshot for proxy %v will not be updated. "+
					"Error is: %s", proxy.Metadata.Ref().Key(), err)
				continue
			}
			logger.Infof("successfully updated EDS information for proxy %v", proxy.Metadata.Ref().Key())
		}

		if err := s.xdsCache.SetSnapshot(key, sanitizedSnapshot); err != nil {
			err := eris.Wrapf(err, "failed while updating xDS snapshot cache")
			logger.DPanicw("", zap.Error(err))
			return err
		}

		// Record some metrics
		clustersLen := len(xdsSnapshot.GetResources(resource.ClusterTypeV3).Items)
		listenersLen := len(xdsSnapshot.GetResources(resource.ListenerTypeV3).Items)
		routesLen := len(xdsSnapshot.GetResources(resource.RouteTypeV3).Items)
		endpointsLen := len(xdsSnapshot.GetResources(resource.EndpointTypeV3).Items)

		measureResource(proxyCtx, "clusters", clustersLen)
		measureResource(proxyCtx, "listeners", listenersLen)
		measureResource(proxyCtx, "routes", routesLen)
		measureResource(proxyCtx, "endpoints", endpointsLen)

		logger.Infow("Setting xDS Snapshot", "key", key,
			"clusters", clustersLen,
			"listeners", listenersLen,
			"routes", routesLen,
			"endpoints", endpointsLen)

		logger.Debugf("Full snapshot for proxy %v: %+v", proxy.Metadata.Name, xdsSnapshot)
	}

	logger.Debugf("gloo reports to be written: %v", allReports)

	return nil
}

// TODO(ilackarms): move this somewhere else, make it part of dev-mode
func (s *translatorSyncer) ServeXdsSnapshots() error {
	r := mux.NewRouter()
	r.HandleFunc("/xds", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, log.Sprintf("%v", s.xdsCache))
	})
	r.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, log.Sprintf("%v", s.latestSnap))
	})
	return http.ListenAndServe(":10010", r)
}

// TODO(marco): should we update CDS resources as well?
// Builds an xDS snapshot by combining:
// - CDS/LDS/RDS information from the previous xDS snapshot
// - EDS from the Gloo API snapshot translated curing this sync
// The resulting snapshot will be checked for consistency before being returned.
func (s *translatorSyncer) updateEndpointsOnly(snapshotKey string, current envoycache.Snapshot) (envoycache.Snapshot, error) {
	var newSnapshot cache.Snapshot

	// Get a copy of the last successful snapshot
	previous, err := s.xdsCache.GetSnapshot(snapshotKey)
	if err != nil {
		// if no previous snapshot exists
		newSnapshot = xds.NewEndpointsSnapshotFromResources(
			current.GetResources(resource.EndpointTypeV3),
			current.GetResources(resource.ClusterTypeV3),
		)
	} else {
		newSnapshot = xds.NewSnapshotFromResources(
			// Set endpoints and clusters calculated during this sync
			current.GetResources(resource.EndpointTypeV3),
			current.GetResources(resource.ClusterTypeV3),
			// Keep other resources from previous snapshot
			previous.GetResources(resource.RouteTypeV3),
			previous.GetResources(resource.ListenerTypeV3),
		)
	}

	if err := newSnapshot.Consistent(); err != nil {
		return nil, err
	}

	return newSnapshot, nil
}
