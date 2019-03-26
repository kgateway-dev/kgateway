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
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/reporter"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/log"
	"go.opencensus.io/tag"
	"go.opencensus.io/trace"
	"go.uber.org/zap"
)

func (s *translatorSyncer) syncEnvoy(ctx context.Context, snap *v1.ApiSnapshot) error {

	ctx, span := trace.StartSpan(ctx, "gloo.syncer.Sync")
	defer span.End()

	s.latestSnap = snap
	ctx = contextutils.WithLogger(ctx, "translatorSyncer")
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("begin sync %v (%v proxies, %v upstreams, %v endpoints, %v secrets, %v artifacts, )", snap.Hash(),
		len(snap.Proxies.List()), len(snap.Upstreams.List()), len(snap.Endpoints.List()), len(snap.Secrets.List()), len(snap.Artifacts.List()))
	defer logger.Infof("end sync %v", snap.Hash())

	logger.Debugf("%v", snap)
	allResourceErrs := make(reporter.ResourceErrors)
	allResourceErrs.Accept(snap.Upstreams.List().AsInputResources()...)
	allResourceErrs.Accept(snap.Proxies.List().AsInputResources()...)

	s.xdsHasher.SetKeysFromProxies(snap.Proxies.List())

	for _, proxy := range snap.Proxies.List() {
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
			logger.Warnf("proxy %v was rejected due to invalid config: %v\nxDS cache will not be updated.", err)
			continue
		}
		key := xds.SnapshotKey(proxy)
		if err := s.xdsCache.SetSnapshot(key, xdsSnapshot); err != nil {
			err := errors.Wrapf(err, "failed while updating xds snapshot cache")
			logger.DPanicw("", zap.Error(err))
			return err
		}

		logger.Infow("Setting xDS Snapshot", "key", key,
			"clusters", len(xdsSnapshot.GetResources(xds.ClusterType).Items),
			"listeners", len(xdsSnapshot.GetResources(xds.ListenerType).Items),
			"routes", len(xdsSnapshot.GetResources(xds.RouteType).Items),
			"endpoints", len(xdsSnapshot.GetResources(xds.EndpointType).Items))

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
		fmt.Fprintf(w, log.Sprintf("%v", s.xdsCache))
	})
	r.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, log.Sprintf("%v", s.latestSnap))
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

	for _, up := range snap.Upstreams.List().AsInputResources() {
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
	for _, up := range snap.Upstreams.List().AsInputResources() {
		if errs[up] != nil {
			delete(errs, up)
		}
	}

	resourcesErr = errs.Validate()
	return snapshot, resourcesErr
}
