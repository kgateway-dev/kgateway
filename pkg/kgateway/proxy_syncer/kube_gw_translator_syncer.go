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
		// Per-cluster readiness resolution. The snapshot was built while some
		// referenced cluster was not ready; decide per cluster against the
		// snapshot published on the client's current connection instead of
		// withholding everything:
		//
		//   - a client not yet published on this connection is withheld up to
		//     the first-publish budget (see publishGate);
		//   - previously-published clusters that vanished from the build are
		//     carried forward with their CLAs (make-before-break);
		//   - previously-referenced clusters whose CLA row is not derived keep
		//     the endpoints the client holds for them
		//     (settleSynthesizedEndpoints): a missing row is unknown, not
		//     empty, since a Service without EndpointSlices still derives an
		//     empty row;
		//   - only a route flip onto a NEWLY-referenced not-yet-derived
		//     cluster is held: routes/listeners/secrets stay at the published
		//     versions while the new clusters warm in the background, and the
		//     flip goes out in a later snapshot once they are ready — or at
		//     the flip-release bound (see publishGate), whichever comes
		//     first, so a backend that never derives endpoints cannot pin
		//     updates forever (#14352). The hold also gives the flip a
		//     reference-ahead shape: the CDS carrying a new cluster is
		//     published in an earlier snapshot than the RDS that uses it.
		//
		// The whole decision runs under the gate lock so it cannot race the
		// gate's expiring budget timers (see resolveDeferred).
		if err := s.gate.resolveDeferred(ctx, s.xdsCache, snapWrap); err != nil {
			logPublishFailure(proxyKey, err)
		}
		return
	}

	// The snapshot publishes no unrequested resource by construction:
	// snapshotPerClient drops CLAs for clusters absent from CDS, and the
	// per-cluster resolution only carries cluster/CLA pairs — so we do not
	// rely on a post-hoc MakeConsistent() pass, which would also have mutated
	// the snapshot shared with the krt cache. Publication goes through the
	// publish gate so it settles synthesized CLAs, cancels any pending bounded
	// publish or flip release, and cannot race an expiring budget timer.
	if err := s.gate.publish(ctx, s.xdsCache, snapWrap); err != nil {
		// A rejected snapshot leaves the client on its previous config; surface
		// it rather than silently dropping the update.
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

// resolveDeferredPerCluster composes the snapshot actually published for a
// deferred wrapper, given the snapshot published on the client's current
// connection:
//
//   - missing referenced clusters present in the published snapshot are
//     carried forward together with their CLAs, and with the listeners,
//     secrets and clusters they need that the build lacks (carryDependencies);
//   - if every remaining gap is a previously-referenced cluster (present in
//     the published routes' reference set), the new routes publish as-is;
//     clusters among them whose CLA is not derived keep the endpoints the
//     client holds (the gate settles synthesized CLAs at publish time);
//   - otherwise some gap is a newly-referenced cluster that has never been
//     ready: routes, listeners, and secrets are held at their published
//     versions (so nothing flips onto the unready cluster), every published
//     cluster the build lacks is carried forward with its CLA (the held
//     configuration's whole dependency closure, filter-called clusters
//     included), and the new CDS/EDS — including the warming cluster — still
//     goes out.
//
// A carried cluster's CLA is taken from the published snapshot unless the
// build already has that assignment for another cluster sharing its EDS
// service_name; the built assignment is then kept.
//
// The second return value lists the flip-blocking clusters when the hold was
// applied, nil otherwise. holdFlips=false composes the released form — the
// new routes publish as-is even with never-ready gaps — used by the gate's
// flip-release bound so a steady-state-unready reference cannot pin the
// client's route/listener/secret updates forever (#14352).
// lastPublished answers for assignments copied from the published snapshot
// with the hash each was last published under (eds_set_version.go). The last
// result holds the contributions of the composed EDS set.
func resolveDeferredPerCluster(snapWrap XdsSnapWrapper, published envoycache.ResourceSnapshot, holdFlips bool, lastPublished func(string) (uint64, bool)) (*envoycache.Snapshot, []string, map[string]uint64) {
	publishedRefs := publishedReferencedClusters(published)
	oldClusters := published.GetResourcesAndTTL(envoyresourcev3.ClusterType)

	// Only gaps the published config does not yet reference block the flip.
	// Missing clusters still in published CDS are carried instead.
	var flipBlocking []string
	for _, name := range snapWrap.missingReferenced {
		if _, wasReferenced := publishedRefs[name]; wasReferenced {
			continue // already live, including a missing cluster published at budget expiry
		}
		if _, wasPublished := oldClusters[name]; wasPublished {
			continue // carried forward below
		}
		flipBlocking = append(flipBlocking, name)
	}
	for _, name := range snapWrap.missingEndpointsReferenced {
		if _, wasReferenced := publishedRefs[name]; wasReferenced {
			continue // previously-referenced: the client keeps its endpoints
		}
		flipBlocking = append(flipBlocking, name)
	}
	slices.Sort(flipBlocking)

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

	// Carry forward cluster/CLA pairs (a carried CLA always travels with
	// its cluster) for carryRefs missing from the new build.
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
		// The carried assignments keep the contribution they were published
		// under, so the composed set is versioned like a direct publication.
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

	return composed, heldBlocking, edsHashes
}

func cloneResourceItems(items map[string]envoycachetypes.ResourceWithTTL) map[string]envoycachetypes.ResourceWithTTL {
	out := make(map[string]envoycachetypes.ResourceWithTTL, len(items))
	maps.Copy(out, items)
	return out
}
