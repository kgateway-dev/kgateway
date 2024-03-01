package xds

import (
	"strings"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
)

var _ cache.NodeHash = new(classicEdgeNodeHash)
var _ cache.NodeHash = new(glooGatewayNodeHash)
var _ cache.NodeHash = new(aggregateNodeHash)

// FallbackNodeCacheKey is used to let nodes know they have a bad config
// we assign a "fix me" snapshot for bad nodes
const FallbackNodeCacheKey = "misconfigured-node"

// KeyDelimiter is the character used to join segments of a cache key
const KeyDelimiter = "~"

// OwnerNamespaceNameID returns the string identifier for an Envoy node in a provided namespace.
// Envoy proxies are assigned their configuration by Gloo based on their Node ID.
// Therefore, proxies must identify themselves using the same naming
// convention that we use to persist the Proxy resource in the snapshot cache.
// The naming convention that we follow is "OWNER~NAMESPACE~NAME"
func OwnerNamespaceNameID(owner, namespace, name string) string {
	return strings.Join([]string{owner, namespace, name}, KeyDelimiter)
}

func NewClassicEdgeNodeHash() *classicEdgeNodeHash {
	return &classicEdgeNodeHash{}
}

// classicEdgeNodeHash identifies a node based on the values provided in the `node.metadata.role`
type classicEdgeNodeHash struct{}

func (c classicEdgeNodeHash) ID(node *envoy_config_core_v3.Node) string {
	if node.GetMetadata() != nil {
		roleValue := node.GetMetadata().GetFields()["role"]
		if roleValue != nil {
			roleString := roleValue.GetStringValue()
			if c.isProxyWorkloadRole(roleString) {
				// Proxy workloads use a key that is prefixed by the translator that produced the xDS Snapshot
				return strings.Join([]string{utils.GlooEdgeTranslatorValue, roleString}, KeyDelimiter)
			} else {
				// Non-Proxy workloads use the exact key as the role
				return roleString
			}

		}
	}

	return FallbackNodeCacheKey
}

// isProxyWorkloadRole returns true if the provided role fits the format that is used by Proxy workloads
// This format is: "NAMESPACE~NAME".
// In classic Edge, nodes could also identify themselves with a node.metadata.role that do not fit this structure
// This is useful in the case of additional services (like Enterprise ext-auth-service, and rate-limit-service)
// who follow a format of "NAME".
func (c classicEdgeNodeHash) isProxyWorkloadRole(role string) bool {
	if strings.Contains(role, KeyDelimiter) {
		return true
	}
	return false
}

func NewGlooGatewayNodeHash() *glooGatewayNodeHash {
	return &glooGatewayNodeHash{}
}

// glooGatewayNodeHash identifies a node based on the values provided in the `node.metadata.gateway`
type glooGatewayNodeHash struct{}

func (g glooGatewayNodeHash) ID(node *envoy_config_core_v3.Node) string {
	if node.GetMetadata() != nil {
		gatewayFields := node.GetMetadata().GetFields()["gateway"].GetStructValue().GetFields()
		if gatewayFields != nil {
			return OwnerNamespaceNameID(
				utils.GlooGatewayTranslatorValue,
				gatewayFields["namespace"].GetStringValue(),
				gatewayFields["name"].GetStringValue())
		}
	}

	return FallbackNodeCacheKey
}

func NewAggregateNodeHash() *aggregateNodeHash {
	return &aggregateNodeHash{
		classicEdgeNodeHash: NewClassicEdgeNodeHash(),
		glooGatewayNodeHash: NewGlooGatewayNodeHash(),
	}
}

// aggregateNodeHash supports identifying a node by BOTH the classicEdgeNodeHash and the glooGatewayNodeHash
type aggregateNodeHash struct {
	*classicEdgeNodeHash
	*glooGatewayNodeHash
}

func (a aggregateNodeHash) ID(node *envoy_config_core_v3.Node) string {
	// As of Gloo Gateway 1.17, we are encouraging broader usage of the newer proxies, which integrate with the Kubernetes Gateway API
	// Therefore, for incoming nodes, we first attempt to process it as a "new" Gloo Gateway node, and then fallback to the "classic"
	// Gloo Edge implementation
	hash := a.glooGatewayNodeHash.ID(node)
	if hash != FallbackNodeCacheKey {
		return hash
	}

	return a.classicEdgeNodeHash.ID(node)
}
