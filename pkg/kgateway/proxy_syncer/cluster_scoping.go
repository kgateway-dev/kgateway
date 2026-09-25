package proxy_syncer

import (
	"time"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
)

// clusterScoping is the whole referenced-only cluster discovery feature's
// configuration, resolved once from Settings.
//
// It exists so that "is this feature on" is one question with one answer,
// asked in the handful of places that would otherwise each re-derive it from
// the mode. Every path it touches -- collecting the emitted set, filtering CDS,
// and the transition windows that make the emitted set safe to change -- is
// guarded by ScopesClusters, and when that is false none of them run: no walk
// over the generated protos, no filtering, no per-client state, and the
// published snapshot is the one the syncer built.
type clusterScoping struct {
	mode apisettings.ClusterDiscoveryMode
	// dereferenceGrace is how long a cluster that has left the emitted set is
	// still published. Meaningless with scoping off, where no cluster ever
	// leaves the set, so it is read only through DereferenceGrace.
	dereferenceGrace time.Duration
}

func clusterScopingFrom(s apisettings.Settings) clusterScoping {
	return clusterScoping{
		mode:             s.ClusterDiscoveryMode,
		dereferenceGrace: s.ClusterDereferenceGrace,
	}
}

// DereferenceGrace is how long a de-referenced cluster stays published, and 0
// whenever CDS is not scoped: with every backend emitted unconditionally, no
// cluster is ever de-referenced, so there is nothing to hold on to and the gate
// keeps no state for it.
func (c clusterScoping) DereferenceGrace() time.Duration {
	if !c.ScopesClusters() {
		return 0
	}
	return c.dereferenceGrace
}

// ScopesClusters reports whether CDS and EDS are scoped to what the generated
// configuration references. False is the default, and means this proxy is sent
// a cluster for every backend in discovery scope exactly as it was before the
// feature existed.
func (c clusterScoping) ScopesClusters() bool {
	return c.mode == apisettings.ClusterDiscoveryReferenced
}

// emittedClustersFor computes the gateway's emitted-cluster set, or nothing at
// all when CDS is not scoped.
//
// The zero emittedClusters is not "no clusters are emitted": with scoping off
// nothing consults it, and the filter it feeds is itself skipped. Returning it
// here keeps the walk -- which visits every message of every generated listener
// and route, including typed_config payloads -- off the translation path
// entirely for deployments that have not opted in.
func emittedClustersFor(scoping clusterScoping, routes, listeners envoycache.Resources) emittedClusters {
	if !scoping.ScopesClusters() {
		return emittedClusters{}
	}
	return collectReferencedClustersForEmission(routes, listeners)
}

// scopedClusters is the enabled configuration, for tests that exercise the
// scoped paths directly. The zero clusterScoping is the disabled one.
func scopedClusters() clusterScoping {
	return clusterScoping{mode: apisettings.ClusterDiscoveryReferenced}
}

// scopedClustersWithGrace is the enabled configuration with a de-reference
// window, for tests that exercise the removal transition.
func scopedClustersWithGrace(grace time.Duration) clusterScoping {
	s := scopedClusters()
	s.dereferenceGrace = grace
	return s
}
