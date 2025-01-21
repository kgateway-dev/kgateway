package admin

import (
	"fmt"

	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/rotisserie/eris"
)

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
			err = eris.New(fmt.Sprintf("panic occurred while getting xds snapshot: %v", r))
		}
	}()
	return xdsCache.GetSnapshot(k)
}
