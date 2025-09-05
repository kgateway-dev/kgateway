package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// MCP configures mcp backends
type MCP struct {
	// Targets is a list of MCP targets to use for this backend.
	// Policies targeting MCP targets must use targetRefs[].sectionName
	// to select the target by name.
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	Targets []McpTargetSelector `json:"targets"`
}

// McpTargetSelector defines the MCP target to use for this backend.
// +kubebuilder:validation:ExactlyOneOf=selector;static
type McpTargetSelector struct {
	// Name of the MCP target.
	Name string `json:"name"`

	// Selector is the selector to use to select the MCP targets.
	// Note: Policies must target the resource selected by the target and
	// not the name of the selector-based target on the Backend resource.
	// +optional
	Selector *McpSelector `json:"selector,omitempty"`

	// Static is the static MCP target to use.
	// Policies can target static backends by targeting the Backend resource
	// and using sectionName to target the specific static target by name.
	// +optional
	Static *McpTarget `json:"static,omitempty"`
}

// McpSelector defines the selector logic to search for MCP targets.
// +kubebuilder:validation:XValidation:message="at least one of namespace or service must be set",rule="has(self.__namespace__) || has(self.service)"
type McpSelector struct {
	// Namespace is the label selector in which namespace the MCP targets
	// are searched for.
	// +optional
	Namespace *metav1.LabelSelector `json:"namespace,omitempty"`

	// Service is the label selector in which services the MCP targets
	// are searched for.
	// +optional
	Service *metav1.LabelSelector `json:"service,omitempty"`
}

// McpTarget defines a single MCP target configuration.
type McpTarget struct {
	// Host is the hostname or IP address of the MCP target.
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`

	// Port is the port number of the MCP target.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// Path is the URL path of the MCP target endpoint.
	// Defaults to "/sse" for SSE protocol or "/mcp" for StreamableHTTP protocol if not specified.
	// +optional
	Path *string `json:"path,omitempty"`

	// Protocol is the protocol to use for the connection to the MCP target.
	// +optional
	// +kubebuilder:validation:Enum=StreamableHTTP;SSE
	Protocol *MCPProtocol `json:"protocol,omitempty"`
}

// MCPProtocol defines the protocol to use for the MCP target
type MCPProtocol string

const (

	// MCPProtocolStreamableHTTP specifies Streamable HTTP must be used as the protocol
	MCPProtocolStreamableHTTP MCPProtocol = "StreamableHTTP"

	// MCPProtocolSSE specifies Server-Sent Events (SSE) must be used as the protocol
	MCPProtocolSSE MCPProtocol = "SSE"
)
