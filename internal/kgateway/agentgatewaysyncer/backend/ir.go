package agentgatewaybackend

import (
	"maps"

	"github.com/agentgateway/agentgateway/go/api"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
)

// AgentGatewayBackendIr is the internal representation of an agent gateway backend.
// This mirrors the envoy BackendIr pattern by pre-resolving all dependencies
// during collection building rather than at translation time.
type AgentGatewayBackendIr struct {
	StaticIr *StaticIr
	AIIr     *AIIr
	MCPIr    *MCPIr
	Errors   []error
}

func (u *AgentGatewayBackendIr) Equals(other any) bool {
	otherBackend, ok := other.(*AgentGatewayBackendIr)
	if !ok {
		return false
	}

	// Compare Static IR
	if !u.StaticIr.Equals(otherBackend.StaticIr) {
		return false
	}

	// Compare AI IR
	if !u.AIIr.Equals(otherBackend.AIIr) {
		return false
	}

	// Compare MCP IR
	if !u.MCPIr.Equals(otherBackend.MCPIr) {
		return false
	}

	// Compare Errors - simple string comparison
	if len(u.Errors) != len(otherBackend.Errors) {
		return false
	}
	for i, err := range u.Errors {
		if err.Error() != otherBackend.Errors[i].Error() {
			return false
		}
	}

	return true
}

// StaticIr contains pre-resolved data for static backends
type StaticIr struct {
	// Pre-resolved static backend configuration
	Backend *api.Backend
}

func (s *StaticIr) Equals(other *StaticIr) bool {
	if s == nil && other == nil {
		return true
	}
	if s == nil || other == nil {
		return false
	}

	// Use protobuf equality for api.Backend
	return proto.Equal(s.Backend, other.Backend)
}

// AIIr contains pre-resolved data for AI backends
type AIIr struct {
	Backend  *api.Backend
	Policies []*api.Policy
}

func (a *AIIr) Equals(other *AIIr) bool {
	if a == nil && other == nil {
		return true
	}
	if a == nil || other == nil {
		return false
	}

	// Compare Backend
	if !proto.Equal(a.Backend, other.Backend) {
		return false
	}

	// Compare Policies slice
	if len(a.Policies) != len(other.Policies) {
		return false
	}
	for i, policy := range a.Policies {
		if !proto.Equal(policy, other.Policies[i]) {
			return false
		}
	}

	return true
}

// MCPIr contains pre-resolved data for MCP backends
type MCPIr struct {
	// Pre-resolved MCP backend and any static backends it references
	Backends []*api.Backend
	// Pre-resolved service endpoints
	ServiceEndpoints map[string]*ServiceEndpoint
}

func (m *MCPIr) Equals(other *MCPIr) bool {
	if m == nil && other == nil {
		return true
	}
	if m == nil || other == nil {
		return false
	}

	// Compare Backends slice
	if len(m.Backends) != len(other.Backends) {
		return false
	}
	for i, backend := range m.Backends {
		if !proto.Equal(backend, other.Backends[i]) {
			return false
		}
	}

	// Compare ServiceEndpoints map
	if len(m.ServiceEndpoints) != len(other.ServiceEndpoints) {
		return false
	}
	for key, endpoint := range m.ServiceEndpoints {
		otherEndpoint, exists := other.ServiceEndpoints[key]
		if !exists || !endpoint.Equals(otherEndpoint) {
			return false
		}
	}

	return true
}

// ServiceEndpoint represents a resolved service endpoint
type ServiceEndpoint struct {
	Host      string
	Port      int32
	Service   *corev1.Service
	Namespace string
}

func (s *ServiceEndpoint) Equals(other *ServiceEndpoint) bool {
	if s == nil && other == nil {
		return true
	}
	if s == nil || other == nil {
		return false
	}

	// Compare primitive fields
	if s.Host != other.Host || s.Port != other.Port || s.Namespace != other.Namespace {
		return false
	}

	// Compare Service objects by comparing only relevant fields, not status
	if s.Service == nil && other.Service == nil {
		return true
	}
	if s.Service == nil || other.Service == nil {
		return false
	}

	// Compare ObjectMeta identity fields
	if s.Service.Name != other.Service.Name || s.Service.Namespace != other.Service.Namespace {
		return false
	}

	// Compare Spec fields that matter for service identity and behavior
	sSpec, oSpec := s.Service.Spec, other.Service.Spec

	// Compare ports
	if len(sSpec.Ports) != len(oSpec.Ports) {
		return false
	}
	for i, port := range sSpec.Ports {
		otherPort := oSpec.Ports[i]
		if port.Name != otherPort.Name ||
			port.Port != otherPort.Port ||
			port.Protocol != otherPort.Protocol ||
			port.TargetPort != otherPort.TargetPort {
			return false
		}
	}

	// Compare selector
	if !maps.Equal(sSpec.Selector, oSpec.Selector) {
		return false
	}

	if sSpec.Type != oSpec.Type {
		return false
	}

	if sSpec.ClusterIP != oSpec.ClusterIP {
		return false
	}

	return true
}
