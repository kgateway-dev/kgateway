package proxy_syncer

import (
	"strconv"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

// EDS set versioning.
//
// go-control-plane answers a watch only on a version change, so the EDS version
// must move exactly when the served assignment set does. A spurious move pushes
// identical endpoints to every client; a missed one leaves clients on stale
// endpoints. All publication paths use this function, digesting no protos:
//
//   - a built row contributes its EndpointsHash;
//   - a carried assignment contributes the hash it was published under, so
//     carrying does not move the version;
//   - a synthesized empty assignment contributes a hash of its name.
//
// The XOR fold is order-independent and matches the unfiltered version when
// every row is published. Hashes include the resource name, so contributions
// cancel, or different sets share a version, only under a 64-bit collision. An
// input-only change (such as a policy generation bump) still moves the version
// and pushes identical endpoints; avoiding that needs per-row content digests.

// endpointSetHashes returns each item's contribution: its row hash, else the
// carried hash (carried may be nil), else a hash of its name.
func endpointSetHashes(items map[string]envoycachetypes.ResourceWithTTL, rowHashes map[string]uint64, carried func(name string) (uint64, bool)) map[string]uint64 {
	out := make(map[string]uint64, len(items))
	for name := range items {
		if h, ok := rowHashes[name]; ok {
			out[name] = h
			continue
		}
		if carried != nil {
			if h, ok := carried(name); ok {
				out[name] = h
				continue
			}
		}
		out[name] = utils.HashString(name)
	}
	return out
}

// endpointSetVersion folds the contributions into the EDS version string.
func endpointSetVersion(hashes map[string]uint64) string {
	var v uint64
	for _, h := range hashes {
		v ^= h
	}
	return strconv.FormatUint(v, 10)
}

// versionEndpointSet returns res versioned by hashes.
func versionEndpointSet(res envoycache.Resources, hashes map[string]uint64) envoycache.Resources {
	return envoycache.Resources{Version: endpointSetVersion(hashes), Items: res.Items}
}
