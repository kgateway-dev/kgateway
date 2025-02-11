package proxy_syncer

import (
	"fmt"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"go.uber.org/zap"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
)

func snapshotPerClient(
	l *zap.Logger,
	krtopts krtutil.KrtOptions,
	uccCol krt.Collection[ir.UniqlyConnectedClient],
	mostXdsSnapshots krt.Collection[GatewayXdsResources],
	endpoints PerClientEnvoyEndpoints,
	clusters PerClientEnvoyClusters,
) krt.Collection[XdsSnapWrapper] {

	xdsSnapshotsForUcc := krt.NewCollection(uccCol, func(kctx krt.HandlerContext, ucc ir.UniqlyConnectedClient) *XdsSnapWrapper {
		maybeMostlySnap := krt.FetchOne(kctx, mostXdsSnapshots, krt.FilterKey(ucc.Role))
		if maybeMostlySnap == nil {
			l.Debug("snapshotPerClient - snapshot missing", zap.String("proxyKey", ucc.Role))
			return nil
		}
		clustersForUcc := clusters.FetchClustersForClient(kctx, ucc)

		clustersProto := make([]envoycachetypes.ResourceWithTTL, 0, len(clustersForUcc)+len(maybeMostlySnap.Clusters))
		var clustersHash uint64
		var erroredClusters []string
		for _, c := range clustersForUcc {
			if c.Error == nil {
				clustersProto = append(clustersProto, envoycachetypes.ResourceWithTTL{Resource: c.Cluster})
				clustersHash ^= c.ClusterVersion
			} else {
				erroredClusters = append(erroredClusters, c.Name)
			}
		}
		clustersProto = append(clustersProto, maybeMostlySnap.Clusters...)
		clustersHash ^= maybeMostlySnap.ClustersHash
		clustersVersion := fmt.Sprintf("%d", clustersHash)

		endpointsForUcc := endpoints.FetchEndpointsForClient(kctx, ucc)
		endpointsProto := make([]envoycachetypes.ResourceWithTTL, 0, len(endpointsForUcc))
		var endpointsHash uint64
		for _, ep := range endpointsForUcc {
			endpointsProto = append(endpointsProto, envoycachetypes.ResourceWithTTL{Resource: ep.Endpoints})
			endpointsHash ^= ep.EndpointsHash
		}

		snap := XdsSnapWrapper{}

		clusterResources := envoycache.NewResourcesWithTTL(clustersVersion, clustersProto)
		endpointResources := envoycache.NewResourcesWithTTL(fmt.Sprintf("%d", endpointsHash), endpointsProto)
		snap.erroredClusters = erroredClusters
		snap.proxyKey = ucc.ResourceName()
		snapshot := &envoycache.Snapshot{}
		snapshot.Resources[envoycachetypes.Cluster] = clusterResources //envoycache.NewResources(version, resource)
		snapshot.Resources[envoycachetypes.Endpoint] = endpointResources
		snapshot.Resources[envoycachetypes.Route] = maybeMostlySnap.Routes
		snapshot.Resources[envoycachetypes.Listener] = maybeMostlySnap.Listeners
		//envoycache.NewResources(version, resource)
		snap.snap = snapshot
		l.Debug("snapshotPerClient", zap.String("proxyKey", snap.proxyKey),
			zap.Stringer("Listeners", resourcesStringer(maybeMostlySnap.Listeners)),
			zap.Stringer("Clusters", resourcesStringer(clusterResources)),
			zap.Stringer("Routes", resourcesStringer(maybeMostlySnap.Routes)),
			zap.Stringer("Endpoints", resourcesStringer(endpointResources)),
		)

		return &snap
	}, krtopts.ToOptions("PerClientXdsSnapshots")...)
	return xdsSnapshotsForUcc
}
