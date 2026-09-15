package proxy_syncer

import (
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
)

// filterClustersToEmitted drops backend clusters the generated configuration
// does not reference, shrinking config_dump and per-cluster stats. It returns
// the retained versions and whether anything was dropped, so the caller can
// re-version CDS: an unchanged version leaves Envoy on the old set.
//
// It is a no-op when CDS is not scoped, or when a request-time selector may pick
// an unnamed cluster, where pruning would silently hit a plugin fallback.
// Errored clusters are already absent. Plugin ExtraClusters merge in afterward
// unfiltered: they scale with plugins, not Services, and the plugin vouches for
// them.
func filterClustersToEmitted(
	scoping clusterScoping,
	emitted emittedClusters,
	clusters envoycache.Resources,
	clusterVersions map[string]uint64,
) (envoycache.Resources, map[string]uint64, bool) {
	claims := scoping.Claims()
	if !scoping.ScopesClusters() || !emitted.Filterable(claims) {
		return clusters, clusterVersions, false
	}

	retained := make(map[string]envoycachetypes.ResourceWithTTL, len(clusters.Items))
	retainedVersions := make(map[string]uint64, len(clusterVersions))
	for name, item := range clusters.Items {
		_, referenced := emitted.Names[name]
		// A claimed cluster is kept even when nothing names it: that is the
		// whole point of a claim, and it covers the plugin's own placeholder
		// cluster, which stops being named the moment its feature is doing its
		// job and would otherwise be the first thing pruned.
		if !referenced && !claims.claims(name) {
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

// emittedClustersHash folds the retained versions as the unfiltered path does,
// so the version covers exactly the clusters present.
func emittedClustersHash(clusterVersions map[string]uint64) uint64 {
	var hash uint64
	for _, version := range clusterVersions {
		hash ^= version
	}
	return hash
}
