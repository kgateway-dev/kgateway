package admin

import (
	"fmt"
	"net/http"

	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
)

// The xDS Snapshot is intended to return the full in-memory xDS cache that the Control Plane manages
// and serves up to running proxies.
func addXdsSnapshotHandler(path string, mux *http.ServeMux, profiles map[string]dynamicProfileDescription, cache cache.SnapshotCache) {
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		response := getXdsSnapshotDataFromCache(cache)
		writeJSON(w, response, r)
	})
	profiles[path] = func() string { return "XDS Snapshot" }
}

func getXdsSnapshotDataFromCache(xdsCache cache.SnapshotCache) SnapshotResponseData {
	cacheKeys := xdsCache.GetStatusKeys()
	cacheEntries := make(map[string]interface{}, len(cacheKeys))

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

func getXdsSnapshot(xdsCache cache.SnapshotCache, k string) (cache cache.ResourceSnapshot, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic occurred while getting xds snapshot: %v", r)
		}
	}()
	return xdsCache.GetSnapshot(k)
}
