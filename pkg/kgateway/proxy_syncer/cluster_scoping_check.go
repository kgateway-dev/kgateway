package proxy_syncer

import (
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

// ClusterScopingDecision is what REFERENCED cluster discovery decides for one
// gateway's generated configuration. DecideClusterScoping returns it, so a
// plugin's tests can check its ClusterEmissionClaim against configuration the
// plugin generates, without a controller.
type ClusterScopingDecision struct {
	// Scoped reports whether the gateway's CDS is filtered. When false, every
	// cluster is emitted.
	Scoped bool
	// Unaccounted are the request-time selectors that keep the gateway
	// unscoped, with any reason, as the controller's warning lists them.
	Unaccounted []string

	emitted emittedClusters
	kept    claimedSet
}

// Emits reports whether a cluster with this name, if it exists, reaches the
// gateway's CDS. On an unscoped gateway every cluster does; on a scoped one,
// a cluster the configuration names or a claimed use keeps.
func (d ClusterScopingDecision) Emits(cluster string) bool {
	return !d.Scoped || d.emitted.emits(d.kept, cluster)
}

// DecideClusterScoping applies REFERENCED cluster discovery to a gateway's
// routes and listeners as the controller does, with the claims the given
// plugins make. A plugin's translation-pass ResourcesToAdd, which scoping never
// filters, is not considered.
func DecideClusterScoping(
	policies sdk.ContributesPolicies,
	routes []*envoyroutev3.RouteConfiguration,
	listeners []*envoylistenerv3.Listener,
) ClusterScopingDecision {
	claims := collectEmissionClaims(policies)
	scoping := clusterScoping{mode: apisettings.ClusterDiscoveryReferenced, claims: claims}
	emitted := emittedClustersFor(scoping, sliceToResources(routes), sliceToResources(listeners))
	unaccounted := emitted.unaccountedSelectors(claims)
	return ClusterScopingDecision{
		Scoped:      len(unaccounted) == 0,
		Unaccounted: unaccounted,
		emitted:     emitted,
		kept:        claims.keptFor(emitted),
	}
}
