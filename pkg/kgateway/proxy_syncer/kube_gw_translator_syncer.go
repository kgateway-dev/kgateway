package proxy_syncer

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sync"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

func (s *ProxyTranslator) syncXds(
	ctx context.Context,
	snapWrap XdsSnapWrapper,
) {
	proxyKey := snapWrap.proxyKey

	// Errored clusters intentionally fail closed. Restoring a cluster from the previous
	// snapshot could keep serving with stale configuration that bypasses the security
	// policy whose validation now fails. snapshotPerClient omits both the errored cluster
	// and its endpoint assignment so that only the affected backend fails, without
	// blocking xDS updates for unrelated clusters.

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	logger.Debug("syncing xds snapshot", "proxy_key", proxyKey)

	logger.Log(ctx, logging.LevelTrace, "syncing xds snapshot", "proxy_key", proxyKey)

	if snapWrap.deferred() {
		recordSnapshotDefer(proxyKey, snapWrap.missingReferenced, snapWrap.missingEndpointsReferenced)
		// Resolve per cluster against the snapshot published on this connection
		// instead of withholding everything: withhold a client not yet published
		// on it up to the first-publish budget, carry referenced clusters missing
		// from the build, keep held endpoints for previously-referenced clusters
		// whose CLA is underived, and hold a flip onto a newly-referenced unready
		// cluster until it is ready or the flip-release bound expires (#14352).
		// The hold publishes a new cluster's CDS before the RDS that uses it
		// (reference-ahead). See resolveDeferredPerCluster. This runs under
		// the gate lock, so budget timers cannot race it.
		if err := s.gate.resolveDeferred(ctx, s.xdsCache, snapWrap); err != nil {
			logPublishFailure(proxyKey, err)
		}
		return
	}

	// The snapshot carries no unrequested resource: snapshotPerClient drops
	// CLAs for clusters absent from CDS, and resolution carries only
	// cluster/CLA pairs. MakeConsistent() would mutate the snapshot shared with
	// the krt cache. The gate settles synthesized CLAs, cancels pending bounded
	// publishes and flip releases, and cannot race a budget timer. A transition
	// alone never makes a build deferred, so with either window configured,
	// coherent builds publish through publishWithTransitionGraces.
	if s.gate.appliesTransitionGraces() {
		if err := s.gate.publishWithTransitionGraces(ctx, s.xdsCache, snapWrap); err != nil {
			logPublishFailure(proxyKey, err)
		}
		return
	}

	if err := s.gate.publish(ctx, s.xdsCache, snapWrap); err != nil {
		// A rejected snapshot leaves the client on its previous config.
		logPublishFailure(proxyKey, err)
	}
}

// publishedReferencedClusters returns the routing targets of the published
// routes and listeners. Callers must not modify the shared result.
func publishedReferencedClusters(published envoycache.ResourceSnapshot) map[string]struct{} {
	routes := published.GetResourcesAndTTL(envoyresourcev3.RouteType)
	listeners := published.GetResourcesAndTTL(envoyresourcev3.ListenerType)
	return publishedRefs.get(routes, listeners, func() map[string]struct{} {
		return collectReferencedClusters(envoycache.Resources{Items: routes}, envoycache.Resources{Items: listeners})
	})
}

// publishedRefs memoizes publishedReferencedClusters. Collecting targets
// unmarshals every listener, which under the gate's single lock would
// serialize all clients' deferred builds. Published routes and listeners
// change rarely, and published snapshots reuse their translation's immutable
// item maps.
var publishedRefs = &referencedClustersMemo{}

// referencedClustersMemo keys target sets by the identity of the route and
// listener item maps. An entry retains its maps, so their addresses cannot be
// reused and identity implies equal content. It is cleared at
// maxReferencedClustersMemo entries.
type referencedClustersMemo struct {
	mu      sync.Mutex
	entries map[[2]uintptr]referencedClustersEntry
}

type referencedClustersEntry struct {
	routes, listeners map[string]envoycachetypes.ResourceWithTTL
	refs              map[string]struct{}
}

const maxReferencedClustersMemo = 4096

func (m *referencedClustersMemo) get(
	routes, listeners map[string]envoycachetypes.ResourceWithTTL,
	compute func() map[string]struct{},
) map[string]struct{} {
	if routes == nil && listeners == nil {
		return compute()
	}
	key := [2]uintptr{reflect.ValueOf(routes).Pointer(), reflect.ValueOf(listeners).Pointer()}
	m.mu.Lock()
	entry, ok := m.entries[key]
	m.mu.Unlock()
	if ok {
		return entry.refs
	}
	refs := compute()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries == nil || len(m.entries) >= maxReferencedClustersMemo {
		m.entries = make(map[[2]uintptr]referencedClustersEntry)
	}
	m.entries[key] = referencedClustersEntry{routes: routes, listeners: listeners, refs: refs}
	return refs
}

