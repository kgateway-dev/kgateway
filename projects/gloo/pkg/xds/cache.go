package xds

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
)

// KeyDelimiter is the character used to join segments of a cache key
const KeyDelimiter = "~"

func IsKubeGatewayCacheKey(key string) bool {
	return strings.HasPrefix(key, utils.GatewayApiProxyValue)
}

// OwnerNamespaceNameID returns the string identifier for an Envoy node in a provided namespace.
// Envoy proxies are assigned their configuration by Gloo based on their Node ID.
// Therefore, proxies must identify themselves using the same naming
// convention that we use to persist the Proxy resource in the snapshot cache.
// The naming convention that we follow is "OWNER~NAMESPACE~NAME"
func OwnerNamespaceNameID(owner, namespace, name string) string {
	return strings.Join([]string{owner, namespace, name}, KeyDelimiter)
}

// SnapshotCacheKey returns the key used to identify a Proxy resource in a SnapshotCache
func SnapshotCacheKey(proxy *v1.Proxy) string {
	namespace, name := proxy.GetMetadata().Ref().Strings()
	owner := proxy.GetMetadata().GetLabels()[utils.ProxyTypeKey]
	if owner == utils.GatewayApiProxyValue {
		// If namespace label is not set, default to proxy label for backwards compatability
		namespaceLabel := proxy.GetMetadata().GetLabels()[utils.GatewayNamespaceKey]
		if namespaceLabel != "" {
			namespace = namespaceLabel
		}
		return OwnerNamespaceNameID(owner, namespace, name)
	}

	return fmt.Sprintf("%v~%v", namespace, name)
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

// NewAdsSnapshotCache returns a snapshot-based cache, used to serve xDS requests
func NewAdsSnapshotCache(ctx context.Context) cache.SnapshotCache {
	settings := cache.CacheSettings{
		Ads:    true,
		Hash:   NewNodeRoleHasher(),
		Logger: contextutils.LoggerFrom(ctx),
	}
	return cache.NewSnapshotCache(settings)
}
