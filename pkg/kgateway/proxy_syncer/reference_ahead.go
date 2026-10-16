package proxy_syncer

import (
	"slices"
	"time"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// referenceAheadLocked returns the clusters this build emits that the client
// has not yet had for a whole reference-ahead window, and how long until the
// last of those windows closes.
//
// These are the additions that need the route update held back a beat. Putting
// a new cluster and the configuration that names it in one coherent snapshot
// does not make Envoy apply CDS before RDS or LDS:
//
//   - On a quiet stream the ads=true cache type-sorts its pushes, so CDS
//     usually reaches the wire first. "Usually" is not a guarantee: the
//     non-ordered server's drain randomizes whenever several per-type channels
//     are ready at once, which is what a busy stream looks like.
//   - ACK skew defeats both server modes outright. After a CDS response is
//     sent its watch is closed until Envoy ACKs; a snapshot landing in that
//     window can only be answered on the still-open RDS watch, so the route
//     reaches the wire before any CDS carrying its destination. No server
//     option closes this, and it is reachable whenever a route is retargeted
//     while an earlier CDS update is un-ACKed.
//
// Emit-all was immune to both, because destinations were delivered and ACKed
// long before any route named them. Referenced-only gives that immunity up, so
// it has to be replaced by holding the route until the cluster has had time to
// land.
//
// A cluster's window starts when it is first sent to this client and is
// tracked per cluster. Being in the published snapshot only means the cluster
// was installed for delivery, not that Envoy has applied it, so a rebuild
// inside the window (the new backend's own endpoints arriving, say) keeps
// holding instead of releasing the routes early. Every cluster new to the
// client counts, not only route targets: a listener that starts calling a new
// ext_authz, ext_proc, rate limit or JWKS cluster needs it delivered first just
// as much, and in REFERENCED mode every emitted cluster is named by something.
//
// The blackhole cluster is excluded: routes whose backends fail resolution
// target it, holding on it would delay exactly the configurations that are
// already broken, and it is emitted unconditionally anyway. A cluster absent
// from this build is the deferred path's missing-cluster case, held there.
// Callers must hold g.mu.
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
	// Forget clusters whose window closed, or that this build stopped emitting
	// (a return later is a fresh addition).
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
				continue // the client has had it for longer than a window
			}
			since = now // sent for the first time by this publish
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

// holdsReferenceAhead reports whether the addition-side hold is configured.
// False leaves every publication path exactly as it was, which is what an
// operator gets by setting the window to 0 and accepting the retarget blip.
func (g *publishGate) holdsReferenceAhead() bool { return g.referenceAhead > 0 }

// holdRoutingTypes returns snap with its routes, listeners and secrets pinned
// at the versions the client already has, leaving clusters and endpoints as
// built. That is the shape of every hold in this package: the new destination
// is delivered, the configuration that would start using it is not, yet.
//
// The pinned types are held together. Publishing new listeners against
// published routes, or either against published secrets, would be a
// combination the client has never been sent, and Snapshot.Consistent()
// requires RDS to match what LDS names.
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
// configured. Both need the same coherent publish path, because both
// transitions -- a cluster losing its last reference, and a route gaining a
// destination the client has never seen -- produce builds with nothing missing,
// so neither reaches the deferred resolution that handles unready clusters.
func (g *publishGate) appliesTransitionGraces() bool {
	return g.retainsDereferenced() || g.holdsReferenceAhead()
}
