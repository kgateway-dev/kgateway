package proxy_syncer

import (
	"fmt"
	"maps"
	"sort"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoytcpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

type clustersWithErrors struct {
	// +noKrtEquals
	clusters envoycache.Resources
	// +noKrtEquals
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
	return c.clustersHash == k.clustersHash && c.erroredClustersHash == k.erroredClustersHash && c.resourceName == k.resourceName
}

func (c endpointsWithUccName) ResourceName() string {
	return c.resourceName
}

var _ krt.Equaler[endpointsWithUccName] = new(endpointsWithUccName)

func (c endpointsWithUccName) Equals(k endpointsWithUccName) bool {
	return c.endpoints.Version == k.endpoints.Version && c.resourceName == k.resourceName
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
			clustersProto = append(clustersProto, envoycachetypes.ResourceWithTTL{Resource: c.Cluster})
			clustersHash ^= c.ClusterVersion
		}
		clustersVersion := fmt.Sprintf("%d", clustersHash)

		clusterResources := envoycache.NewResourcesWithTTL(clustersVersion, clustersProto)

		return &clustersWithErrors{
			clusters:            clusterResources,
			erroredClusters:     erroredClusters,
			clustersHash:        clustersHash,
			erroredClustersHash: erroredClustersHash,
			resourceName:        ucc.ResourceName(),
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
		defer (collectXDSTransformMetrics(ucc.ResourceName()))(nil)

		listenerRouteSnapshot := krt.FetchOne(kctx, mostXdsSnapshots, krt.FilterKey(ucc.Role))
		if listenerRouteSnapshot == nil {
			logger.Debug("snapshot missing", "proxy_key", ucc.Role)
			return nil
		}
		clustersForUcc := krt.FetchOne(kctx, clusterSnapshot, krt.FilterKey(ucc.ResourceName()))
		clientEndpointResources := krt.FetchOne(kctx, endpointResources, krt.FilterKey(ucc.ResourceName()))

		// Defer publishing until per-client inputs are coherent.
		//
		// This handler can fire before the per-client cluster and endpoint
		// collections — driven by the same upstream events — have re-derived
		// their outputs, so FetchOne may return nil even though results are
		// imminent. Publishing anyway would emit a snapshot whose routes and
		// listeners reference clusters not yet in CDS, causing Envoy to return
		// 500/NC transiently.
		//
		// A second guard below (findMissingReferencedClusters) further delays
		// publishing until every cluster referenced by routes or listeners is
		// either present or explicitly errored.
		//
		// Historical context: https://github.com/solo-io/gloo/pull/10611.
		if clustersForUcc == nil || clientEndpointResources == nil {
			logger.Info("no perclient clusters; defer building snapshot", "client", ucc.ResourceName())
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
			clusterResources.Version = fmt.Sprintf("%d", clustersForUcc.clustersHash^listenerRouteSnapshot.ClustersHash)
			clusterResources.Items = clustersProto
		}
		if missingClusters := findMissingReferencedClusters(
			listenerRouteSnapshot.ReferencedClusters,
			clusterResources.Items,
			clustersForUcc.erroredClusters,
		); len(missingClusters) > 0 {
			logger.Info(
				"defer building snapshot until all referenced clusters are ready",
				"client", ucc.ResourceName(),
				"missing_clusters", missingClusters,
			)
			return nil
		}

		snap.erroredClusters = clustersForUcc.erroredClusters
		snap.proxyKey = ucc.ResourceName()
		snapshot := &envoycache.Snapshot{}
		snapshot.Resources[envoycachetypes.Cluster] = clusterResources
		// Exclude CLAs for STATIC clusters so ADS snapshot only contains resources Envoy will request.
		endpointRes := filterEndpointResourcesForStaticClusters(clusterResources, clientEndpointResources.endpoints)
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

// collectReferencedClusters returns the set of cluster names referenced by the
// given routes and listeners. It walks typed_config extensions via protoreflect
// so it stays correct as Envoy adds new filter types that embed cluster names.
//
// This is computed once per GatewayXdsResources (shared across all connected
// clients for that role) rather than per client — the proto walk and Any
// unmarshalling are non-trivial on large LDS/RDS.
func collectReferencedClusters(routes, listeners envoycache.Resources) map[string]struct{} {
	referenced := make(map[string]struct{})
	collectResourceClusterReferences(routes, referenced)
	collectResourceClusterReferences(listeners, referenced)
	return referenced
}

func findMissingReferencedClusters(
	referencedClusters map[string]struct{},
	clusters map[string]envoycachetypes.ResourceWithTTL,
	erroredClusters []string,
) []string {
	erroredClusterSet := make(map[string]struct{}, len(erroredClusters))
	for _, name := range erroredClusters {
		erroredClusterSet[name] = struct{}{}
	}

	missingClusters := make([]string, 0, len(referencedClusters))
	for name := range referencedClusters {
		if _, ok := clusters[name]; ok {
			continue
		}
		if _, ok := erroredClusterSet[name]; ok {
			continue
		}
		if name == wellknown.BlackholeClusterName {
			continue
		}
		missingClusters = append(missingClusters, name)
	}
	sort.Strings(missingClusters)

	return missingClusters
}

func collectResourceClusterReferences(resources envoycache.Resources, referencedClusters map[string]struct{}) {
	for _, item := range resources.Items {
		if item.Resource == nil {
			continue
		}
		collectProtoClusterReferences(item.Resource, referencedClusters)
	}
}

func collectProtoClusterReferences(msg proto.Message, referencedClusters map[string]struct{}) {
	if msg == nil {
		return
	}

	switch typedMsg := msg.(type) {
	case *envoyroutev3.RouteAction:
		switch clusterSpecifier := typedMsg.GetClusterSpecifier().(type) {
		case *envoyroutev3.RouteAction_Cluster:
			if clusterSpecifier.Cluster != "" {
				referencedClusters[clusterSpecifier.Cluster] = struct{}{}
			}
		case *envoyroutev3.RouteAction_WeightedClusters:
			if clusterSpecifier.WeightedClusters == nil {
				break
			}
			for _, cluster := range clusterSpecifier.WeightedClusters.GetClusters() {
				if cluster.GetName() != "" {
					referencedClusters[cluster.GetName()] = struct{}{}
				}
			}
		}
	case *envoytcpv3.TcpProxy:
		switch clusterSpecifier := typedMsg.GetClusterSpecifier().(type) {
		case *envoytcpv3.TcpProxy_Cluster:
			if clusterSpecifier.Cluster != "" {
				referencedClusters[clusterSpecifier.Cluster] = struct{}{}
			}
		case *envoytcpv3.TcpProxy_WeightedClusters:
			if clusterSpecifier.WeightedClusters == nil {
				break
			}
			for _, cluster := range clusterSpecifier.WeightedClusters.GetClusters() {
				if cluster.GetName() != "" {
					referencedClusters[cluster.GetName()] = struct{}{}
				}
			}
		}
	case *envoycorev3.GrpcService_EnvoyGrpc:
		if typedMsg.GetClusterName() != "" {
			referencedClusters[typedMsg.GetClusterName()] = struct{}{}
		}
	case *envoycorev3.HttpUri:
		if typedMsg.GetCluster() != "" {
			referencedClusters[typedMsg.GetCluster()] = struct{}{}
		}
	}

	collectNestedProtoClusterReferences(msg.ProtoReflect(), referencedClusters)
}

func collectNestedProtoClusterReferences(
	msg protoreflect.Message,
	referencedClusters map[string]struct{},
) {
	if !msg.IsValid() {
		return
	}

	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsList() && fd.Message() != nil:
			list := v.List()
			for i := 0; i < list.Len(); i++ {
				collectProtoClusterReferencesFromValue(list.Get(i), referencedClusters)
			}
		case fd.IsMap() && fd.MapValue().Message() != nil:
			m := v.Map()
			m.Range(func(_ protoreflect.MapKey, value protoreflect.Value) bool {
				collectProtoClusterReferencesFromValue(value, referencedClusters)
				return true
			})
		case !fd.IsList() && !fd.IsMap() && fd.Message() != nil:
			collectProtoClusterReferencesFromValue(v, referencedClusters)
		}
		return true
	})
}

func collectProtoClusterReferencesFromValue(v protoreflect.Value, referencedClusters map[string]struct{}) {
	msg := v.Message()
	if !msg.IsValid() {
		return
	}

	if anyMsg, ok := msg.Interface().(*anypb.Any); ok {
		nestedMsg, err := anyMsg.UnmarshalNew()
		if err != nil {
			// Typed extensions whose Go types aren't linked into this binary will fail here;
			// that's expected, but log at debug so genuinely malformed configs are diagnosable.
			logger.Debug("skipping typed_config during cluster reference scan", "type_url", anyMsg.GetTypeUrl(), "error", err)
			return
		}
		collectProtoClusterReferences(nestedMsg, referencedClusters)
		return
	}

	collectProtoClusterReferences(msg.Interface(), referencedClusters)
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
