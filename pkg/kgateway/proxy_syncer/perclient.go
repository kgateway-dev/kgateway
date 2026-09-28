package proxy_syncer

import (
	"maps"
	"slices"
	"strconv"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoytcpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
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
	// PerClientEnvoyClusters stores each client's assembled CDS payload (shared bases plus the client's overlays).
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
		// A nil row means its transform has not run yet (the cluster transform
		// also returns nil while no backend exists), so there is nothing to
		// publish.
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

		snap.missingReferenced = missingClusters
		snap.missingEndpointsReferenced = missingEndpointClusters
		snap.synthesizedEndpoints = slices.Sorted(maps.Keys(synthesizedEndpoints))
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
		if snap.deferred() {
			logger.Debug(
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

	// Track connected clients without snapshot rows, including those still
	// waiting for their first snapshot.
	newSnapshotDeferralTracker().register(uccCol, xdsSnapshotsForUcc)
	return xdsSnapshotsForUcc
}

// collectReferencedClusters returns the clusters the given routes and
// listeners route to: the cluster or weighted clusters of every route action
// and TCP proxy, including route configurations inlined in an HTTP connection
// manager. It reads only those fields, so it never unpacks HTTP filter or
// per-route configs.
//
// Clusters that filters call out to (ext_authz, ext_proc, JWKS, OAuth2, rate
// limit, access logs) are not routing targets and are not collected. They are
// ordinary backends in the same per-client CDS, so they can lag like any
// other, but holding every route update of a gateway behind one side service
// would starve it on a single unready dependency; the filter fails, or
// degrades per its failure_mode_allow, until the cluster is ready. Their
// endpoints on the client are still kept while their CLAs are not derived
// (settleSynthesizedEndpoints).
//
// This is computed once per GatewayXdsResources (shared across all connected
// clients for that role) rather than per client.
func collectReferencedClusters(routes, listeners envoycache.Resources) map[string]struct{} {
	referenced := make(map[string]struct{})
	for _, item := range routes.Items {
		if routeConfig, ok := item.Resource.(*envoyroutev3.RouteConfiguration); ok {
			addRouteConfigurationTargets(routeConfig, referenced)
		}
	}
	for _, item := range listeners.Items {
		listener, ok := item.Resource.(*envoylistenerv3.Listener)
		if !ok {
			continue
		}
		for _, filterChain := range listener.GetFilterChains() {
			addFilterChainTargets(filterChain, referenced)
		}
		addFilterChainTargets(listener.GetDefaultFilterChain(), referenced)
	}
	return referenced
}

func addRouteConfigurationTargets(routeConfig *envoyroutev3.RouteConfiguration, referenced map[string]struct{}) {
	for _, virtualHost := range routeConfig.GetVirtualHosts() {
		for _, route := range virtualHost.GetRoutes() {
			action := route.GetRoute()
			addClusterName(action.GetCluster(), referenced)
			for _, cluster := range action.GetWeightedClusters().GetClusters() {
				addClusterName(cluster.GetName(), referenced)
			}
		}
	}
}

// addFilterChainTargets adds the targets of the filter chain's TCP proxies and
// of any route configuration its HTTP connection managers inline; a manager
// that uses RDS contributes through the route configurations instead.
func addFilterChainTargets(filterChain *envoylistenerv3.FilterChain, referenced map[string]struct{}) {
	for _, filter := range filterChain.GetFilters() {
		typedConfig := filter.GetTypedConfig()
		switch {
		case typedConfig.MessageIs((*envoytcpv3.TcpProxy)(nil)):
			var tcpProxy envoytcpv3.TcpProxy
			if err := typedConfig.UnmarshalTo(&tcpProxy); err != nil {
				logger.Debug("skipping unreadable tcp_proxy config during cluster reference scan", "error", err)
				continue
			}
			addClusterName(tcpProxy.GetCluster(), referenced)
			for _, cluster := range tcpProxy.GetWeightedClusters().GetClusters() {
				addClusterName(cluster.GetName(), referenced)
			}
		case typedConfig.MessageIs((*envoyhcmv3.HttpConnectionManager)(nil)):
			var hcm envoyhcmv3.HttpConnectionManager
			if err := typedConfig.UnmarshalTo(&hcm); err != nil {
				logger.Debug("skipping unreadable http_connection_manager config during cluster reference scan", "error", err)
				continue
			}
			if routeConfig := hcm.GetRouteConfig(); routeConfig != nil {
				addRouteConfigurationTargets(routeConfig, referenced)
			}
		}
	}
}

func addClusterName(name string, referenced map[string]struct{}) {
	if name != "" {
		referenced[name] = struct{}{}
	}
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

// filterEndpointResourcesForClusters returns the EDS resources that belong with
// the EDS clusters in the same CDS snapshot. It drops CLAs for clusters that do
// not use EDS or are not in CDS: Envoy requests EDS resources by the names CDS
// gives it, and go-control-plane withholds a whole ADS response that carries
// one the client did not request (#14471). It adds an empty CLA for every EDS
// cluster with no derived one and returns their names, so the snapshot stays
// EDS-consistent; the publish gate decides which of those synthesized CLAs the
// client receives (settleSynthesizedEndpoints), because an empty CLA would
// replace endpoints the client already holds. bootstrapEndpoint names the
// gateway's bootstrap local cluster for clients that subscribed to it; its CLA
// is kept although that cluster is not in dynamic CDS.
//
// The version is the input's when nothing was dropped or added, so leaving a
// filtered state restores it. Otherwise the dropped and synthesized names are
// folded into it: the input version already covers the content of every kept
// CLA, so no CLA is hashed here.
func filterEndpointResourcesForClusters(clusters envoycache.Resources, endpoints envoycache.Resources, bootstrapEndpoint string) (envoycache.Resources, map[string]struct{}) {
	if endpointsMatchClusters(clusters, endpoints, bootstrapEndpoint) {
		return endpoints, nil
	}
	requiredEndpointNames := make(map[string]struct{}, len(clusters.Items)+1)
	for _, item := range clusters.Items {
		if endpointName, requiresEndpointResource := endpointResourceNameForCluster(item); requiresEndpointResource {
			requiredEndpointNames[endpointName] = struct{}{}
		}
	}
	if bootstrapEndpoint != "" {
		requiredEndpointNames[bootstrapEndpoint] = struct{}{}
	}
	covered := make(map[string]struct{}, len(requiredEndpointNames))
	filteredEndpoints := make([]envoycachetypes.ResourceWithTTL, 0, len(endpoints.Items))
	var namesHash uint64
	for _, item := range endpoints.Items {
		cla, ok := item.Resource.(*envoyendpointv3.ClusterLoadAssignment)
		if !ok {
			continue
		}
		if _, required := requiredEndpointNames[cla.GetClusterName()]; !required {
			namesHash ^= utils.HashString("dropped:" + cla.GetClusterName())
			continue
		}
		filteredEndpoints = append(filteredEndpoints, item)
		covered[cla.GetClusterName()] = struct{}{}
	}
	var synthesized map[string]struct{}
	for name := range requiredEndpointNames {
		if _, ok := covered[name]; ok {
			continue
		}
		if synthesized == nil {
			synthesized = make(map[string]struct{})
		}
		filteredEndpoints = append(filteredEndpoints, envoycachetypes.ResourceWithTTL{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: name}})
		synthesized[name] = struct{}{}
		namesHash ^= utils.HashString("synthesized:" + name)
	}
	if len(synthesized) == 0 && len(filteredEndpoints) == len(endpoints.Items) {
		return endpoints, nil
	}
	return envoycache.NewResourcesWithTTL(endpoints.Version+"-"+strconv.FormatUint(namesHash, 10), filteredEndpoints), synthesized
}

// endpointsMatchClusters reports, without allocating, whether endpoints holds
// exactly one CLA for each EDS cluster in clusters and for bootstrapEndpoint,
// and nothing else: the common case, in which there is nothing to filter or
// synthesize. It gives up (returns false) on anything unusual, such as an EDS
// service_name alias, and leaves that to the general path.
func endpointsMatchClusters(clusters envoycache.Resources, endpoints envoycache.Resources, bootstrapEndpoint string) bool {
	required := 0
	for name, item := range clusters.Items {
		endpointName, requiresEndpointResource := endpointResourceNameForCluster(item)
		if !requiresEndpointResource {
			continue
		}
		if endpointName != name || endpointName == bootstrapEndpoint {
			return false
		}
		required++
	}
	if bootstrapEndpoint != "" {
		required++
	}
	if len(endpoints.Items) != required {
		return false
	}
	for name, item := range endpoints.Items {
		if _, ok := item.Resource.(*envoyendpointv3.ClusterLoadAssignment); !ok {
			return false
		}
		if bootstrapEndpoint != "" && name == bootstrapEndpoint {
			continue
		}
		cluster, ok := clusters.Items[name]
		if !ok {
			return false
		}
		if _, requiresEndpointResource := endpointResourceNameForCluster(cluster); !requiresEndpointResource {
			return false
		}
	}
	return true
}
