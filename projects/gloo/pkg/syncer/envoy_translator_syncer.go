package syncer

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/log"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/reporter"
	"github.com/solo-io/solo-kit/pkg/errors"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.opencensus.io/trace"
	"go.uber.org/zap"
)

var (
	envoySnapshotOut   = stats.Int64("api.gloo.solo.io/translator/resources", "The number of resources in the snapshot in", "1")
	resourceNameKey, _ = tag.NewKey("resource")

	envoySnapshotOutView = &view.View{
		Name:        "api.gloo.solo.io/translator/resources",
		Measure:     envoySnapshotOut,
		Description: "The number of resources in the snapshot for envoy",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{proxyNameKey, resourceNameKey},
	}
)

func init() {
	_ = view.Register(envoySnapshotOutView)
}

func measureResource(ctx context.Context, resource string, len int) {
	if ctxWithTags, err := tag.New(ctx, tag.Insert(resourceNameKey, resource)); err == nil {
		stats.Record(ctxWithTags, envoySnapshotOut.M(int64(len)))
	}
}

func (s *translatorSyncer) syncEnvoy(ctx context.Context, snap *v1.ApiSnapshot) error {
	ctx, span := trace.StartSpan(ctx, "gloo.syncer.Sync")
	defer span.End()

	s.latestSnap = snap
	ctx = contextutils.WithLogger(ctx, "translatorSyncer")
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("begin sync %v (%v proxies, %v upstreams, %v endpoints, %v secrets, %v artifacts, )", snap.Hash(),
		len(snap.Proxies), len(snap.Upstreams), len(snap.Endpoints), len(snap.Secrets), len(snap.Artifacts))
	defer logger.Infof("end sync %v", snap.Hash())

	logger.Debugf("%v", snap)
	allResourceErrs := make(reporter.ResourceErrors)
	allResourceErrs.Accept(snap.Upstreams.AsInputResources()...)
	allResourceErrs.Accept(snap.Upstreamgroups.AsInputResources()...)
	allResourceErrs.Accept(snap.Proxies.AsInputResources()...)

	s.xdsHasher.SetKeysFromProxies(snap.Proxies)

	for _, proxy := range snap.Proxies {
		proxyCtx := ctx
		if ctxWithTags, err := tag.New(proxyCtx, tag.Insert(proxyNameKey, proxy.Metadata.Ref().Key())); err == nil {
			proxyCtx = ctxWithTags
		}

		params := plugins.Params{
			Ctx:      proxyCtx,
			Snapshot: snap,
		}

		xdsSnapshot, resourceErrs, err := s.translator.Translate(params, proxy)
		if err != nil {
			err := errors.Wrapf(err, "translation loop failed")
			logger.DPanicw("", zap.Error(err))
			return err
		}

		allResourceErrs.Merge(resourceErrs)

		if xdsSnapshot, err = validateSnapshot(snap, xdsSnapshot, resourceErrs, logger); err != nil {
			logger.Warnf("proxy %v was rejected due to invalid config: %v\nxDS cache will not be updated.", proxy.Metadata.Ref().Key(), err)
			continue
		}
		key := xds.SnapshotKey(proxy)
		if err := s.xdsCache.SetSnapshot(key, xdsSnapshot); err != nil {
			err := errors.Wrapf(err, "failed while updating xDS snapshot cache")
			logger.DPanicw("", zap.Error(err))
			return err
		}

		clustersLen := len(xdsSnapshot.GetResources(xds.ClusterType).Items)
		listenersLen := len(xdsSnapshot.GetResources(xds.ListenerType).Items)
		routesLen := len(xdsSnapshot.GetResources(xds.RouteType).Items)
		endpointsLen := len(xdsSnapshot.GetResources(xds.EndpointType).Items)

		measureResource(proxyCtx, "clusters", clustersLen)
		measureResource(proxyCtx, "listeners", listenersLen)
		measureResource(proxyCtx, "routes", routesLen)
		measureResource(proxyCtx, "endpoints", endpointsLen)

		logger.Infow("Setting xDS Snapshot", "key", key,
			"clusters", clustersLen,
			"listeners", listenersLen,
			"routes", routesLen,
			"endpoints", endpointsLen)

		logger.Debugf("Full snapshot for proxy %v: %v", proxy.Metadata.Name, xdsSnapshot)
	}
	if err := s.reporter.WriteReports(ctx, allResourceErrs, nil); err != nil {
		logger.Debugf("Failed writing report for proxies: %v", err)
		return errors.Wrapf(err, "writing reports")
	}
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

func validateSnapshot(snap *v1.ApiSnapshot, snapshot envoycache.Snapshot, errs reporter.ResourceErrors, logger *zap.SugaredLogger) (envoycache.Snapshot, error) {

	// find all the errored upstreams and remove them. if the snapshot is still consistent, we are good to go
	resourcesErr := errs.Validate()
	if resourcesErr == nil {
		return snapshot, nil
	}

	logger.Debug("removing errored upstream and checking consistency")

	clusters := snapshot.GetResources(xds.ClusterType)
	endpoints := snapshot.GetResources(xds.EndpointType)

	for _, up := range snap.Upstreams.AsInputResources() {
		if errs[up] != nil {
			clusterName := translator.UpstreamToClusterName(up.GetMetadata().Ref())
			// remove cluster and endpoints
			delete(clusters.Items, clusterName)
			delete(endpoints.Items, clusterName)
		}
	}

	snapshot = xds.NewSnapshotFromResources(
		endpoints,
		clusters,
		snapshot.GetResources(xds.RouteType),
		snapshot.GetResources(xds.ListenerType),
	)

	if snapshot.Consistent() != nil {
		return snapshot, resourcesErr
	}

	// snapshot is consistent, so check if we have errors not related to the upstreams
	for _, up := range snap.Upstreams.AsInputResources() {
		if errs[up] != nil {
			delete(errs, up)
		}
	}

	resourcesErr = errs.Validate()
	return snapshot, resourcesErr
}
