package proxy_syncer

import (
	"context"
	"fmt"
	"maps"
	"slices"

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
	snap := snapWrap.snap
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

	if snapWrap.deferred {
		recordSnapshotDefer(proxyKey, snapWrap.missingReferenced, snapWrap.missingEndpointsReferenced)
		// Per-cluster readiness resolution. The snapshot was built while some
		// referenced cluster was not ready; decide per cluster against the
		// currently-published snapshot instead of withholding everything:
		//
		//   - a never-published client is withheld up to the first-publish
		//     budget (see publishGate);
		//   - previously-published clusters that vanished from the build are
		//     carried forward with their CLAs (make-before-break);
		//   - previously-referenced clusters whose CLA row vanished publish
		//     the synthesized empty — their EndpointSlices are gone, and that
		//     is the truth — so Envoy stops routing to endpoints that no
		//     longer exist;
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
		if err := s.gate.resolveDeferred(ctx, s.xdsCache, snapWrap, s.hasPriorXDSVersion); err != nil {
			logger.Error("failed to set xds snapshot", "proxy_key", proxyKey, "error", err)
		}
		return
	}

	// The snapshot is EDS-consistent by construction: snapshotPerClient drops
	// CLAs for clusters absent from CDS and synthesizes empty assignments for
	// EDS clusters that have no CLA yet (see filterEndpointResourcesForClusters),
	// and the per-cluster resolution only carries cluster/CLA pairs — so we do
	// not rely on a post-hoc MakeConsistent() pass, which would also have
	// mutated the snapshot shared with the krt cache. Publication goes
	// through the publish gate so it cancels any pending bounded publish or
	// flip release and cannot race an expiring budget timer.
	if err := s.gate.publish(ctx, s.xdsCache, proxyKey, snap); err != nil {
		// A rejected snapshot leaves the client on its previous config; surface
		// it rather than silently dropping the update.
		logger.Error("failed to set xds snapshot", "proxy_key", proxyKey, "error", err)
	}
}

// hasPriorXDSVersion reports whether the client reported a prior accepted xDS
// version on connect — i.e. it may already be serving traffic even though this
// controller has no local snapshot for it (reconnect / controller restart).
func (s *ProxyTranslator) hasPriorXDSVersion(proxyKey string) bool {
	return s.xdsClientState != nil && s.xdsClientState.HasPriorXDSVersion(proxyKey)
}

// publishedReferencedClusters returns the dataplane-referenced cluster set of
// the currently-published snapshot (its routes and listeners).
func publishedReferencedClusters(published envoycache.ResourceSnapshot) map[string]struct{} {
	routes := envoycache.Resources{Items: published.GetResourcesAndTTL(envoyresourcev3.RouteType)}
	listeners := envoycache.Resources{Items: published.GetResourcesAndTTL(envoyresourcev3.ListenerType)}
	return collectReferencedClusters(routes, listeners)
}

