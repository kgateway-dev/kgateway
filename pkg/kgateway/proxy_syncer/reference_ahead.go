package proxy_syncer

import (
	"slices"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// newlyEmittedReferences returns the clusters this build both references and
// emits for the first time: present in the snapshot being published, named by
// its routes, and absent from what the client has already been sent.
//
// These are the additions that need the route update held back a beat. Putting
// the new cluster and the route that targets it in one coherent snapshot does
// not make Envoy apply CDS before RDS:
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
// The blackhole cluster is excluded: routes whose backends fail resolution
// target it, holding on it would delay exactly the configurations that are
// already broken, and it is emitted unconditionally anyway.
func newlyEmittedReferences(snapWrap XdsSnapWrapper, published envoycache.ResourceSnapshot) []string {
	if len(snapWrap.referencedClusters) == 0 {
		return nil
	}
	building := snapWrap.snap.Resources[envoycachetypes.Cluster].Items
	alreadySent := published.GetResourcesAndTTL(envoyresourcev3.ClusterType)

	var newly []string
	for name := range snapWrap.referencedClusters {
		if name == wellknown.BlackholeClusterName {
			continue
		}
		if _, sent := alreadySent[name]; sent {
			continue // the client already has it; a retarget onto it is safe now
		}
		if _, emitting := building[name]; !emitting {
			continue // absent from this build too: that is the missing-cluster
			// case, which the existing hold already covers
		}
		newly = append(newly, name)
	}
	slices.Sort(newly)
	return newly
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
