package proxy_syncer

import (
	"fmt"
	"maps"
	"slices"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
)

// routingTargets are the clusters a snapshot's routes and listeners route to,
// as collectReferencedClusters defines them. The gate compares a deferred
// build's gaps with the routing targets of the snapshot it installed on the
// client's connection (flipBlockingClusters).
//
// They are a union of immutable parts, each computed once where the routes or
// listeners it describes are produced: one per gateway translation
// (GatewayXdsResources.ReferencedClusters) and one per backend listener
// (baseResource.routing). Clients built from the same translation share its
// parts, and composing a snapshot adds the parts of what it adds instead of
// rescanning what it keeps. Parts hold cluster names only, so recording the
// targets of a published snapshot never keeps its routes or listeners alive.
type routingTargets struct {
	parts []map[string]struct{}
}

// newRoutingTargets returns the union of parts. Callers must not modify the
// parts afterwards.
func newRoutingTargets(parts ...map[string]struct{}) routingTargets {
	return routingTargets{}.with(parts...)
}

// with returns t plus parts, leaving t unchanged. Empty parts are dropped.
func (t routingTargets) with(parts ...map[string]struct{}) routingTargets {
	// Clipped, the first append copies instead of writing into a backing array
	// another routingTargets shares.
	out := slices.Clip(t.parts)
	for _, part := range parts {
		if len(part) > 0 {
			out = append(out, part)
		}
	}
	return routingTargets{parts: out}
}

// union returns the targets of t and other.
func (t routingTargets) union(other routingTargets) routingTargets {
	return t.with(other.parts...)
}

// has reports whether name is a routing target.
func (t routingTargets) has(name string) bool {
	for _, part := range t.parts {
		if _, ok := part[name]; ok {
			return true
		}
	}
	return false
}

// names returns the targets as one new set.
func (t routingTargets) names() map[string]struct{} {
	out := make(map[string]struct{})
	for _, part := range t.parts {
		maps.Copy(out, part)
	}
	return out
}

// scannedRoutingTargets scans snap's routes and listeners. It is for snapshots
// whose producers recorded no parts; every snapshot the gate composes carries
// its own.
func scannedRoutingTargets(snap *envoycache.Snapshot) routingTargets {
	if snap == nil {
		return routingTargets{}
	}
	return newRoutingTargets(collectReferencedClusters(snap.Resources[envoycachetypes.Route], snap.Resources[envoycachetypes.Listener]))
}

// routingTargetsError reports how routing differs from a fresh scan of snap's
// routes and listeners, or nil if they agree.
func routingTargetsError(snap *envoycache.Snapshot, routing routingTargets) error {
	scanned := scannedRoutingTargets(snap).names()
	recorded := routing.names()
	if maps.Equal(scanned, recorded) {
		return nil
	}
	var unrecorded, stale []string
	for name := range scanned {
		if _, ok := recorded[name]; !ok {
			unrecorded = append(unrecorded, name)
		}
	}
	for name := range recorded {
		if _, ok := scanned[name]; !ok {
			stale = append(stale, name)
		}
	}
	slices.Sort(unrecorded)
	slices.Sort(stale)
	return fmt.Errorf("recorded routing targets differ from the snapshot's routes and listeners: unrecorded %v, not routed to %v", unrecorded, stale)
}

// routedSnapshot is a snapshot and the routing targets of its routes and
// listeners. The gate composes and installs the two together, so the targets
// it records for a connection describe the snapshot installed there.
type routedSnapshot struct {
	snap    *envoycache.Snapshot
	routing routingTargets
}
