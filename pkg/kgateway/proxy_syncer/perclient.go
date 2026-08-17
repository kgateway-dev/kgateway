package proxy_syncer

import (
	"fmt"
	"maps"
	"strconv"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// clustersWithErrors is one client's assembled CDS payload plus the clusters that
// failed to translate for that client. Errored clusters are deliberately kept out of
// the payload but tracked by name, because a route pointing at one must return a
// direct response rather than silently falling through to a cluster that isn't there.
//
// The two hashes are the equality keys: publishable clusters are versioned by
// content, errored ones only by name, so an error changing its message does not
// churn a snapshot Envoy will never see.
type clustersWithErrors struct {
	// +noKrtEquals
	clusters envoycache.Resources
	// +noKrtEquals
	erroredClusters     []string
	erroredClustersHash uint64
	clustersHash        uint64
	resourceName        string
}

// endpointsWithUccName is one client's assembled EDS payload, keyed by client.
// envoycache.Resources already carries a version, which is the equality key.
type endpointsWithUccName struct {
	endpoints    envoycache.Resources
	resourceName string
}

func (c clustersWithErrors) ResourceName() string {
	return c.resourceName
}

var _ krt.Equaler[clustersWithErrors] = new(clustersWithErrors)

func (c clustersWithErrors) Equals(k clustersWithErrors) bool {
	return c.clustersHash == k.clustersHash && c.erroredClustersHash == k.erroredClustersHash && c.resourceName == k.resourceName
}

func (c endpointsWithUccName) ResourceName() string {
	return c.resourceName
}

var _ krt.Equaler[endpointsWithUccName] = new(endpointsWithUccName)

func (c endpointsWithUccName) Equals(k endpointsWithUccName) bool {
	return c.endpoints.Version == k.endpoints.Version && c.resourceName == k.resourceName
}

// snapshotPerClient assembles the complete xDS snapshot each connected client should
// receive, joining the per-Gateway listener/route translation with that client's own
// clusters and endpoints. It is the last stage of translation: everything downstream
// just ships what this produces.
//
// It publishes only complete snapshots. When a client's per-client inputs have not
// caught up with the event being processed, it returns nil rather than a partial
// snapshot; the subscriber treats that as "keep serving what Envoy already has".
// Retaining the last coherent config is always preferable to publishing an
// incoherent one, which Envoy would apply — dropping routes or endpoints that are
// still valid.
//
// extraEndpointCollections are additional per-client endpoint sources merged into the
// same EDS payload, currently the gateway's own local cluster.
func snapshotPerClient(
	krtopts krtutil.KrtOptions,
	uccCol krt.Collection[ir.UniquelyConnectedClient],
	mostXdsSnapshots krt.Collection[GatewayXdsResources],
	endpoints PerClientEnvoyEndpoints,
	clusters PerClientEnvoyClusters,
	extraEndpointCollections ...PerClientEnvoyEndpoints,
) krt.Collection[XdsSnapWrapper] {
	clusterSnapshot := krt.NewCollection(uccCol, func(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient) *clustersWithErrors {
		clustersForUcc := clusters.FetchClustersForClient(kctx, ucc)
		if len(clustersForUcc) == 0 {
			logger.Info("no perclient clusters; defer building snapshot", "client", ucc.ResourceName())
			return nil
		}
		logger.Debug("found perclient clusters", "client", ucc.ResourceName(), "clusters", len(clustersForUcc))

		clustersProto := make([]envoycachetypes.ResourceWithTTL, 0, len(clustersForUcc))
		var (
			clustersHash        uint64
			erroredClustersHash uint64
			erroredClusters     []string
		)
		for _, c := range clustersForUcc {
			if c.Error != nil {
				erroredClusters = append(erroredClusters, c.Name)
				// For errored clusters, we don't want to include the cluster version
				// in the hash. The cluster version is the hash of the proto. because this cluster
				// won't be sent to envoy anyway, there's no point trigger updates if it changes from
				// one error state to a different error state.
				erroredClustersHash ^= utils.HashString(c.Name)
				continue
			}
			// ResourceWithTTL is the only exit for the shared proto; it runs
			// the mutation tripwire when armed. See package sharedproto.
			clustersProto = append(clustersProto, c.Cluster.ResourceWithTTL())
			clustersHash ^= c.ClusterVersion
		}
		clustersVersion := strconv.FormatUint(clustersHash, 10)

		clusterResources := envoycache.NewResourcesWithTTL(clustersVersion, clustersProto)

		return &clustersWithErrors{
			clusters:            clusterResources,
			erroredClusters:     erroredClusters,
			clustersHash:        clustersHash,
			erroredClustersHash: erroredClustersHash,
			resourceName:        ucc.ResourceName(),
		}
	}, krtopts.ToOptions("ClusterResources")...)

	endpointResources := krt.NewCollection(uccCol, func(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient) *endpointsWithUccName {
		endpointsForUcc := endpoints.FetchEndpointsForClient(kctx, ucc)
		for _, extraEndpoints := range extraEndpointCollections {
			endpointsForUcc = append(endpointsForUcc, extraEndpoints.FetchEndpointsForClient(kctx, ucc)...)
		}
		endpointsProto := make([]envoycachetypes.ResourceWithTTL, 0, len(endpointsForUcc))
		var endpointsHash uint64
		for _, ep := range endpointsForUcc {
			// ResourceWithTTL is the only exit for the interned CLA; it runs
			// the mutation tripwire when armed. See package sharedproto.
			endpointsProto = append(endpointsProto, ep.Endpoints.ResourceWithTTL())
			endpointsHash ^= ep.EndpointsHash
		}

		endpointResources := envoycache.NewResourcesWithTTL(strconv.FormatUint(endpointsHash, 10), endpointsProto)
		return &endpointsWithUccName{
			endpoints:    endpointResources,
			resourceName: ucc.ResourceName(),
		}
	}, krtopts.ToOptions("EndpointResources")...)

	xdsSnapshotsForUcc := krt.NewCollection(uccCol, func(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient) *XdsSnapWrapper {
		defer (collectXDSTransformMetrics(ucc.ResourceName()))(nil)

		listenerRouteSnapshot := krt.FetchOne(kctx, mostXdsSnapshots, krt.FilterKey(ucc.Role))
		if listenerRouteSnapshot == nil {
			logger.Debug("snapshot missing", "proxy_key", ucc.Role)
			return nil
		}
		clustersForUcc := krt.FetchOne(kctx, clusterSnapshot, krt.FilterKey(ucc.ResourceName()))
		clientEndpointResources := krt.FetchOne(kctx, endpointResources, krt.FilterKey(ucc.ResourceName()))

		// HACK
		// https://github.com/solo-io/gloo/pull/10611/files#diff-060acb7cdd3a287a3aef1dd864aae3e0193da17b6230c382b649ce9dc0eca80b
		// This handler can fire before the per-client collections — driven
		// by the same upstream events — have re-run, so FetchOne may briefly
		// return nil even though results are imminent. Defer instead of
		// publishing: returning nil surfaces as a Delete event in
		// proxy_syncer.go's xDS subscriber, whose Delete branch is
		// intentionally a no-op, so the xDS snapshot cache retains the
		// last-published snapshot and Envoy keeps serving its previous,
		// coherent config until a new snapshot overwrites it.
		//
		// Between the first per-client cluster row landing and the last,
		// published snapshots can still be partial — the reconnect-time race
		// #13868 tried to close with whole-snapshot readiness gates. Those
		// gates could stay unsatisfied indefinitely (stranding warm clients
		// on stale endpoints and starving new pods into crashloops, #14184)
		// and were reverted. The first-connect delay in
		// pkg/krtcollections/uniqueclients.go keeps a client's first watch
		// from observing that convergence window.
		if clustersForUcc == nil || clientEndpointResources == nil {
			logger.Info("per-client inputs not ready; deferring snapshot", "client", ucc.ResourceName())
			return nil
		}

		logger.Debug("found perclient clusters", "client", ucc.ResourceName(), "clusters", len(clustersForUcc.clusters.Items))
		clusterResources := clustersForUcc.clusters

		snap := XdsSnapWrapper{}
		if len(listenerRouteSnapshot.Clusters) > 0 {
			clustersProto := make(map[string]envoycachetypes.ResourceWithTTL, len(listenerRouteSnapshot.Clusters)+len(clustersForUcc.clusters.Items))
			maps.Copy(clustersProto, clustersForUcc.clusters.Items)
			for _, item := range listenerRouteSnapshot.Clusters {
				clustersProto[envoycache.GetResourceName(item.Resource)] = item
			}
			clusterResources.Version = strconv.FormatUint(clustersForUcc.clustersHash^listenerRouteSnapshot.ClustersHash, 10)
			clusterResources.Items = clustersProto
		}
		// Exclude CLAs for STATIC clusters so ADS snapshot only contains resources Envoy will request.
		endpointRes := filterEndpointResourcesForStaticClusters(clusterResources, clientEndpointResources.endpoints)
		// Backend translation errors remove the corresponding cluster from CDS above. Remove
		// its CLA as well: after Envoy observes the CDS removal it stops naming that resource
		// in EDS requests, and go-control-plane otherwise withholds the entire named EDS
		// response, including updates for healthy clusters.
		//
		// Keep this filtering explicitly error-scoped. Some endpoint resources, such as the
		// bootstrap-defined local cluster, intentionally have no cluster in this CDS snapshot.
		endpointRes = filterEndpointResourcesForErroredClusters(endpointRes, clustersForUcc.erroredClusters)

		snap.erroredClusters = clustersForUcc.erroredClusters
		snap.proxyKey = ucc.ResourceName()
		snapshot := &envoycache.Snapshot{}
		snapshot.Resources[envoycachetypes.Cluster] = clusterResources
		snapshot.Resources[envoycachetypes.Endpoint] = endpointRes
		snapshot.Resources[envoycachetypes.Route] = listenerRouteSnapshot.Routes
		snapshot.Resources[envoycachetypes.Listener] = listenerRouteSnapshot.Listeners
		snapshot.Resources[envoycachetypes.Secret] = listenerRouteSnapshot.Secrets
		// envoycache.NewResources(version, resource)
		snap.snap = snapshot
		logger.Debug("snapshots", "proxy_key", snap.proxyKey,
			"listeners", resourcesStringer(listenerRouteSnapshot.Listeners).String(),
			"clusters", resourcesStringer(clusterResources).String(),
			"routes", resourcesStringer(listenerRouteSnapshot.Routes).String(),
			"endpoints", resourcesStringer(endpointRes).String(),
			"secrets", resourcesStringer(listenerRouteSnapshot.Secrets).String(),
		)

		return &snap
	}, krtopts.ToOptions("PerClientXdsSnapshots")...)

	metrics.RegisterEvents(xdsSnapshotsForUcc, func(o krt.Event[XdsSnapWrapper]) {
		cd := getDetailsFromXDSClientResourceName(o.Latest().ResourceName())

		switch o.Event {
		case controllers.EventDelete:
			snapshotResources.Set(0, snapshotResourcesMetricLabels{
				Gateway:   cd.Gateway,
				Namespace: cd.Namespace,
				Resource:  "Cluster",
			}.toMetricsLabels()...)

			snapshotResources.Set(0, snapshotResourcesMetricLabels{
				Gateway:   cd.Gateway,
				Namespace: cd.Namespace,
				Resource:  "Endpoint",
			}.toMetricsLabels()...)

			snapshotResources.Set(0, snapshotResourcesMetricLabels{
				Gateway:   cd.Gateway,
				Namespace: cd.Namespace,
				Resource:  "Route",
			}.toMetricsLabels()...)

			snapshotResources.Set(0, snapshotResourcesMetricLabels{
				Gateway:   cd.Gateway,
				Namespace: cd.Namespace,
				Resource:  "Listener",
			}.toMetricsLabels()...)

			snapshotResources.Set(0, snapshotResourcesMetricLabels{
				Gateway:   cd.Gateway,
				Namespace: cd.Namespace,
				Resource:  "Secret",
			}.toMetricsLabels()...)

		case controllers.EventAdd, controllers.EventUpdate:
			snapshotResources.Set(float64(len(o.Latest().snap.Resources[envoycachetypes.Cluster].Items)),
				snapshotResourcesMetricLabels{
					Gateway:   cd.Gateway,
					Namespace: cd.Namespace,
					Resource:  "Cluster",
				}.toMetricsLabels()...)

			snapshotResources.Set(float64(len(o.Latest().snap.Resources[envoycachetypes.Endpoint].Items)),
				snapshotResourcesMetricLabels{
					Gateway:   cd.Gateway,
					Namespace: cd.Namespace,
					Resource:  "Endpoint",
				}.toMetricsLabels()...)

			snapshotResources.Set(float64(len(o.Latest().snap.Resources[envoycachetypes.Route].Items)),
				snapshotResourcesMetricLabels{
					Gateway:   cd.Gateway,
					Namespace: cd.Namespace,
					Resource:  "Route",
				}.toMetricsLabels()...)

			snapshotResources.Set(float64(len(o.Latest().snap.Resources[envoycachetypes.Listener].Items)),
				snapshotResourcesMetricLabels{
					Gateway:   cd.Gateway,
					Namespace: cd.Namespace,
					Resource:  "Listener",
				}.toMetricsLabels()...)

			snapshotResources.Set(float64(len(o.Latest().snap.Resources[envoycachetypes.Secret].Items)),
				snapshotResourcesMetricLabels{
					Gateway:   cd.Gateway,
					Namespace: cd.Namespace,
					Resource:  "Secret",
				}.toMetricsLabels()...)
		}
	})

	return xdsSnapshotsForUcc
}

// filterEndpointResourcesForStaticClusters returns endpoint resources excluding CLAs for clusters
// that are STATIC (inline endpoints). Envoy does not request EDS for those; including them in the
// snapshot triggers the ADS cache "not listed" warning when responding to EDS requests.
func filterEndpointResourcesForStaticClusters(clusters envoycache.Resources, endpoints envoycache.Resources) envoycache.Resources {
	staticClusterNames := make(map[string]struct{})
	for _, item := range clusters.Items {
		if c, ok := item.Resource.(*envoyclusterv3.Cluster); ok && c.GetType() == envoyclusterv3.Cluster_STATIC {
			staticClusterNames[c.GetName()] = struct{}{}
		}
	}
	if len(staticClusterNames) == 0 {
		return endpoints
	}
	filteredEndpoints := make([]envoycachetypes.ResourceWithTTL, 0, len(endpoints.Items))
	for _, item := range endpoints.Items {
		cla, ok := item.Resource.(*envoyendpointv3.ClusterLoadAssignment)
		if !ok {
			continue
		}
		if _, isStatic := staticClusterNames[cla.GetClusterName()]; isStatic {
			continue
		}
		filteredEndpoints = append(filteredEndpoints, item)
	}
	if len(filteredEndpoints) == len(endpoints.Items) {
		return endpoints
	}
	return envoycache.NewResourcesWithTTL(endpoints.Version, filteredEndpoints)
}

// filterEndpointResourcesForErroredClusters removes CLAs for clusters omitted from CDS
// because backend translation failed.
//
// The version handling is load-bearing. Filtering must change the EDS version (the
// "-errors-" suffix), and clearing the error must restore the original version, because
// go-control-plane's SOTW push path responds only when the snapshot version differs from
// the watch's last-acked version. If the version stayed the same across an error->recovery
// flap, a recovered cluster whose endpoints never changed would get no EDS push, and the
// request path can also stay silent (the client's returned-resources state still lists the
// CLA if Envoy never sent a narrowed EDS request between the two CDS updates) - leaving the
// cluster warming until some unrelated endpoint change bumps the version. The original
// endpoint version remains part of the filtered version so endpoint changes for healthy
// clusters keep triggering EDS updates while another cluster is errored.
func filterEndpointResourcesForErroredClusters(endpoints envoycache.Resources, erroredClusters []string) envoycache.Resources {
	if len(erroredClusters) == 0 {
		return endpoints
	}

	erroredClusterNames := make(map[string]struct{}, len(erroredClusters))
	var erroredClustersHash uint64
	for _, name := range erroredClusters {
		erroredClusterNames[name] = struct{}{}
		erroredClustersHash ^= utils.HashString(name)
	}

	filteredEndpoints := make([]envoycachetypes.ResourceWithTTL, 0, len(endpoints.Items))
	for _, item := range endpoints.Items {
		cla, ok := item.Resource.(*envoyendpointv3.ClusterLoadAssignment)
		if ok {
			if _, errored := erroredClusterNames[cla.GetClusterName()]; errored {
				continue
			}
		}
		filteredEndpoints = append(filteredEndpoints, item)
	}
	if len(filteredEndpoints) == len(endpoints.Items) {
		return endpoints
	}

	version := fmt.Sprintf("%s-errors-%d", endpoints.Version, erroredClustersHash)
	return envoycache.NewResourcesWithTTL(version, filteredEndpoints)
}
