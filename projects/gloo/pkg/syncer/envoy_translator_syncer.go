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
	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/go-utils/log"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
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
	ctx = contextutils.WithLogger(ctx, "envoyTranslatorSyncer")
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("begin sync %v (%v proxies, %v upstreams, %v endpoints, %v secrets, %v artifacts, %v auth configs)", snap.Hash(),
		len(snap.Proxies), len(snap.Upstreams), len(snap.Endpoints), len(snap.Secrets), len(snap.Artifacts), len(snap.AuthConfigs))
	defer logger.Infof("end sync %v", snap.Hash())

	logger.Debugf("%v", snap)
	allReports := make(reporter.ResourceReports)
	allReports.Accept(snap.Upstreams.AsInputResources()...)
	allReports.Accept(snap.UpstreamGroups.AsInputResources()...)
	allReports.Accept(snap.Proxies.AsInputResources()...)

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

		xdsSnapshot, reports, _, err := s.translator.Translate(params, proxy)
		if err != nil {
			err := errors.Wrapf(err, "translation loop failed")
			logger.DPanicw("", zap.Error(err))
			return err
		}

		allReports.Merge(reports)

		key := xds.SnapshotKey(proxy)

		xdsSnapshot, err = validateSnapshot(snap, xdsSnapshot, reports, logger)
		if err != nil {
			logger.Warnf("proxy %v was rejected due to invalid config: %v\n"+
				"Attempting to update only EDS information", proxy.Metadata.Ref().Key(), err)

			// If the snapshot is invalid, attempt at least to update the EDS information. This is important because
			// endpoints are relatively ephemeral entities and the previous snapshot Envoy got might be stale by now.
			xdsSnapshot, err = s.updateEndpointsOnly(key, xdsSnapshot)
			if err != nil {
				logger.Warnf("endpoint update failed. xDS snapshot for proxy %v will not be updated. "+
					"Error is: %s", proxy.Metadata.Ref().Key(), err)
				continue
			}
			logger.Infof("successfully updated EDS information for proxy %v", proxy.Metadata.Ref().Key())
		}

		if err := s.xdsCache.SetSnapshot(key, xdsSnapshot); err != nil {
			err := errors.Wrapf(err, "failed while updating xDS snapshot cache")
			logger.DPanicw("", zap.Error(err))
			return err
		}

		// Record some metrics
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

	if err := s.reporter.WriteReports(ctx, allReports, nil); err != nil {
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

// If there are any errors on upstreams, this function tries to remove the correspondent clusters and endpoints from
// the xDS snapshot. If the snapshot is still consistent after these mutations and there are no errors related to other
// resources, we are good to send it to Envoy.
func validateSnapshot(glooSnapshot *v1.ApiSnapshot, xdsSnapshot envoycache.Snapshot, errs reporter.ResourceReports, logger *zap.SugaredLogger) (envoycache.Snapshot, error) {
	resourcesErr := errs.Validate()
	if resourcesErr == nil {
		return xdsSnapshot, nil
	}

	logger.Debug("removing errored upstreams and checking consistency")

	clusters := xdsSnapshot.GetResources(xds.ClusterType)
	endpoints := xdsSnapshot.GetResources(xds.EndpointType)

	// Find all the errored upstreams and remove them from the xDS snapshot
	for _, up := range glooSnapshot.Upstreams.AsInputResources() {
		if errs[up].Errors != nil {
			clusterName := translator.UpstreamToClusterName(up.GetMetadata().Ref())
			// remove cluster and endpoints
			delete(clusters.Items, clusterName)
			delete(endpoints.Items, clusterName)
		}
	}

	// TODO(marco): the function accepts and return a Snapshot interface, but then swaps in its own implementation.
	//  This breaks the abstraction and mocking the snapshot becomes impossible. We should have a generic way of
	//  creating snapshots.
	xdsSnapshot = xds.NewSnapshotFromResources(
		endpoints,
		clusters,
		xdsSnapshot.GetResources(xds.RouteType),
		xdsSnapshot.GetResources(xds.ListenerType),
	)

	// If the snapshot is not consistent,
	if xdsSnapshot.Consistent() != nil {
		return xdsSnapshot, resourcesErr
	}

	// Remove errors related to upstreams
	for _, up := range glooSnapshot.Upstreams.AsInputResources() {
		if errs[up].Errors != nil {
			delete(errs, up)
		}
	}

	// Snapshot is consistent, so check if we have errors not related to the upstreams
	resourcesErr = errs.Validate()

	return xdsSnapshot, resourcesErr
}

// TODO(marco): should we update CDS resources as well?
// Builds an xDS snapshot by combining:
// - CDS/LDS/RDS information from the previous xDS snapshot
// - EDS from the Gloo API snapshot translated curing this sync
// The resulting snapshot will be checked for consistency before being returned.
func (s *translatorSyncer) updateEndpointsOnly(snapshotKey string, current envoycache.Snapshot) (envoycache.Snapshot, error) {

	// Get a copy of the last successful snapshot
	previous, err := s.xdsCache.GetSnapshot(snapshotKey)
	if err != nil {
		return nil, err
	}

	newSnapshot := xds.NewSnapshotFromResources(
		// Set endpoints and clusters calculated during this sync
		current.GetResources(xds.EndpointType),
		current.GetResources(xds.ClusterType),
		// Keep other resources from previous snapshot
		previous.GetResources(xds.RouteType),
		previous.GetResources(xds.ListenerType),
	)

	if err := newSnapshot.Consistent(); err != nil {
		return nil, err
	}

	return newSnapshot, nil
}