// resolveDeferredPerCluster composes the snapshot actually published for a
// deferred wrapper, given the currently-published snapshot:
//
//   - missing referenced clusters present in the published snapshot are
//     carried forward together with their CLAs;
//   - if every remaining gap is a previously-referenced cluster (present in
//     the published routes' reference set), the new routes publish as-is —
//     including synthesized empty CLAs for clusters whose slices vanished;
//   - otherwise some gap is a newly-referenced cluster that has never been
//     ready: routes, listeners, and secrets are held at their published
//     versions (so nothing flips onto the unready cluster), the held routes'
//     cluster/CLA dependencies are carried forward, and the new CDS/EDS —
//     including the warming cluster — still goes out.
//
// The second return value lists the flip-blocking clusters when the hold was
// applied, nil otherwise. holdFlips=false composes the released form — the
// new routes publish as-is even with never-ready gaps — used by the gate's
// flip-release bound so a steady-state-unready reference cannot pin the
// client's route/listener/secret updates forever (#14352).
func resolveDeferredPerCluster(snapWrap XdsSnapWrapper, published envoycache.ResourceSnapshot, holdFlips bool) (*envoycache.Snapshot, []string) {
	publishedRefs := publishedReferencedClusters(published)
	oldClusters := published.GetResourcesAndTTL(envoyresourcev3.ClusterType)

	// A gap blocks the route flip only if the published config was not
	// already using the cluster: a previously-referenced cluster that is
	// missing gets carried forward, and one whose CLA row vanished publishes
	// the synthesized empty (its truth).
	var flipBlocking []string
	for _, name := range snapWrap.missingReferenced {
		if _, wasPublished := oldClusters[name]; wasPublished {
			continue // carried forward below
		}
		flipBlocking = append(flipBlocking, name)
	}
	for _, name := range snapWrap.missingEndpointsReferenced {
		if _, wasReferenced := publishedRefs[name]; wasReferenced {
			continue // previously-referenced: truth (synthesized empty CLA) publishes
		}
		flipBlocking = append(flipBlocking, name)
	}
	slices.Sort(flipBlocking)

	composed := &envoycache.Snapshot{}
	*composed = *snapWrap.snap

	// carryRefs are the references whose cluster/CLA pairs must exist in the
	// composed snapshot: the published routes' references when the flip is
	// held (those routes stay live), plus any previously-published missing
	// clusters on the truth path.
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
		logger.Info("holding route flip until newly-referenced clusters are ready",
			"proxy_key", snapWrap.proxyKey,
			"flip_blocking", flipBlocking,
		)
		recordFlipHeld(snapWrap.proxyKey)
		for name := range publishedRefs {
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
	oldEndpoints := published.GetResourcesAndTTL(envoyresourcev3.EndpointType)
	erroredSet := stringSet(snapWrap.erroredClusters)
	var carried []string
	var carryHash uint64
	clusterItems := newClusters.Items
	endpointItems := newEndpoints.Items
	for name := range carryRefs {
		if name == wellknown.BlackholeClusterName {
			continue
		}
		if _, errored := erroredSet[name]; errored {
			// Fail closed: a cluster whose current translation is errored is
			// never resurrected from the published snapshot, even while a flip
			// is held for an unrelated cluster. Serving it with its stale
			// (pre-error) config would silently bypass the very policy whose
			// failure errored it — e.g. an invalid BackendTLSPolicy must 5xx
			// (Gateway API conformance
			// BackendTLSPolicyInvalidCACertificateRef), not keep serving with
			// the previous TLS configuration.
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
		// The hash folds in the carried CONTENT, not just the names: the same
		// carried set over the same new-build version can otherwise reproduce
		// an identical version string around a vanish/return/vanish cycle
		// whose carried protos differ, and a client reconnecting with that
		// version as last-accepted would never be resent the newer content.
		carryHash ^= utils.HashString(name) ^ utils.HashProto(old.Resource)
		claName, requiresEndpointResource := endpointResourceNameForCluster(old)
		if !requiresEndpointResource {
			continue
		}
		if oldCla, ok := oldEndpoints[claName]; ok {
			endpointItems[claName] = oldCla
			carryHash ^= utils.HashProto(oldCla.Resource)
		}
	}
	if len(carried) > 0 {
		slices.Sort(carried)
		composed.Resources[envoycachetypes.Cluster] = envoycache.Resources{
			Version: fmt.Sprintf("%s-carry-%d", newClusters.Version, carryHash),
			Items:   clusterItems,
		}
		composed.Resources[envoycachetypes.Endpoint] = envoycache.Resources{
			Items: endpointItems,
		}
		composed.Resources[envoycachetypes.Endpoint] = versionEndpointResources(
			composed.Resources[envoycachetypes.Endpoint], nil,
			endpointClusterDigests(composed.Resources[envoycachetypes.Cluster], nil))
		logger.Info("carried forward previously-published clusters",
			"proxy_key", snapWrap.proxyKey,
			"carried", carried,
		)
		recordCarriedClusters(snapWrap.proxyKey, len(carried))
	}

	return composed, heldBlocking
}

func cloneResourceItems(items map[string]envoycachetypes.ResourceWithTTL) map[string]envoycachetypes.ResourceWithTTL {
	out := make(map[string]envoycachetypes.ResourceWithTTL, len(items))
	maps.Copy(out, items)
	return out
}
