package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// MCP configures mcp backends
type MCP struct {
	// Name is the backend name for this MCP configuration.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Targets is a list of MCP targets to use for this backend.
	// +required
	// +kubebuilder:validation:MinItems=1
	Targets []McpTargetSelector `json:"targets"`
}

// McpTargetSelector defines the MCP target to use for this backend.
// +kubebuilder:validation:XValidation:message="exactly one of static or selectors must be set",rule="!(has(self.static) && has(self.selectors))"
type McpTargetSelector struct {
	// Selectors is the selector logic to search for MCP targets with the mcp app protocol.
	// +optional
	Selectors *McpSelector `json:"selectors,omitempty"`

	// StaticTarget is the MCP target to use for this backend.
	// +optional
	StaticTarget *McpTarget `json:"static,omitempty"`
}

// McpSelector defines the selector logic to search for MCP targets.
// +kubebuilder:validation:XValidation:message="at least one of namespaceSelector and serviceSelector must be set",rule="has(self.namespaceSelector) || has(self.serviceSelector)"
type McpSelector struct {
	// NamespaceSelector is the label selector in which namespace the MCP targets
	// are searched for.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// ServiceSelector is the label selector in which services the MCP targets
	// are searched for.
	// +optional
	ServiceSelector *metav1.LabelSelector `json:"serviceSelector,omitempty"`
}

// McpTarget defines a single MCP target configuration.
type McpTarget struct {
	// Name is the name of this MCP target.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Host is the hostname or IP address of the MCP target.
	// +required
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`

	// Path is the URL path of the MCP target endpoint.
	// Defaults to "/sse" for SSE protocol or "/mcp" for StreamableHTTP protocol if not specified.
	// +optional
	Path *string `json:"path"`

	// Port is the port number of the MCP target.
	// +required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// Protocol is the protocol to use for the connection to the MCP target.
	// +optional
	// +kubebuilder:validation:Enum=StreamableHTTP;SSE
	Protocol MCPProtocol `json:"protocol,omitempty"`
}

// MCPProtocol defines the protocol to use for the MCP target
type MCPProtocol string

const (

	// MCPProtocolStreamableHTTP specifies Streamable HTTP must be used as the protocol
	MCPProtocolStreamableHTTP MCPProtocol = "StreamableHTTP"

	// MCPProtocolSSE specifies Server-Sent Events (SSE) must be used as the protocol
	MCPProtocolSSE MCPProtocol = "SSE"
)
