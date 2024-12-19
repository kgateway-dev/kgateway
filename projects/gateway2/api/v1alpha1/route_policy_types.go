package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:rbac:groups=gateway.gloo.solo.io,resources=routepolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.gloo.solo.io,resources=routepolicies/status,verbs=get;update;patch

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=gateway,app.kubernetes.io/name=gateway}
// +kubebuilder:resource:categories=gateway,shortName=rp
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels="gateway.networking.k8s.io/policy=Direct"
type RoutePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RoutePolicySpec `json:"spec,omitempty"`
	Status PolicyStatus    `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type RoutePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RoutePolicy `json:"items"`
}

type RoutePolicySpec struct {
	TargetRef LocalPolicyTargetReference `json:"targetRef,omitempty"`
	// +kubebuilder:validation:Minimum=1
	Timeout int `json:"timeout,omitempty"`
}
