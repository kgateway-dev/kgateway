package proxy_syncer

import (
	"maps"
	"slices"
	"time"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// referenceAheadLocked returns the clusters this build emits that the client
// has not had for a whole reference-ahead window, and how long until the last
// window closes. Callers must hold g.mu.
//
// One coherent snapshot does not make Envoy apply CDS before RDS or LDS:
//
//   - The ads=true cache type-sorts pushes, but the non-ordered server drains
//     ready per-type channels in random order on a busy stream.
//   - In either server mode, a sent CDS response closes its watch until Envoy
//     ACKs, so a snapshot landing then goes out on the open RDS watch first.
//     Any retarget during an un-ACKed CDS update hits this, and no server
//     option prevents it.
//
// ALL mode is immune because destinations are sent long before routes name
// them.
//
// Each cluster's window starts at its first send. Being in the published
// snapshot means installed, not applied, so a rebuild inside the window keeps
// holding. Every new cluster counts, including ones only a filter calls
// (ext_authz, ext_proc, rate limit, JWKS).
//
// The blackhole cluster is excluded: it is always emitted, and holding on it
// would delay only configuration that is already broken. A cluster absent from
// the build is the deferred path's missing-cluster case.
func (g *publishGate) referenceAheadLocked(
	proxyKey string,
	snapWrap XdsSnapWrapper,
	published envoycache.ResourceSnapshot,
	now time.Time,
) ([]string, time.Duration) {
	if g.referenceAhead <= 0 {
		return nil, 0
	}
	building := snapWrap.snap.Resources[envoycachetypes.Cluster].Items
	alreadySent := published.GetResourcesAndTTL(envoyresourcev3.ClusterType)
	sent := g.aheadSent[proxyKey]
	// Forget closed windows and no-longer-emitted clusters; a later return is
	// new.
	for name, since := range sent {
		if _, emitting := building[name]; !emitting || now.Sub(since) >= g.referenceAhead {
			delete(sent, name)
		}
	}

	var ahead []string
	var longest time.Duration
	for name := range building {
		if name == wellknown.BlackholeClusterName {
			continue
		}
		since, inWindow := sent[name]
		if !inWindow {
			if _, has := alreadySent[name]; has {
				continue // already published, no open window
			}
			since = now // first published by this build
			if sent == nil {
				sent = make(map[string]time.Time)
				g.aheadSent[proxyKey] = sent
			}
			sent[name] = since
		}
		ahead = append(ahead, name)
		longest = max(longest, g.referenceAhead-now.Sub(since))
	}
	if len(sent) == 0 {
		delete(g.aheadSent, proxyKey)
	}
	slices.Sort(ahead)
	return ahead, longest
}

// holdsReferenceAhead reports whether the reference-ahead hold is configured.
// With 0, a new cluster and the configuration naming it publish together.
func (g *publishGate) holdsReferenceAhead() bool { return g.referenceAhead > 0 }

// referenceAheadHoldLocked composes a reference-ahead hold: the build's
// clusters and endpoints go out while routes, listeners and secrets stay at
// their published versions. As in the deferred hold, every published cluster
// the build lacks is carried. Otherwise a build that also drops a cluster would
// remove it from under the held configuration, covered only by a de-reference
// grace that may be 0. Errored clusters are never carried. Callers must hold
// g.mu.
func (g *publishGate) referenceAheadHoldLocked(
	snapWrap XdsSnapWrapper,
	published envoycache.ResourceSnapshot,
	graced map[string]struct{},
) (*envoycache.Snapshot, map[string]uint64) {
	closure := make(map[string]struct{}, len(graced))
	maps.Copy(closure, graced)
	for name := range published.GetResourcesAndTTL(envoyresourcev3.ClusterType) {
		closure[name] = struct{}{}
	}
	resolved, _, edsHashes := resolveDeferredPerCluster(snapWrap, published, false, closure, g.lastPublishedEDSLocked(snapWrap.proxyKey))
	return holdRoutingTypes(resolved, published), edsHashes
}

// holdRoutingTypes returns snap with routes, listeners and secrets pinned at
// their published versions and clusters and endpoints as built, the shape of
// every hold here. The three are pinned together: mixing builds would send a
// combination the client never had, and Snapshot.Consistent() requires RDS to
// match LDS.
func holdRoutingTypes(snap *envoycache.Snapshot, published envoycache.ResourceSnapshot) *envoycache.Snapshot {
	held := &envoycache.Snapshot{}
	*held = *snap
	for _, t := range []struct {
		rt      envoycachetypes.ResponseType
		typeURL envoyresourcev3.Type
	}{
		{envoycachetypes.Route, envoyresourcev3.RouteType},
		{envoycachetypes.Listener, envoyresourcev3.ListenerType},
		{envoycachetypes.Secret, envoyresourcev3.SecretType},
	} {
		held.Resources[t.rt] = envoycache.Resources{
			Version: published.GetVersion(t.typeURL),
			Items:   published.GetResourcesAndTTL(t.typeURL),
		}
	}
	return held
}

// appliesTransitionGraces reports whether either transition window is
// configured, so coherent builds need publishWithTransitionGraces.
func (g *publishGate) appliesTransitionGraces() bool {
	return g.retainsDereferenced() || g.holdsReferenceAhead()
}
