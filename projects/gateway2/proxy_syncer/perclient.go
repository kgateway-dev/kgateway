package proxy_syncer

import (
	"fmt"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	ggv2utils "github.com/solo-io/gloo/projects/gateway2/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"go.uber.org/zap"
	"istio.io/istio/pkg/kube/krt"
)

func snapshotPerClient(l *zap.Logger, dbg *krt.DebugHandler, uccCol krt.Collection[krtcollections.UniqlyConnectedClient],
	mostXdsSnapshots krt.Collection[xdsSnapWrapper], endpoints PerClientEnvoyEndpoints, clusters PerClientEnvoyClusters) krt.Collection[xdsSnapWrapper] {

	xdsSnapshotsForUcc := krt.NewCollection(uccCol, func(kctx krt.HandlerContext, ucc krtcollections.UniqlyConnectedClient) *xdsSnapWrapper {
		maybeMostlySnap := krt.FetchOne(kctx, mostXdsSnapshots, krt.FilterKey(ucc.Role))
		if maybeMostlySnap == nil {
			l.Debug("snapshotPerClient - snapshot missing", zap.String("proxyKey", ucc.Role))
			return nil
		}
		genericSnap := maybeMostlySnap.snap
		clustersForUcc := clusters.FetchClustersForClient(kctx, ucc)

		clustersProto := make([]envoycache.Resource, 0, len(clustersForUcc))
		var clustersHash uint64
		for _, ep := range clustersForUcc {
			clustersProto = append(clustersProto, ep.Cluster)
			clustersHash ^= ep.ClusterVersion
		}
		clustersVersion := fmt.Sprintf("%d", clustersHash)

		endpointsForUcc := endpoints.FetchEndpointsForClient(kctx, ucc)
		endpointsProto := make([]envoycache.Resource, 0, len(endpointsForUcc))
		var endpointsHash uint64
		for _, ep := range endpointsForUcc {
			endpointsProto = append(endpointsProto, ep.Endpoints)
			endpointsHash ^= ep.EndpointsHash
		}

		mostlySnap := *maybeMostlySnap

		clusterResources := envoycache.NewResources(clustersVersion, clustersProto)
		// add missing generated resource from GeneratedResources plugins.
		// To be able to do individual upstream translation, We need to redo the GeneratedResources,
		// so they don't take as input the entire xds snapshot. the main offender is the tunneling plugin.
		//
		// for now, a manual audit showed that these only add clusters and listeners. As we don't touch the listeners,
		// we just need to account for potentially missing clusters.
		for name, cluster := range genericSnap.Clusters.Items {
			// only copy clusters that don't exist. as we do cluster translation per client,
			// our clusters might be slightly different.
			if _, ok := clusterResources.Items[name]; !ok {
				clusterResources.Items[name] = cluster
				clustersHash ^= ggv2utils.HashProto(cluster.ResourceProto().(*envoy_config_cluster_v3.Cluster))
			}
		}
		clusterResources.Version = fmt.Sprintf("%d", clustersHash)

		mostlySnap.proxyKey = ucc.ResourceName()
		mostlySnap.snap = &xds.EnvoySnapshot{
			Clusters:  clusterResources,
			Endpoints: envoycache.NewResources(fmt.Sprintf("%s-%d", clustersVersion, endpointsHash), endpointsProto),
			Routes:    genericSnap.Routes,
			Listeners: genericSnap.Listeners,
		}
		l.Debug("snapshotPerClient", zap.String("proxyKey", mostlySnap.proxyKey),
			zap.Stringer("Listeners", resourcesStringer(mostlySnap.snap.Listeners)),
			zap.Stringer("Clusters", resourcesStringer(mostlySnap.snap.Clusters)),
			zap.Stringer("Routes", resourcesStringer(mostlySnap.snap.Routes)),
			zap.Stringer("Endpoints", resourcesStringer(mostlySnap.snap.Endpoints)),
		)

		return &mostlySnap
	}, krt.WithDebugging(dbg), krt.WithName("PerClientXdsSnapshots"))
	return xdsSnapshotsForUcc
}
