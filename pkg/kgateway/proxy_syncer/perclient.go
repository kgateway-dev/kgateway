package proxy_syncer

import (
	"maps"
	"slices"
	"strconv"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
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

type endpointsWithUccName struct {
	endpoints    envoycache.Resources
	resourceName string
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
	uccCol krt.Collection[ir.UniquelyConnectedClient],
	mostXdsSnapshots krt.Collection[GatewayXdsResources],
	endpoints PerClientEnvoyEndpoints,
	clusters PerClientEnvoyClusters,
	extraEndpointCollections ...PerClientEnvoyEndpoints,
) krt.Collection[XdsSnapWrapper] {
	// The stack assembles one complete CDS row per client from shared bases.
	clusterSnapshot := clusters.perClient

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

		// Annotate missing routing targets and underived CLAs for the bounded
		// publication gate. A derived empty CLA is backend truth, not a gap.
		// Keep the complete client-keyed CDS row; a nil row means its transform
		// has not run yet, whereas an empty row is a valid zero-backend result.

		if clustersForUcc == nil || clientEndpointResources == nil {
			logger.Debug("per-client inputs not ready; deferring snapshot", "client", ucc.ResourceName())
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
		missingClusters := findMissingReferencedClusters(
			listenerRouteSnapshot.ReferencedClusters,
			clusterResources.Items,
			clustersForUcc.erroredClusters,
		)
		// Keep EDS resources aligned with the EDS clusters in the same CDS snapshot.
		// Envoy's named EDS requests are induced by CDS; stale CLAs for clusters no
		// longer present in CDS can make go-control-plane suppress ADS responses.
		bootstrapEndpoint := ""
		if ucc.KnowsLocalCluster {
			bootstrapEndpoint, _, _ = ucc.LocalClusterInfo()
		}
		endpointRes, synthesizedEndpoints := filterEndpointResourcesForClusters(clusterResources, clientEndpointResources.endpoints, bootstrapEndpoint)
		// Post-synthesis every EDS cluster has a CLA; the synthesized set
		// identifies exactly the referenced clusters whose CLA was not
		// derived (a derived-but-empty CLA is truth, not a gap).
		missingEndpointClusters := findMissingReferencedEndpointResources(
			listenerRouteSnapshot.ReferencedClusters,
			clusterResources.Items,
			synthesizedEndpoints,
			clustersForUcc.erroredClusters,
		)

		snap.deferred = len(missingClusters) > 0 || len(missingEndpointClusters) > 0
		snap.missingReferenced = missingClusters
		snap.missingEndpointsReferenced = missingEndpointClusters
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
		if snap.deferred {
			logger.Info(
				"snapshot has unready referenced clusters; syncXds will resolve per cluster",
				"client", ucc.ResourceName(),
				"missing_clusters", missingClusters,
				"missing_endpoint_clusters", missingEndpointClusters,
			)
		}
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

	newSnapshotDeferralTracker().register(uccCol, xdsSnapshotsForUcc)
	return xdsSnapshotsForUcc
}

// collectReferencedClusters returns the set of cluster names referenced as
// dataplane routing targets (RouteAction and TcpProxy cluster / weighted-
// cluster specifiers) by the given routes and listeners. It walks typed_config
// extensions via protoreflect so it stays correct as Envoy adds new filter
// types that embed dataplane-target clusters.
//
// Scope is intentionally narrowed to dataplane targets. Ancillary cluster
// references (access-log GrpcService, JWT jwks HttpUri, ext_authz cluster,
// ratelimit cluster, etc.) are deliberately ignored because:
//
//  1. The plugin that emits the filter is responsible for also emitting the
//     ancillary cluster in the same per-gateway snapshot's ExtraClusters,
//     so there is no reconnect race between listener and cluster — they
//     arrive coherent or not at all.
//  2. If a plugin emits an ancillary reference without declaring the
//     cluster, that is a plugin bug. Gating on it would starve the entire
//     gateway forever; publishing and letting the filter fail (or degrade
//     per its failure_mode_allow) surfaces the bug without blocking valid
//     traffic.
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
	erroredClusterSet := stringSet(erroredClusters)

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
	slices.Sort(missingClusters)

	return missingClusters
}

// findMissingReferencedEndpointResources reports the referenced EDS clusters
// whose ClusterLoadAssignment was not derived by the per-client endpoints
// collection — i.e. their CLA in the snapshot is a synthesized empty
// placeholder (see filterEndpointResourcesForClusters). Whether such a
// backend has endpoints is UNKNOWN — per-client derivation lag for kube
// Services (whose endpoints transform emits a row for every resolvable
// port, even sliceless ones like ExternalName), or a plugin that
// contributed an EDS cluster without an endpoints row — which is what
// warrants deferral. PRESENCE, not contents, is the test: a derived CLA
// with zero usable endpoints is the backend's known truth (scale-to-zero
// and crashlooping backends are steady states, not races — #14352) and must
// not defer the snapshot; this matches the existence semantics of the
// whole-snapshot gate this replaced, the behavior production configs are
// built against.
func findMissingReferencedEndpointResources(
	referencedClusters map[string]struct{},
	clusters map[string]envoycachetypes.ResourceWithTTL,
	synthesizedEndpoints map[string]struct{},
	erroredClusters []string,
) []string {
	erroredClusterSet := stringSet(erroredClusters)

	missingEndpointClusters := make([]string, 0, len(referencedClusters))
	for name := range referencedClusters {
		if _, ok := erroredClusterSet[name]; ok {
			continue
		}
		if name == wellknown.BlackholeClusterName {
			continue
		}

		clusterResource, ok := clusters[name]
		if !ok {
			continue
		}
		endpointResourceName, requiresEndpointResource := endpointResourceNameForCluster(clusterResource)
		if !requiresEndpointResource {
			continue
		}
		if _, synthesized := synthesizedEndpoints[endpointResourceName]; !synthesized {
			continue
		}
		missingEndpointClusters = append(missingEndpointClusters, name)
	}
	slices.Sort(missingEndpointClusters)

	return missingEndpointClusters
}

func endpointResourceNameForCluster(resource envoycachetypes.ResourceWithTTL) (string, bool) {
	cluster, ok := resource.Resource.(*envoyclusterv3.Cluster)
	if !ok {
		return "", false
	}
	clusterType, ok := cluster.GetClusterDiscoveryType().(*envoyclusterv3.Cluster_Type)
	if !ok || clusterType.Type != envoyclusterv3.Cluster_EDS {
		return "", false
	}
	if edsServiceName := cluster.GetEdsClusterConfig().GetServiceName(); edsServiceName != "" {
		return edsServiceName, true
	}
	return cluster.GetName(), true
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
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

// filterEndpointResourcesForClusters returns the EDS resource set that exactly
// matches the EDS clusters in the same CDS snapshot: it drops CLAs for STATIC
// clusters and for EDS clusters no longer in CDS (Envoy requests EDS resources
// from CDS, so a stale CLA can make the ADS cache refuse named EDS responses),
// and it synthesizes an empty ClusterLoadAssignment for any EDS cluster that
// has no derived CLA. The result keeps the published snapshot EDS-consistent —
// every EDS cluster has exactly one CLA, including explicitly supplied
// bootstrap cluster names that intentionally do not appear in dynamic CDS — rather than
// relying on the cache tolerating a dangling EDS cluster, and it lets Envoy
// treat such a cluster as active-with-no-hosts immediately instead of stalling
// its warming on an absent EDS resource until the initial-fetch timeout.
//
// The second return value is the set of endpoint resource names that were
// synthesized. Referenced clusters backed by a synthesized CLA mark the
// wrapper deferred (classifyReferencedEndpointResources) so a route flip
// does not land on a cluster whose endpoints simply have not been derived
// yet; synthesized empties still reach Envoy for clusters no route targets,
// and on the bounded publish paths (publishGate), where active-with-no-hosts
// is the correct interim state.
func filterEndpointResourcesForClusters(clusters envoycache.Resources, endpoints envoycache.Resources, bootstrapEndpoints ...string) (envoycache.Resources, map[string]struct{}) {
	requiredEndpointNames := make(map[string]struct{})
	for _, item := range clusters.Items {
		if endpointName, requiresEndpointResource := endpointResourceNameForCluster(item); requiresEndpointResource {
			requiredEndpointNames[endpointName] = struct{}{}
		}
	}
	for _, name := range bootstrapEndpoints {
		if name != "" {
			requiredEndpointNames[name] = struct{}{}
		}
	}
	covered := make(map[string]struct{}, len(requiredEndpointNames))
	filteredEndpoints := make([]envoycachetypes.ResourceWithTTL, 0, len(endpoints.Items))
	var resourcesHash uint64
	for _, item := range endpoints.Items {
		cla, ok := item.Resource.(*envoyendpointv3.ClusterLoadAssignment)
		if !ok {
			continue
		}
		if _, required := requiredEndpointNames[cla.GetClusterName()]; !required {
			continue
		}
		filteredEndpoints = append(filteredEndpoints, item)
		covered[cla.GetClusterName()] = struct{}{}
		resourcesHash ^= utils.HashProto(cla)
	}
	// Synthesize empty assignments for EDS clusters that have no derived CLA
	// so the published snapshot stays EDS-consistent.
	synthesized := make(map[string]struct{})
	for name := range requiredEndpointNames {
		if _, ok := covered[name]; ok {
			continue
		}
		empty := &envoyendpointv3.ClusterLoadAssignment{ClusterName: name}
		filteredEndpoints = append(filteredEndpoints, envoycachetypes.ResourceWithTTL{Resource: empty})
		resourcesHash ^= utils.HashProto(empty)
		synthesized[name] = struct{}{}
	}
	if len(synthesized) == 0 && len(filteredEndpoints) == len(endpoints.Items) {
		return endpoints, nil
	}
	return envoycache.NewResourcesWithTTL(strconv.FormatUint(resourcesHash, 10), filteredEndpoints), synthesized
}
