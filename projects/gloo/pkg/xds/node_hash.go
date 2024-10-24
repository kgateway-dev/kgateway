package xds

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
)

var _ cache.NodeHash = new(nodeRoleHasher)

const (
	// FallbackNodeCacheKey is used to let nodes know they have a bad config
	// we assign a "fix me" snapshot for bad nodes
	FallbackNodeCacheKey = "misconfigured-node"

	// RoleKey is the name of the ket in the node.metadata used to store the role
	RoleKey = "role"
)

func NewNodeRoleHasher() *nodeRoleHasher {
	return &nodeRoleHasher{}
}

// nodeRoleHasher identifies a node based on the values provided in the `node.metadata.role`
type nodeRoleHasher struct{}

// ID returns the string value of the xDS cache key
// This value must match role metadata format: <owner>~<proxy_namespace>~<proxy_name>
// which is equal to role defined on proxy-deployment ConfigMap:
// gloo-kube-gateway-api~{{ $gateway.gatewayNamespace }}-{{ $gateway.gatewayName | default (include "gloo-gateway.gateway.fullname" .) }}
func (h *nodeRoleHasher) ID(node *envoy_config_core_v3.Node) string {
	if node.GetMetadata() != nil {
		roleValue := node.GetMetadata().GetFields()[RoleKey]
		if roleValue != nil {
			return roleValue.GetStringValue()
		}
	}

	return FallbackNodeCacheKey
}
