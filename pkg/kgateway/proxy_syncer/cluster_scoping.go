package proxy_syncer

import (
	"time"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

// clusterScoping is the referenced-only discovery configuration, resolved once
// from Settings. ScopesClusters guards every scoping path (emitted-set walk,
// CDS filtering, transition windows); when false none run and the syncer's
// snapshot publishes unchanged.
type clusterScoping struct {
	mode apisettings.ClusterDiscoveryMode
	// dereferenceGrace is the de-reference grace. Read it through
	// DereferenceGrace, which is 0 when scoping is off.
	dereferenceGrace time.Duration
	// referenceAhead is how long a route update onto a newly-emitted cluster is
	// held. Read only through ReferenceAhead, which ignores it with scoping
	// off.
	referenceAhead time.Duration
	// claims are plugins' declared destinations the configuration never names,
	// collected once at startup.
	claims emissionClaims
}

func clusterScopingFrom(s apisettings.Settings, policies sdk.ContributesPolicies) clusterScoping {
	return clusterScoping{
		mode:             s.ClusterDiscoveryMode,
		dereferenceGrace: s.ClusterDereferenceGrace,
		referenceAhead:   s.ClusterReferenceAhead,
		claims:           collectEmissionClaims(policies),
	}
}

// Claims are the plugins' declared destinations, used both to decide whether a
// gateway can be scoped and to keep the claimed clusters.
func (c clusterScoping) Claims() emissionClaims { return c.claims }

// ReferenceAhead is the reference-ahead window, or 0 when CDS is not scoped:
// with every backend emitted, no cluster is new to a client.
func (c clusterScoping) ReferenceAhead() time.Duration {
	if !c.ScopesClusters() {
		return 0
	}
	return c.referenceAhead
}

// DereferenceGrace is how long a de-referenced cluster stays published; 0 when
// CDS is not scoped, where no cluster is ever de-referenced.
func (c clusterScoping) DereferenceGrace() time.Duration {
	if !c.ScopesClusters() {
		return 0
	}
	return c.dereferenceGrace
}

// ScopesClusters reports whether CDS and EDS are scoped to referenced clusters.
// False, the default, emits every backend in discovery scope.
func (c clusterScoping) ScopesClusters() bool {
	return c.mode == apisettings.ClusterDiscoveryReferenced
}

// emittedClustersFor returns the zero value when CDS is not scoped. Nothing
// consults it then, and skipping the full proto walk keeps it off the
// translation path. Otherwise claims with DestinationsFor decide each cluster
// specifier use here, once per gateway rebuild.
func emittedClustersFor(scoping clusterScoping, routes, listeners envoycache.Resources) emittedClusters {
	if !scoping.ScopesClusters() {
		return emittedClusters{}
	}
	emitted := collectReferencedClustersForEmission(routes, listeners)
	emitted.deriveClaims(scoping.Claims())
	return emitted
}

// scopedClusters returns the enabled configuration for tests; the zero value is
// disabled.
func scopedClusters() clusterScoping {
	return clusterScoping{mode: apisettings.ClusterDiscoveryReferenced}
}

// scopedClustersWithGrace is scopedClusters with a de-reference grace.
func scopedClustersWithGrace(grace time.Duration) clusterScoping {
	s := scopedClusters()
	s.dereferenceGrace = grace
	return s
}

// scopedClustersWithReferenceAhead is scopedClusters with a reference-ahead
// window, for tests.
func scopedClustersWithReferenceAhead(ahead time.Duration) clusterScoping {
	s := scopedClusters()
	s.referenceAhead = ahead
	return s
}
