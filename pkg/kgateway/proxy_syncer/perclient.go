package proxy_syncer

import (
	"fmt"
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
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

type endpointsWithUccName struct {
	endpoints envoycache.Resources
	// rowHashes maps assignment names to built-row contributions (eds_set_version.go).
	// +noKrtEquals folded into endpoints.Version
	rowHashes    map[string]uint64
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
	scoping clusterScoping,
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
		rowHashes := make(map[string]uint64, len(endpointsForUcc))
		var endpointsHash uint64
		for _, ep := range endpointsForUcc {
			rowHashes[ep.Endpoints.BorrowForRead().GetClusterName()] = ep.EndpointsHash
			// ResourceWithTTL is the only exit for the interned CLA; it runs
			// the mutation tripwire when armed. See package sharedproto.
			endpointsProto = append(endpointsProto, ep.Endpoints.ResourceWithTTL())
			endpointsHash ^= ep.EndpointsHash
		}

		endpointResources := envoycache.NewResourcesWithTTL(strconv.FormatUint(endpointsHash, 10), endpointsProto)
		return &endpointsWithUccName{
			endpoints:    endpointResources,
			rowHashes:    rowHashes,
			resourceName: ucc.ResourceName(),
		}
	}, krtopts.ToOptions("EndpointResources")...)

	// The snapshot transform depends on change signals rather than on the three
	// payloads, so its queue, which falls behind during bursts, pins none of
	// them (see changeSignals).
	gatewaySignals := changeSignals(krtopts, "GatewayXdsResourceSignals", mostXdsSnapshots)
	clusterSignals := changeSignals(krtopts, "ClusterResourceSignals", clusterSnapshot)
	endpointSignals := changeSignals(krtopts, "EndpointResourceSignals", endpointResources)

	xdsSnapshotsForUcc := krt.NewCollection(uccCol, func(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient) *XdsSnapWrapper {
		defer collectXDSTransformMetrics(ucc.ResourceName())(nil)

		listenerRouteSnapshot := fetchSignaled(kctx, gatewaySignals, mostXdsSnapshots, ucc.Role)
		if listenerRouteSnapshot == nil {
			logger.Debug("snapshot missing", "proxy_key", ucc.Role)
			return nil
		}
		clustersForUcc := fetchSignaled(kctx, clusterSignals, clusterSnapshot, ucc.ResourceName())
		clientEndpointResources := fetchSignaled(kctx, endpointSignals, endpointResources, ucc.ResourceName())

		// A nil row has not been computed yet (the cluster transform also returns
		// nil with no backends), so there is nothing to publish.
		if clustersForUcc == nil || clientEndpointResources == nil {
			logger.Debug("per-client inputs not ready; deferring snapshot", "client", ucc.ResourceName())
			return nil
		}

		logger.Debug("found perclient clusters", "client", ucc.ResourceName(), "clusters", len(clustersForUcc.clusters.Items))
		clusterResources := clustersForUcc.clusters
		clustersHash := clustersForUcc.clustersHash
		clusterVersions := clustersForUcc.clusterVersions

		// Filter after shared backend translation, which stays O(backends). EDS
		// alignment below drops CLAs for clusters removed here; backend-owned
		// LDS listeners and SDS secrets are filtered with their clusters too.
		var filtered bool
		clusterResources, clusterVersions, filtered = filterClustersToEmitted(
			scoping,
			listenerRouteSnapshot.EmittedClusters,
			clusterResources,
			clusterVersions,
		)
		// unscopedBy is why this client's xDS scoping is off: the gateway's own
		// request-time selectors no claim accounts for, else those in a kept
		// backend's listeners.
		var unscopedBy []string
		if scoping.ScopesClusters() {
			unscopedBy = listenerRouteSnapshot.EmittedClusters.unaccountedSelectors(scoping.Claims())
			if len(unscopedBy) == 0 {
				clusterResources, clusterVersions, filtered, unscopedBy = keepBackendListenerTargets(
					clustersForUcc.clusters,
					clustersForUcc.clusterVersions,
					clusterResources,
					clusterVersions,
					clustersForUcc.backendListeners,
				)
			}
		}
		backendListeners, backendSecrets := clustersForUcc.listeners, clustersForUcc.secrets
		if filtered {
			// A dropped backend's own listeners and secrets go too: kept, they would
			// sit on every proxy, naming a cluster it does not have.
			backendListeners = keptBackendResources(clusterResources.Items, clustersForUcc.backendListeners)
			backendSecrets = keptBackendResources(clusterResources.Items, clustersForUcc.backendSecrets)
			// Re-version: an unchanged version would leave Envoy serving dropped clusters.
			clustersHash = emittedClustersHash(clusterVersions)
			clusterResources.Version = strconv.FormatUint(clustersHash, 10)
			logger.Debug("emitting referenced clusters only",
				"client", ucc.ResourceName(),
				"emitted", len(clusterResources.Items),
				"translated", len(clustersForUcc.clusters.Items))
		}
		if scoping.ScopesClusters() {
			// Recorded even when nothing was dropped: dropped_clusters at 0
			// means scoping gains nothing, and disabled_gateways must read 0
			// before it can go to 1.
			recordClusterScopingDropped(ucc.ResourceName(),
				len(clustersForUcc.clusters.Items)-len(clusterResources.Items))
			reportClusterScopingDisabled(logger, ucc.ResourceName(), unscopedBy)
		}

		snap := XdsSnapWrapper{translatedClusters: clustersForUcc.clusters.Items}
		if snap.translatedClusters == nil {
			// Known to be empty, unlike an unset field.
			snap.translatedClusters = map[string]envoycachetypes.ResourceWithTTL{}
		}
		if len(listenerRouteSnapshot.Clusters) > 0 {
			clustersProto := make(map[string]envoycachetypes.ResourceWithTTL, len(listenerRouteSnapshot.Clusters)+len(clusterResources.Items))
			maps.Copy(clustersProto, clusterResources.Items)
			for _, item := range listenerRouteSnapshot.Clusters {
				clustersProto[envoycache.GetResourceName(item.Resource)] = item
			}
			clusterResources.Version = strconv.FormatUint(clustersHash^listenerRouteSnapshot.ClustersHash, 10)
			clusterResources.Items = clustersProto
		}
		// A referenced backend's own listeners route on to clusters no gateway
		// route names (a CONNECT tunnel's proxy), so its routes wait for those too.
		referenced := withBackendListenerRoutingTargets(listenerRouteSnapshot.ReferencedClusters, clustersForUcc.backendListeners)
		missingClusters := findMissingReferencedClusters(
			referenced,
			clusterResources.Items,
			clustersForUcc.erroredClusters,
		)
		// Envoy requests CLAs by the names CDS gives it, so EDS matches this
		// snapshot's CDS. newSnapshotCache serves EDS per request (#14865), so an
		// unrequested CLA no longer holds back the rest; matching keeps the
		// snapshot consistent, and keeps that protection if the cache changes.
		bootstrapEndpoint := ""
		if ucc.KnowsLocalCluster {
			bootstrapEndpoint, _, _ = ucc.LocalClusterInfo()
		}
		endpointRes, endpointHashes, synthesizedEndpoints := filterEndpointResourcesForClusters(clusterResources, clientEndpointResources.endpoints, clientEndpointResources.rowHashes, bootstrapEndpoint)
		// Every EDS cluster now has a CLA; synthesized ones mark underived rows.
		// A derived empty CLA is not a gap.
		missingEndpointClusters := findMissingReferencedEndpointResources(
			referenced,
			clusterResources.Items,
			synthesizedEndpoints,
			clustersForUcc.erroredClusters,
		)

		snap.missingReferenced = missingClusters
		snap.missingEndpointsReferenced = missingEndpointClusters
		// Settle only placeholders whose rows are on their way. A source-less
		// synthesized CLA is the cluster's steady state and publishes as-is.
		for name := range synthesizedEndpoints {
			if name == bootstrapEndpoint || endpoints.HasEndpointsSource(kctx, name) {
				snap.synthesizedEndpoints = append(snap.synthesizedEndpoints, name)
			}
		}
		slices.Sort(snap.synthesizedEndpoints)
		snap.erroredClusters = slices.Sorted(slices.Values(clustersForUcc.erroredClusters))
		snap.proxyKey = ucc.ResourceName()
		snap.scopesClusters = scoping.ScopesClusters()
		snap.endpointHashes = endpointHashes
		snapshot := &envoycache.Snapshot{}
		snapshot.Resources[envoycachetypes.Cluster] = clusterResources
		snapshot.Resources[envoycachetypes.Endpoint] = endpointRes
		snapshot.Resources[envoycachetypes.Route] = listenerRouteSnapshot.Routes
		snapshot.Resources[envoycachetypes.Listener] = mergeBackendResources(listenerRouteSnapshot.Listeners, backendListeners)
		snapshot.Resources[envoycachetypes.Secret] = mergeBackendResources(listenerRouteSnapshot.Secrets, backendSecrets)
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
			"listeners", resourcesStringer(snapshot.Resources[envoycachetypes.Listener]).String(),
			"clusters", resourcesStringer(clusterResources).String(),
			"routes", resourcesStringer(listenerRouteSnapshot.Routes).String(),
			"endpoints", resourcesStringer(endpointRes).String(),
			"secrets", resourcesStringer(snapshot.Resources[envoycachetypes.Secret]).String(),
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

// mergeBackendResources combines Gateway resources with those generated with
// backend clusters, without mutating shared resources. Gateway resources take
// precedence on name collisions.
func mergeBackendResources[M proto.Message](gateway envoycache.Resources, backend baseResources[M]) envoycache.Resources {
	if len(backend.items) == 0 {
		return gateway
	}
	items := make(map[string]envoycachetypes.ResourceWithTTL, len(gateway.Items)+len(backend.items))
	for _, r := range backend.items {
		res := r.ResourceWithTTL()
		items[envoycache.GetResourceName(res.Resource)] = res
	}
	maps.Copy(items, gateway.Items)
	return envoycache.Resources{
		Version: fmt.Sprintf("%s-backend-%d", gateway.Version, backend.hash),
		Items:   items,
	}
}

// collectReferencedClusters returns the routing targets of routes and
// listeners: route-action and TCP-proxy clusters and weighted clusters,
// including inlined route configurations. It never unpacks HTTP filter or
// per-route configs. Translation computes it once per gateway, not per client.
//
// Filter-called clusters (ext_authz, ext_proc, JWKS, OAuth2, rate limit,
// access logs) are not collected: one lagging side service must not block all
// route updates. The filter fails or follows its failure_mode_allow until the
// cluster is ready, and the gate keeps held endpoints for underived CLAs.
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

// addFilterChainTargets collects TCP targets and inline HTTP route targets.
// RDS targets come from the separate route configurations.
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

// findMissingReferencedEndpointResources lists referenced EDS clusters whose
// CLA is a synthesized placeholder because no row was derived. Their
// endpoints are unknown, so they defer: derivation is lagging (a kube
// Service, even ExternalName, derives a row per resolvable port), or a plugin
// never supplies one. A derived empty CLA is known truth and must not defer:
// scale-to-zero and crashlooping backends are steady states (#14352).
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

// filterEndpointResourcesForClusters keeps only the CLAs that CDS's EDS clusters
// name, plus bootstrapEndpoint's (the bootstrap local cluster, not in dynamic
// CDS): Envoy never requests the others, and an ADS cache withholds a whole
// response carrying one (#14471; newSnapshotCache serves EDS per request since
// #14865). It adds an empty CLA for each EDS cluster without one and returns
// their names; the gate decides which reach the client
// (settleSynthesizedEndpoints), since an empty CLA replaces held endpoints. The
// returned hashes version only the published set.
func filterEndpointResourcesForClusters(clusters envoycache.Resources, endpoints envoycache.Resources, rowHashes map[string]uint64, bootstrapEndpoint string) (envoycache.Resources, map[string]uint64, map[string]struct{}) {
	if endpointsMatchClusters(clusters, endpoints, bootstrapEndpoint) {
		return endpoints, rowHashes, nil
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
	}
	if len(synthesized) == 0 && len(filteredEndpoints) == len(endpoints.Items) {
		return endpoints, rowHashes, nil
	}
	filtered := envoycache.NewResourcesWithTTL("", filteredEndpoints)
	hashes := endpointSetHashes(filtered.Items, rowHashes, nil)
	return versionEndpointSet(filtered, hashes), hashes, synthesized
}

// endpointsMatchClusters reports, without allocating, the common case: one
// CLA per EDS cluster plus bootstrapEndpoint, and nothing to filter or
// synthesize. It returns false on anything unusual, such as a service_name
// alias, leaving it to the general path.
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
