package admin

import (
	"fmt"
	"maps"
	"net/http"

	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
)

// The xDS Snapshot is intended to return the full in-memory xDS cache that the Control Plane manages
// and serves up to running proxies.
func addXdsSnapshotHandler(path string, mux *http.ServeMux, profiles map[string]dynamicProfileDescription, cache cache.SnapshotCache) {
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if cache == nil {
			writeJSON(w, map[string]string{"error": "Envoy xDS cache not available (Envoy controller may be disabled)"}, r)
			return
		}
		response := getXdsSnapshotDataFromCache(cache)
		writeJSON(w, response, r)
	})
	profiles[path] = func() string { return "XDS Snapshot (Envoy only)" }
}

func getXdsSnapshotDataFromCache(xdsCache cache.SnapshotCache) SnapshotResponseData {
	cacheKeys := xdsCache.GetStatusKeys()
	cacheEntries := make(map[string]any, len(cacheKeys))

	for _, k := range cacheKeys {
		xdsSnapshot, err := getXdsSnapshot(xdsCache, k)
		if err != nil {
			cacheEntries[k] = err.Error()
		} else {
			cacheEntries[k] = xdsSnapshot
		}
	}

	return completeSnapshotResponse(cacheEntries)
}

func getXdsSnapshot(xdsCache cache.SnapshotCache, k string) (c cache.ResourceSnapshot, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic occurred while getting xds snapshot: %v", r)
		}
	}()
	snap, err := xdsCache.GetSnapshot(k)
	tmp, ok := snap.(*cache.Snapshot)
	if !ok {
		return nil, fmt.Errorf("invalid snapshot type; expected *cache.Snapshot, got %T", snap)
	}
	redacted := redactSecrets(tmp)
	return redacted, err
}

func redactSecrets(snap *cache.Snapshot) *cache.Snapshot {
	if snap == nil {
		return snap
	}
	resources := snap.Resources // Resources is an array, so this makes a copy
	// secrets is a struct and not a pointer, so modifications are safe
	secrets := resources[types.Secret]
	if len(secrets.Items) == 0 {
		return snap
	}

	// need to redact secrets, so create a new snapshot to avoid modifying the original
	snap = &cache.Snapshot{
		VersionMap: snap.VersionMap,
	}

	// avoid modifying the original resource map
	items := maps.Clone(secrets.Items)
	for key, res := range items {
		original, ok := res.Resource.(*envoytlsv3.Secret)
		if !ok {
			// should never happen
			continue
		}
		redacted := &envoytlsv3.Secret{
			Name: original.Name,
			// redact actual secret data
		}
		items[key] = types.ResourceWithTTL{
			Resource: redacted,
			TTL:      res.TTL,
		}
	}
	secrets.Items = items
	resources[types.Secret] = secrets
	snap.Resources = resources
	return snap
}
