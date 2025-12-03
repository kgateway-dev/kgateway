package kgateway

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=listenerpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=listenerpolicies/status,verbs=get;update;patch

// +kubebuilder:printcolumn:name="Accepted",type=string,JSONPath=".status.ancestors[*].conditions[?(@.type=='Accepted')].status",description="Listener policy acceptance status"
// +kubebuilder:printcolumn:name="Attached",type=string,JSONPath=".status.ancestors[*].conditions[?(@.type=='Attached')].status",description="Listener policy attachment status"

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels="gateway.networking.k8s.io/policy=Direct"
// ListenerPolicy is used for configuring Envoy listener-level settings that apply to all protocol types (HTTP, HTTPS, TCP, TLS).
// These policies can only target `Gateway` objects.
type ListenerPolicy struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +required
	Spec ListenerPolicySpec `json:"spec"`
	// +optional
	Status gwv1.PolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ListenerPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ListenerPolicy `json:"items"`
}

// ListenerPolicySpec defines the desired state of a listener policy.
type ListenerPolicySpec struct {
	// TargetRefs specifies the target resources by reference to attach the policy to.
	// Only supports `Gateway` resources
	// +optional
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:XValidation:rule="self.all(r, r.kind == 'Gateway' && (!has(r.group) || r.group == 'gateway.networking.k8s.io'))",message="targetRefs may only reference Gateway resource"
	TargetRefs []shared.LocalPolicyTargetReference `json:"targetRefs,omitempty"`

	// TargetSelectors specifies the target selectors to select `Gateway` resources to attach the policy to.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.all(r, r.kind == 'Gateway' && (!has(r.group) || r.group == 'gateway.networking.k8s.io'))",message="targetSelectors may only reference Gateway resource"
	TargetSelectors []shared.LocalPolicyTargetSelector `json:"targetSelectors,omitempty"`

	// ProxyProtocol configures the PROXY protocol listener filter.
	// When set, Envoy will expect connections to include the PROXY protocol header.
	// This is commonly used when kgateway is behind a load balancer that preserves client IP information.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/listener/proxy_protocol/v3/proxy_protocol.proto
	// +optional
	ProxyProtocol *ProxyProtocolConfig `json:"proxyProtocol,omitempty"`

	// PerConnectionBufferLimitBytes sets the per-connection buffer limit for all listeners on the gateway.
	// This controls the maximum size of read and write buffers for new connections.
	// When using Envoy as an edge proxy, configuring the listener buffer limit is important to guard against
	// potential attacks or misconfigured downstreams that could hog the proxy's resources.
	// If unspecified, an implementation-defined default is applied (1MiB).
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/listener/v3/listener.proto#envoy-v3-api-field-config-listener-v3-listener-per-connection-buffer-limit-bytes
	// +optional
	// +kubebuilder:validation:Minimum=0
	PerConnectionBufferLimitBytes *int32 `json:"perConnectionBufferLimitBytes,omitempty"`
}

// ProxyProtocolConfig configures the PROXY protocol listener filter.
// The presence of this configuration enables PROXY protocol support.
type ProxyProtocolConfig struct {
}
