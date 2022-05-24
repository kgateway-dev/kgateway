package xds

import (
	"context"
	"fmt"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
)

// FallbackNodeCacheKey is used to let nodes know they have a bad config
// we assign a "fix me" snapshot for bad nodes
const FallbackNodeCacheKey = "misconfigured-node"

var (
	// Compile-time assertion
	_ cache.NodeHash = &nodeRoleHasher{}
)

// nodeRoleHasher returns the node.metadata.role or the FallbackNodeCacheKey if no role is defined
// Envoy proxies are assigned their configuration by Gloo based on their Node ID
// Therefore, proxies must identify themselves (using node.metadata.role) using the same naming
// convention that we use to persist the Proxy resource in the snapshot cache.
// The naming convention that we follow is "NAMESPACE~NAME"
// If none is provided, we provide the Fallback snapshot, as a way of alerting the user that their Envoy
// configuration is missing the expected role property
type nodeRoleHasher struct{}

func NewNodeRoleHasher() *nodeRoleHasher {
	return &nodeRoleHasher{}
}

func (h *nodeRoleHasher) ID(node *envoy_config_core_v3.Node) string {
	if node.GetMetadata() != nil {
		roleValue := node.GetMetadata().GetFields()["role"]
		if roleValue != nil {
			return roleValue.GetStringValue()
		}
	}

	return FallbackNodeCacheKey
}

// SnapshotCacheKey returns the key used to identify a Proxy resource in a SnapshotCache
// This key must match the node.metadata.role of the Envoy Node
func SnapshotCacheKey(proxy *v1.Proxy) string {
	namespace, name := proxy.GetMetadata().Ref().Strings()
	return fmt.Sprintf("%v~%v", namespace, name)
}

// SnapshotCacheKeys returns a list with the SnapshotCacheKey for each Proxy
func SnapshotCacheKeys(proxies v1.ProxyList) []string {
	var keys []string
	// Get keys from proxies
	for _, proxy := range proxies {
		// This is where we correlate Node ID with proxy namespace~name
		keys = append(keys, SnapshotCacheKey(proxy))
	}
	return keys
}

// NewAdsSnapshotCache returns a snapshot-based cache, used to serve xDS requests
func NewAdsSnapshotCache(ctx context.Context) cache.SnapshotCache {
	return cache.NewSnapshotCache(true, NewNodeRoleHasher(), contextutils.LoggerFrom(ctx))
}
