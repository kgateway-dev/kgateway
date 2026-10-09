package proxy_syncer

import (
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
)

// filterClustersToEmitted drops backend clusters the gateway's generated
// configuration does not reference, which is what shrinks a proxy's
// config_dump and its per-cluster stats cardinality to the backends it
// actually uses.
//
// It returns the retained clusters and the digests of the ones kept, so the
// caller can version the result over what survived: dropping a cluster has to
// move the CDS version, or Envoy keeps serving the set it already has.
//
// Three cases pass through untouched, each deliberately:
//
//   - CDS is not scoped, which is the default, so the filter is inert until an
//     operator opts in.
//   - A gateway with an unresolvable request-time selector. Its routes may
//     select a cluster named nowhere in the configuration, and pruning
//     candidates would not fail visibly -- the selecting plugin falls back --
//     so the whole gateway reverts to emitting everything.
//   - Errored clusters, which are already excluded from CDS upstream of here
//     and whose names the caller still needs for EDS alignment and status.
//
// Ancillary clusters contributed by plugins as ExtraClusters are merged into
// CDS after this runs and are not filtered. They are per-gateway and O(plugins),
// not O(Services), so they are not the population #13586 is about, and a plugin
// that declares one has already taken responsibility for it being needed.
func filterClustersToEmitted(
	scoping clusterScoping,
	emitted emittedClusters,
	clusters envoycache.Resources,
	clusterVersions map[string]uint64,
) (envoycache.Resources, map[string]uint64, bool) {
	if !scoping.ScopesClusters() || !emitted.Filterable() {
		return clusters, clusterVersions, false
	}

	retained := make(map[string]envoycachetypes.ResourceWithTTL, len(clusters.Items))
	retainedVersions := make(map[string]uint64, len(clusterVersions))
	for name, item := range clusters.Items {
		if _, referenced := emitted.Names[name]; !referenced {
			continue
		}
		retained[name] = item
		if version, ok := clusterVersions[name]; ok {
			retainedVersions[name] = version
		}
	}

	if len(retained) == len(clusters.Items) {
		return clusters, clusterVersions, false
	}

	clusters.Items = retained
	return clusters, retainedVersions, true
}

// emittedClustersHash folds the retained cluster digests the way the unfiltered
// path folds all of them, so a filtered snapshot carries a version derived from
// exactly the clusters it contains.
func emittedClustersHash(clusterVersions map[string]uint64) uint64 {
	var hash uint64
	for _, version := range clusterVersions {
		hash ^= version
	}
	return hash
}
