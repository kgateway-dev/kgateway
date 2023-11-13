package utils

import (
	"context"
	"fmt"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// SnapshotCacheKey returns the key used to identify a Proxy resource in a SnapshotCache
// This key must match the node.metadata.role of the Envoy Node
func SnapshotCacheKey(gw *apiv1.Gateway) string {
	return fmt.Sprintf("%v~%v", gw.Namespace, gw.Name)
}

// SnapshotCacheKeys returns a list with the SnapshotCacheKey for each Proxy
func SnapshotCacheKeys(gws []*apiv1.Gateway) []string {
	var keys []string
	// Get keys from gws
	for _, gw := range gws {
		// This is where we correlate Node ID with gw namespace~name
		keys = append(keys, SnapshotCacheKey(gw))
	}
	return keys
}

type NodeNameNsHasher struct{}

func (h *NodeNameNsHasher) ID(node *corev3.Node) string {
	if node.GetMetadata() != nil {
		gatewayValue := node.GetMetadata().GetFields()["gateway"].GetStructValue()
		if gatewayValue != nil {
			name := gatewayValue.GetFields()["name"]
			ns := gatewayValue.GetFields()["namespace"]
			if name != nil && ns != nil {
				return fmt.Sprintf("%v~%v", ns.GetStringValue(), name.GetStringValue())
			}
		}
	}

	return xds.FallbackNodeCacheKey
}

func NewAdsSnapshotCache(ctx context.Context) cache.SnapshotCache {
	settings := cache.CacheSettings{
		Ads:    true,
		Hash:   &NodeNameNsHasher{},
		Logger: contextutils.LoggerFrom(ctx),
	}
	return cache.NewSnapshotCache(settings)
}
