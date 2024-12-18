package proxy_syncer

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/utils/krtutil"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"go.uber.org/zap"
	"istio.io/istio/pkg/kube/krt"
)

func snapshotPerClient(l *zap.Logger, krtopts krtutil.KrtOptions, uccCol krt.Collection[ir.UniqlyConnectedClient],
	mostXdsSnapshots krt.Collection[GatewayXdsResources], endpoints PerClientEnvoyEndpoints, clusters PerClientEnvoyClusters) krt.Collection[XdsSnapWrapper] {

	xdsSnapshotsForUcc := krt.NewCollection(uccCol, func(kctx krt.HandlerContext, ucc ir.UniqlyConnectedClient) *XdsSnapWrapper {
		maybeMostlySnap := krt.FetchOne(kctx, mostXdsSnapshots, krt.FilterKey(ucc.Role))
		if maybeMostlySnap == nil {
			l.Debug("snapshotPerClient - snapshot missing", zap.String("proxyKey", ucc.Role))
			return nil
		}
		clustersForUcc := clusters.FetchClustersForClient(kctx, ucc)

		clustersProto := make([]envoycache.Resource, 0, len(clustersForUcc)+len(maybeMostlySnap.Clusters))
		var clustersHash uint64
		var erroredClusters []string
		for _, c := range clustersForUcc {
			if c.Error == nil {
				clustersProto = append(clustersProto, c.Cluster)
				clustersHash ^= c.ClusterVersion
			} else {
				erroredClusters = append(erroredClusters, c.Name)
			}
		}
		clustersProto = append(clustersProto, maybeMostlySnap.Clusters...)
		clustersHash ^= maybeMostlySnap.ClustersHash
		clustersVersion := fmt.Sprintf("%d", clustersHash)

		endpointsForUcc := endpoints.FetchEndpointsForClient(kctx, ucc)
		endpointsProto := make([]envoycache.Resource, 0, len(endpointsForUcc))
		var endpointsHash uint64
		for _, ep := range endpointsForUcc {
			endpointsProto = append(endpointsProto, ep.Endpoints)
			endpointsHash ^= ep.EndpointsHash
		}

		snap := XdsSnapWrapper{}

		clusterResources := envoycache.NewResources(clustersVersion, clustersProto)
		snap.erroredClusters = erroredClusters
		snap.proxyKey = ucc.ResourceName()
		snap.snap = &xds.EnvoySnapshot{
			Clusters:  clusterResources,
			Endpoints: envoycache.NewResources(fmt.Sprintf("%d", endpointsHash), endpointsProto),
			Routes:    maybeMostlySnap.Routes,
			Listeners: maybeMostlySnap.Listeners,
		}
		l.Debug("snapshotPerClient", zap.String("proxyKey", snap.proxyKey),
			zap.Stringer("Listeners", resourcesStringer(maybeMostlySnap.Listeners)),
			zap.Stringer("Clusters", resourcesStringer(snap.snap.Clusters)),
			zap.Stringer("Routes", resourcesStringer(maybeMostlySnap.Routes)),
			zap.Stringer("Endpoints", resourcesStringer(snap.snap.Endpoints)),
		)

		return &snap
	}, krtopts.ToOptions("PerClientXdsSnapshots")...)
	return xdsSnapshotsForUcc
}
