package xds

import (
	"context"
	"strings"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
)

// SnapshotCacheKey returns the key used to identify a Proxy resource in a SnapshotCache
func SnapshotCacheKey(proxy *v1.Proxy) string {
	namespace, name := proxy.GetMetadata().Ref().Strings()
	owner := utils.GetTranslatorValue(proxy.GetMetadata())
	return OwnerNamespaceNameID(owner, namespace, name)
}

// SnapshotCacheKeys returns a list with the SnapshotCacheKey for each Proxy
func SnapshotCacheKeys(proxies v1.ProxyList) []string {
	var keys []string
	// Get keys from proxies
	for _, proxy := range proxies {
		// This is where we correlate Node ID with proxy owner~namespace~name
		keys = append(keys, SnapshotCacheKey(proxy))
	}
	return keys
}

// SnapshotBelongsTo returns true if the snapshot with the given cache key was created by the given
// owner (translator).
func SnapshotBelongsTo(key string, owner string) bool {
	return strings.HasPrefix(key, owner+"~")
}

// NewAdsSnapshotCache returns a snapshot-based cache, used to serve xDS requests
func NewAdsSnapshotCache(ctx context.Context) cache.SnapshotCache {
	settings := cache.CacheSettings{
		Ads:    true,
		Hash:   NewAggregateNodeHash(),
		Logger: contextutils.LoggerFrom(ctx),
	}
	return cache.NewSnapshotCache(settings)
}
