package xds

import (
	"strings"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

var _ cache.NodeHash = new(nodeRoleHasher)

const (
	// KeyDelimiter is the character used to join segments of a cache key
	KeyDelimiter = "~"

	// RoleKey is the name of the ket in the node.metadata used to store the role
	RoleKey = "role"

	// PeerCtxKey is the key used to store the peer information in the context
	PeerCtxKey = "peer"

	// FallbackNodeCacheKey is used to let nodes know they have a bad config
	// we assign a "fix me" snapshot for bad nodes
	FallbackNodeCacheKey = "misconfigured-node"
)

func IsKubeGatewayCacheKey(key string) bool {
	return strings.HasPrefix(key, wellknown.GatewayApiProxyValue)
}

// OwnerNamespaceNameID returns the string identifier for an Envoy node in a provided namespace.
// Envoy proxies are assigned their configuration by Gloo based on their Node ID.
// Therefore, proxies must identify themselves using the same naming
// convention that we use to persist the Proxy resource in the snapshot cache.
// The naming convention that we follow is "OWNER~NAMESPACE~NAME"
func OwnerNamespaceNameID(owner, namespace, name string) string {
	return strings.Join([]string{owner, namespace, name}, KeyDelimiter)
}

func NewNodeRoleHasher() *nodeRoleHasher {
	return &nodeRoleHasher{}
}

// nodeRoleHasher identifies a node based on the values provided in the `node.metadata.role`
type nodeRoleHasher struct{}

// ID returns the string value of the xDS cache key
// This value must match role metadata format: <owner>~<proxy_namespace>~<proxy_name>
// which is equal to role defined on proxy-deployment ConfigMap:
// kgateway-kube-gateway-api~{{ $gateway.gatewayNamespace }}-{{ $gateway.gatewayName | default (include "kgateway.gateway.fullname" .) }}
func (h *nodeRoleHasher) ID(node *envoycorev3.Node) string {
	if node.GetMetadata() != nil {
		roleValue := node.GetMetadata().GetFields()[RoleKey]
		if roleValue != nil {
			return roleValue.GetStringValue()
		}
	}

	return FallbackNodeCacheKey
}

func CloneSnap(snap *cache.Snapshot) *cache.Snapshot {
	s := &cache.Snapshot{}
	for k, v := range snap.Resources {
		s.Resources[k].Version = v.Version
		items := map[string]envoycachetypes.ResourceWithTTL{}
		s.Resources[k].Items = items
		for a, b := range v.Items {
			b.Resource = proto.Clone(b.Resource)
			items[a] = b
		}
	}
	return s
}
