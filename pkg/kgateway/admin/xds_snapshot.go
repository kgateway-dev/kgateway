package admin

import (
	"fmt"
	"maps"
	"net/http"

	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
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
	redacted := redactListenerCredentials(redactSecrets(tmp))
	return redacted, err
}

// redactListenerCredentials redacts CONNECT credentials without mutating the snapshot.
func redactListenerCredentials(snap *cache.Snapshot) *cache.Snapshot {
	return redactResources(snap, types.Listener, func(res types.Resource) types.Resource {
		if l, ok := res.(*envoylistenerv3.Listener); ok {
			return xds.RedactListenerCredentials(l)
		}
		return res
	})
}

func redactSecrets(snap *cache.Snapshot) *cache.Snapshot {
	return redactResources(snap, types.Secret, func(res types.Resource) types.Resource {
		if secret, ok := res.(*envoytlsv3.Secret); ok {
			return &envoytlsv3.Secret{Name: secret.Name}
		}
		return res
	})
}

// redactResources copies changed resources without mutating the live snapshot.
func redactResources(snap *cache.Snapshot, typ types.ResponseType, redact func(types.Resource) types.Resource) *cache.Snapshot {
	if snap == nil {
		return snap
	}
	resources := snap.Resources // Resources is an array, so this makes a copy
	typed := resources[typ]
	var items map[string]types.ResourceWithTTL
	for key, res := range typed.Items {
		redacted := redact(res.Resource)
		if redacted == res.Resource {
			continue
		}
		if items == nil {
			items = maps.Clone(typed.Items)
		}
		items[key] = types.ResourceWithTTL{Resource: redacted, TTL: res.TTL}
	}
	if items == nil {
		return snap
	}
	typed.Items = items
	resources[typ] = typed
	return &cache.Snapshot{Resources: resources, VersionMap: snap.VersionMap}
}