// flipBlockingClusters returns the clusters a deferred build would hold its
// flip for: missing ones the published configuration does not yet reference.
// Missing clusters still in published CDS are carried instead.
// Pending blockers remain blocking while the build still reports their
// cluster or endpoint resources missing. Publishing a listener during the
// hold can introduce a reference to a blocked cluster without filling that gap.
func flipBlockingClusters(snapWrap XdsSnapWrapper, published envoycache.ResourceSnapshot, pending []string) []string {
	publishedRefs := publishedReferencedClusters(published)
	oldClusters := published.GetResourcesAndTTL(envoyresourcev3.ClusterType)
	var flipBlocking []string
	pendingSet := stringSet(pending)
	for _, name := range snapWrap.missingReferenced {
		if _, held := pendingSet[name]; held {
			flipBlocking = append(flipBlocking, name)
			continue
		}
		if _, wasReferenced := publishedRefs[name]; wasReferenced {
			continue // already live, including a missing cluster published at budget expiry
		}
		if _, wasPublished := oldClusters[name]; wasPublished {
			continue // carried forward
		}
		flipBlocking = append(flipBlocking, name)
	}
	for _, name := range snapWrap.missingEndpointsReferenced {
		if _, held := pendingSet[name]; held {
			flipBlocking = append(flipBlocking, name)
			continue
		}
		// Previously referenced: the client keeps its endpoints. When scoped, a
		// dropped backend's listener still ships and can name a cluster scoping
		// left out of CDS; the client has no endpoints for that one.
		_, wasReferenced := publishedRefs[name]
		_, wasPublished := oldClusters[name]
		if wasReferenced && (wasPublished || !snapWrap.scopesClusters) {
			continue
		}
		flipBlocking = append(flipBlocking, name)
	}
	slices.Sort(flipBlocking)
	return flipBlocking
}

