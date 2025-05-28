package proxy_syncer

import (
	"fmt"
	"maps"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
)

type clustersWithErrors struct {
	clusters            envoycache.Resources
	erroredClusters     []string
	erroredClustersHash uint64
	clustersHash        uint64
	resourceName        string
}

type endpointsWithUccName struct {
	endpoints    envoycache.Resources
	resourceName string
}

func (c clustersWithErrors) ResourceName() string {
	return c.resourceName
}

var _ krt.Equaler[clustersWithErrors] = new(clustersWithErrors)

func (c clustersWithErrors) Equals(k clustersWithErrors) bool {
	return c.clusters.Version == k.clusters.Version && c.erroredClustersHash == k.erroredClustersHash
}

func (c endpointsWithUccName) ResourceName() string {
	return c.resourceName
}

var _ krt.Equaler[endpointsWithUccName] = new(endpointsWithUccName)

func (c endpointsWithUccName) Equals(k endpointsWithUccName) bool {
	return c.endpoints.Version == k.endpoints.Version
}

func snapshotPerClient(
	krtopts krtutil.KrtOptions,
	uccCol krt.Collection[ir.UniqlyConnectedClient],
	mostXdsSnapshots krt.Collection[GatewayXdsResources],
	endpoints PerClientEnvoyEndpoints,
	clusters PerClientEnvoyClusters,
) krt.Collection[XdsSnapWrapper] {

	clusterSnapshot := krt.NewCollection(uccCol, func(kctx krt.HandlerContext, ucc ir.UniqlyConnectedClient) *clustersWithErrors {
		clustersForUcc := clusters.FetchClustersForClient(kctx, ucc)

		logger.Debug("found perclient clusters", "client", ucc.ResourceName(), "clusters", len(clustersForUcc))

		if len(clustersForUcc) == 0 {
			logger.Info("no perclient clusters; defer building snapshot", "client", ucc.ResourceName())
			return nil
		}

		clustersProto := make([]envoycachetypes.ResourceWithTTL, 0, len(clustersForUcc))
		var clustersHash uint64
		var erroredClustersHash uint64
		var erroredClusters []string
		for _, c := range clustersForUcc {
			if c.Error == nil {
				clustersProto = append(clustersProto, envoycachetypes.ResourceWithTTL{Resource: c.Cluster})
				clustersHash ^= c.ClusterVersion
			} else {
				erroredClusters = append(erroredClusters, c.Name)
				erroredClustersHash ^= utils.HashString(c.Name)
			}
		}
		clustersVersion := fmt.Sprintf("%d", clustersHash)

		clusterResources := envoycache.NewResourcesWithTTL(clustersVersion, clustersProto)

		return &clustersWithErrors{
			clusters:        clusterResources,
			erroredClusters: erroredClusters,
			clustersHash:    clustersHash,
			resourceName:    ucc.ResourceName(),
		}
	}, krtopts.ToOptions("ClusterResources")...)

	endpointResources := krt.NewCollection(uccCol, func(kctx krt.HandlerContext, ucc ir.UniqlyConnectedClient) *endpointsWithUccName {

		endpointsForUcc := endpoints.FetchEndpointsForClient(kctx, ucc)
		endpointsProto := make([]envoycachetypes.ResourceWithTTL, 0, len(endpointsForUcc))
		var endpointsHash uint64
		for _, ep := range endpointsForUcc {
			endpointsProto = append(endpointsProto, envoycachetypes.ResourceWithTTL{Resource: ep.Endpoints})
			endpointsHash ^= ep.EndpointsHash
		}

		endpointResources := envoycache.NewResourcesWithTTL(fmt.Sprintf("%d", endpointsHash), endpointsProto)
		return &endpointsWithUccName{
			endpoints:    endpointResources,
			resourceName: ucc.ResourceName(),
		}
	}, krtopts.ToOptions("EndpointResources")...)

	xdsSnapshotsForUcc := krt.NewCollection(uccCol, func(kctx krt.HandlerContext, ucc ir.UniqlyConnectedClient) *XdsSnapWrapper {
		maybeMostlySnap := krt.FetchOne(kctx, mostXdsSnapshots, krt.FilterKey(ucc.Role))
		if maybeMostlySnap == nil {
			logger.Debug("snapshot missing", "proxy_key", ucc.Role)
			return nil
		}
		clustersForUcc := krt.FetchOne(kctx, clusterSnapshot, krt.FilterKey(ucc.ResourceName()))

		// HACK
		// https://github.com/solo-io/gloo/pull/10611/files#diff-060acb7cdd3a287a3aef1dd864aae3e0193da17b6230c382b649ce9dc0eca80b
		// Without this, we will send a "blip" where the DestinationRule
		// or other per-client config is not applied to the clusters
		// by sending the genericSnap clusters on the first pass, then
		// the correct ones.
		// This happens because the event for the new connected client
		// triggers the per-client cluster transformation in parallel
		// with this snapshotPerClient transformation. This Fetch is racing
		// with that computation and will almost always lose.
		// While we're looking for a way to make this ordering predictable
		// to avoid hacks like this, it will do for now.
		if clustersForUcc == nil {
			logger.Info("no perclient clusters; defer building snapshot", "client", ucc.ResourceName())
			return nil
		}

		logger.Debug("found perclient clusters", "client", ucc.ResourceName(), "clusters", len(clustersForUcc.clusters.Items))
		clusterResources := clustersForUcc.clusters
		endpointResources := krt.FetchOne(kctx, endpointResources, krt.FilterKey(ucc.ResourceName()))

		snap := XdsSnapWrapper{}
		if len(maybeMostlySnap.Clusters) > 0 {
			clustersProto := make(map[string]envoycachetypes.ResourceWithTTL, len(maybeMostlySnap.Clusters)+len(clustersForUcc.clusters.Items))
			maps.Copy(clustersProto, clustersForUcc.clusters.Items)
			for _, item := range maybeMostlySnap.Clusters {
				clustersProto[envoycache.GetResourceName(item.Resource)] = item
			}
			clusterResources.Version = fmt.Sprintf("%d", clustersForUcc.clustersHash^maybeMostlySnap.ClustersHash)
			clusterResources.Items = clustersProto
		}

		snap.erroredClusters = clustersForUcc.erroredClusters
		snap.proxyKey = ucc.ResourceName()
		snapshot := &envoycache.Snapshot{}
		snapshot.Resources[envoycachetypes.Cluster] = clusterResources //envoycache.NewResources(version, resource)
		snapshot.Resources[envoycachetypes.Endpoint] = endpointResources.endpoints
		snapshot.Resources[envoycachetypes.Route] = maybeMostlySnap.Routes
		snapshot.Resources[envoycachetypes.Listener] = maybeMostlySnap.Listeners
		// envoycache.NewResources(version, resource)
		snap.snap = snapshot
		logger.Debug("snapshots", "proxy_key", snap.proxyKey,
			"listeners", resourcesStringer(maybeMostlySnap.Listeners).String(),
			"clusters", resourcesStringer(clusterResources).String(),
			"routes", resourcesStringer(maybeMostlySnap.Routes).String(),
			"endpoints", resourcesStringer(endpointResources.endpoints).String(),
		)

		return &snap
	}, krtopts.ToOptions("PerClientXdsSnapshots")...)
	return xdsSnapshotsForUcc
}
