package syncer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	syncerstats "github.com/solo-io/gloo/projects/gloo/pkg/syncer/stats"
	"github.com/solo-io/go-utils/hashutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/types"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/gorilla/mux"
	"github.com/solo-io/gloo/pkg/utils/syncutil"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
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

const (
	// The port used to expose a developer server
	devModePort = 10010
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

// syncEnvoy will translate, sanatize, and set the snapshot for each of the proxies, all while merging all the reports into allReports.
func (s *translatorSyncer) syncEnvoy(ctx context.Context, snap *v1snap.ApiSnapshot, allReports reporter.ResourceReports) {
	ctx, span := trace.StartSpan(ctx, "gloo.syncer.Sync")
	defer span.End()

	s.latestSnap = snap
	ctx = contextutils.WithLogger(ctx, "envoyTranslatorSyncer")
	logger := contextutils.LoggerFrom(ctx)
	snapHash := hashutils.MustHash(snap)
	logger.Infof("begin sync %v (%v proxies, %v upstreams, %v endpoints, %v secrets, %v artifacts, %v auth configs, %v rate limit configs, %v graphql apis)", snapHash,
		len(snap.Proxies), len(snap.Upstreams), len(snap.Endpoints), len(snap.Secrets), len(snap.Artifacts), len(snap.AuthConfigs), len(snap.Ratelimitconfigs), len(snap.GraphqlApis))
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
			xds.FallbackNodeCacheKey: true,
		}
		// Get all envoy node ID keys
		for _, key := range s.xdsCache.GetStatusKeys() {
			allKeys[key] = false
		}
		// Get all valid node ID keys for Proxies
		for _, key := range xds.SnapshotCacheKeys(snap.Proxies) {
			allKeys[key] = true
		}
		// Get all valid node ID keys for syncerExtensions (rate-limit, ext-auth)
		for _, extension := range s.syncerExtensions {
			allKeys[extension.ID()] = true
		}

		// preserve keys from the current list of proxies, set previous invalid snapshots to empty snapshot
		for key, valid := range allKeys {
			if !valid {
				s.xdsCache.SetSnapshot(key, emptySnapshot)
			}
		}
	}
	for _, proxy := range snap.Proxies {
		proxyCtx := ctx
		if ctxWithTags, err := tag.New(proxyCtx, tag.Insert(syncerstats.ProxyNameKey, proxy.GetMetadata().Ref().Key())); err == nil {
			proxyCtx = ctxWithTags
		}

		params := plugins.Params{
			Ctx:      proxyCtx,
			Snapshot: snap,
			Messages: map[*core.ResourceRef][]string{},
		}

		xdsSnapshot, reports, _ := s.translator.Translate(params, proxy)

		// Messages are aggregated during translation, and need to be added to reports
		for _, messages := range params.Messages {
			reports.AddMessages(proxy, messages...)
		}

		if validateErr := reports.ValidateStrict(); validateErr != nil {
			logger.Warnw("Proxy had invalid config", zap.Any("proxy", proxy.GetMetadata().Ref()), zap.Error(validateErr))
		}

		sanitizedSnapshot := s.sanitizer.SanitizeSnapshot(ctx, snap, xdsSnapshot, reports)
		// if the snapshot is not consistent, make it so
		xdsSnapshot.MakeConsistent()

		if validateErr := reports.ValidateStrict(); validateErr != nil {
			logger.Warnw("Proxy had invalid config after xds sanitization", zap.Any("proxy", proxy.GetMetadata().Ref()), zap.Error(validateErr))
		}

		// Merge reports after sanitization to capture changes made by the sanitizers
		allReports.Merge(reports)
		key := xds.SnapshotCacheKey(proxy)
		s.xdsCache.SetSnapshot(key, sanitizedSnapshot)

		// Record some metrics
		clustersLen := len(xdsSnapshot.GetResources(types.ClusterTypeV3).Items)
		listenersLen := len(xdsSnapshot.GetResources(types.ListenerTypeV3).Items)
		routesLen := len(xdsSnapshot.GetResources(types.RouteTypeV3).Items)
		endpointsLen := len(xdsSnapshot.GetResources(types.EndpointTypeV3).Items)

		measureResource(proxyCtx, "clusters", clustersLen)
		measureResource(proxyCtx, "listeners", listenersLen)
		measureResource(proxyCtx, "routes", routesLen)
		measureResource(proxyCtx, "endpoints", endpointsLen)

		logger.Infow("Setting xDS Snapshot", "key", key,
			"clusters", clustersLen,
			"listeners", listenersLen,
			"routes", routesLen,
			"endpoints", endpointsLen)

		logger.Debugf("Full snapshot for proxy %v: %+v", proxy.GetMetadata().GetName(), xdsSnapshot)
	}

	logger.Debugf("gloo reports to be written: %v", allReports)
}

// ServeXdsSnapshots exposes Gloo configuration as an API when `devMode` in Settings is True.
// TODO(ilackarms): move this somewhere else, make it part of dev-mode
// https://github.com/solo-io/gloo/issues/6494
func (s *translatorSyncer) ServeXdsSnapshots() error {
	r := mux.NewRouter()

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%v", "Developer API")
	})
	r.HandleFunc("/xds", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%+v", prettify(s.xdsCache.GetStatusKeys()))
	})
	r.HandleFunc("/xds/{key}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		xdsCacheKey := vars["key"]

		xdsSnapshot, _ := s.xdsCache.GetSnapshot(xdsCacheKey)
		_, _ = fmt.Fprintf(w, "%+v", prettify(xdsSnapshot))
	})
	r.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%+v", prettify(s.latestSnap))
	})

	return http.ListenAndServe(fmt.Sprintf(":%d", devModePort), r)
}

func prettify(original interface{}) string {
	b, err := json.MarshalIndent(original, "", "    ")
	if err != nil {
		return ""
	}

	return string(b)
}
