package xds

import (
	"strings"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/types"

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

	// TLSSecretName is the name of the Kubernetes Secret containing the TLS certificate,
	// private key, and CA certificate for xDS communication. This secret must exist in the
	// kgateway installation namespace when TLS is enabled.
	TLSSecretName = "kgateway-xds-cert" //nolint:gosec // G101: This is a well-known xDS TLS secret name, not a credential

	// TLSCertPath is the path to the TLS certificate
	TLSCertPath = "/etc/xds-tls/tls.crt"

	// TLSKeyPath is the path to the TLS key
	TLSKeyPath = "/etc/xds-tls/tls.key"

	// TLSRootCAPath is the path to the TLS root CA
	TLSRootCAPath = "/etc/xds-tls/ca.crt"
)

func IsKubeGatewayCacheKey(key string) bool {
	return strings.HasPrefix(key, wellknown.GatewayApiProxyValue)
}

// OwnerNamespaceNameID returns the string identifier for an Envoy node in a provided namespace.
// Envoy proxies are assigned their configuration by kgateway based on their Node ID.
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

func AgentgatewayID(node *envoycorev3.Node) types.NamespacedName {
	if node.GetMetadata() != nil {
		roleValue := node.GetMetadata().GetFields()[RoleKey]
		if roleValue != nil {
			s := roleValue.GetStringValue()
			ns, name, ok := strings.Cut(s, KeyDelimiter)
			if ok {
				return types.NamespacedName{
					Namespace: ns,
					Name:      name,
				}
			}
		}
	}

	return types.NamespacedName{}
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
