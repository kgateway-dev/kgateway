package kgateway

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=httplistenerpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=httplistenerpolicies/status,verbs=get;update;patch

// +kubebuilder:printcolumn:name="Accepted",type=string,JSONPath=".status.ancestors[*].conditions[?(@.type=='Accepted')].status",description="HTTP listener policy acceptance status"
// +kubebuilder:printcolumn:name="Attached",type=string,JSONPath=".status.ancestors[*].conditions[?(@.type=='Attached')].status",description="HTTP listener policy attachment status"

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels="gateway.networking.k8s.io/policy=Direct"
// HTTPListenerPolicy is intended to be used for configuring the Envoy `HttpConnectionManager` and any other config or policy
// that should map 1-to-1 with a given HTTP listener, such as the Envoy health check HTTP filter.
// Currently these policies can only be applied per `Gateway` but support for `Listener` attachment may be added in the future.
// See https://github.com/kgateway-dev/kgateway/issues/11786 for more details.
// Deprecated: Use the httpSettings field on ListenerPolicy instead.
type HTTPListenerPolicy struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +required
	Spec HTTPListenerPolicySpec `json:"spec"`
	// +optional
	Status gwv1.PolicyStatus `json:"status,omitempty"`
	// TODO: embed this into a typed Status field when
	// https://github.com/kubernetes/kubernetes/issues/131533 is resolved
}

// +kubebuilder:object:root=true
type HTTPListenerPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HTTPListenerPolicy `json:"items"`
}

// HTTPListenerPolicySpec defines the desired state of a HTTP listener policy.
type HTTPListenerPolicySpec struct {
	// TargetRefs specifies the target resources by reference to attach the policy to.
	// +optional
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:XValidation:rule="self.all(r, r.kind == 'Gateway' && (!has(r.group) || r.group == 'gateway.networking.k8s.io'))",message="targetRefs may only reference Gateway resources"
	TargetRefs []shared.LocalPolicyTargetReference `json:"targetRefs,omitempty"`

	// TargetSelectors specifies the target selectors to select resources to attach the policy to.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.all(r, r.kind == 'Gateway' && (!has(r.group) || r.group == 'gateway.networking.k8s.io'))",message="targetSelectors may only reference Gateway resources"
	TargetSelectors []shared.LocalPolicyTargetSelector `json:"targetSelectors,omitempty"`

	HTTPSettings `json:",inline"`
}