// resolveDeferredPerCluster composes the snapshot to publish for a deferred
// wrapper against the one published on the client's current connection:
//
//   - missing referenced clusters that are published are carried with their
//     CLAs, and with what they need that the build lacks (carryDependencies);
//   - if every other gap was already referenced, the new routes publish, and
//     underived CLAs keep the client's endpoints (the gate settles them);
//   - otherwise a never-ready cluster is newly referenced: routes, listeners and
//     secrets stay published, every published cluster the build lacks is
//     carried (the held config's whole closure, filter-called clusters
//     included), and new CDS/EDS still go out so the cluster can warm.
//
// A carried CLA yields to a built one sharing its EDS service_name. graced
// clusters are carried too. holdFlips=false, used by the flip-release bound,
// publishes despite never-ready gaps so an unready reference cannot pin updates
// forever (#14352). lastPublished gives each copied CLA's last published hash.
// pendingBlockers preserves gaps across listeners installed by an earlier hold.
// Returns the snapshot, the flip-blocking clusters if held (else nil), and the
// EDS contributions.
func resolveDeferredPerCluster(snapWrap XdsSnapWrapper, published envoycache.ResourceSnapshot, holdFlips bool, pendingBlockers []string, graced map[string]struct{}, lastPublished func(string) (uint64, bool)) (*envoycache.Snapshot, []string, map[string]uint64) {
	oldClusters := published.GetResourcesAndTTL(envoyresourcev3.ClusterType)
	flipBlocking := flipBlockingClusters(snapWrap, published, pendingBlockers)

	composed := &envoycache.Snapshot{}
	*composed = *snapWrap.snap

	// carryRefs are the clusters whose cluster/CLA pairs the composed snapshot
	// needs. A held flip needs every published cluster: the held listeners
	// still call filter clusters (ext_authz, JWKS, ...) that a new build may
	// drop. The flip-release bound limits this to one budget. Without a hold,
	// only missing referenced clusters are carried.
	carryRefs := make(map[string]struct{})
	var heldBlocking []string
	if holdFlips && len(flipBlocking) > 0 {
		holdType := func(rt envoycachetypes.ResponseType, typeURL envoyresourcev3.Type) {
			composed.Resources[rt] = envoycache.Resources{
				Version: published.GetVersion(typeURL),
				Items:   published.GetResourcesAndTTL(typeURL),
			}
		}
		holdType(envoycachetypes.Route, envoyresourcev3.RouteType)
		holdType(envoycachetypes.Listener, envoyresourcev3.ListenerType)
		holdType(envoycachetypes.Secret, envoyresourcev3.SecretType)
		logger.Debug("holding route flip until newly-referenced clusters are ready",
			"proxy_key", snapWrap.proxyKey,
			"flip_blocking", flipBlocking,
		)
		for name := range oldClusters {
			carryRefs[name] = struct{}{}
		}
		heldBlocking = flipBlocking
	} else {
		for _, name := range snapWrap.missingReferenced {
			carryRefs[name] = struct{}{}
		}
	}

	// Graced clusters are carried the same way, giving the route update that
	// dropped them time to land.
	for name := range graced {
		carryRefs[name] = struct{}{}
	}

	// Carry missing carryRefs from the published snapshot. A CLA is carried only
	// with its cluster.
	newClusters := composed.Resources[envoycachetypes.Cluster]
	newEndpoints := composed.Resources[envoycachetypes.Endpoint]
	edsHashes := snapWrap.endpointHashes
	oldEndpoints := published.GetResourcesAndTTL(envoyresourcev3.EndpointType)
	erroredSet := stringSet(snapWrap.erroredClusters)
	expiredSet := stringSet(snapWrap.expiredCarry)
	var carried []string
	var carryHash uint64
	clusterItems := newClusters.Items
	endpointItems := newEndpoints.Items
	// A carried cluster can need listeners, secrets and clusters its build
	// lacks (carryDependencies); the clusters join the worklist under the same
	// rules.
	deps := newCarryDependencies(published, composed)
	pending := slices.Collect(maps.Keys(carryRefs))
	for len(pending) > 0 {
		name := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if name == wellknown.BlackholeClusterName {
			continue
		}
		if _, expired := expiredSet[name]; expired {
			// Its carry hit the warm withhold bound (boundCarryLocked); a held
			// flip's closure must not bring it back.
			continue
		}
		if _, errored := erroredSet[name]; errored {
			// Never carry an errored cluster, even during an unrelated held flip:
			// its stale config would bypass the policy whose failure errored it.
			// An invalid BackendTLSPolicy must 5xx (conformance test
			// BackendTLSPolicyInvalidCACertificateRef), not keep its old TLS.
			continue
		}
		if _, ok := clusterItems[name]; ok {
			continue
		}
		old, ok := oldClusters[name]
		if !ok {
			continue // neither snapshot has it; the route 503s as it already did
		}
		if len(carried) == 0 {
			// copy-on-write: never mutate the krt-cached wrapper's maps
			clusterItems = cloneResourceItems(newClusters.Items)
			endpointItems = cloneResourceItems(newEndpoints.Items)
		}
		clusterItems[name] = old
		carried = append(carried, name)
		// Hash carried content too. Otherwise a vanish/return/vanish cycle can
		// reuse a version for different protos, and a client reconnecting with
		// that version is never resent the newer content.
		carryHash ^= utils.HashString(name) ^ utils.HashProto(old.Resource)
		pending = append(pending, deps.carry(old)...)
		claName, requiresEndpointResource := endpointResourceNameForCluster(old)
		if !requiresEndpointResource {
			continue
		}
		if _, built := endpointItems[claName]; built {
			// Another built cluster shares this EDS service_name. Keep the
			// built CLA; the published copy would serve stale endpoints under
			// the built version.
			continue
		}
		if oldCla, ok := oldEndpoints[claName]; ok {
			endpointItems[claName] = oldCla
		}
	}
	if len(carried) > 0 {
		slices.Sort(carried)
		composed.Resources[envoycachetypes.Cluster] = envoycache.Resources{
			Version: fmt.Sprintf("%s-carry-%d", newClusters.Version, carryHash),
			Items:   clusterItems,
		}
		// Reuse carried contributions so direct and composed publication agree.
		edsHashes = endpointSetHashes(endpointItems, snapWrap.endpointHashes, lastPublished)
		composed.Resources[envoycachetypes.Endpoint] = envoycache.Resources{
			Version: endpointSetVersion(edsHashes),
			Items:   endpointItems,
		}
		logger.Debug("carried forward previously-published clusters",
			"proxy_key", snapWrap.proxyKey,
			"carried", carried,
		)
	}
	deps.apply(composed)
	if len(heldBlocking) > 0 {
		addBuiltClusterListeners(composed, snapWrap.snap)
	}

	return composed, heldBlocking, edsHashes
}

func cloneResourceItems(items map[string]envoycachetypes.ResourceWithTTL) map[string]envoycachetypes.ResourceWithTTL {
	out := make(map[string]envoycachetypes.ResourceWithTTL, len(items))
	maps.Copy(out, items)
	return out
}
