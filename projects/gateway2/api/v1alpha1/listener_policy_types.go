package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:rbac:groups=gateway.gloo.solo.io,resources=listenerpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.gloo.solo.io,resources=listenerpolicies/status,verbs=get;update;patch

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=gateway,app.kubernetes.io/name=gateway}
// +kubebuilder:resource:categories=gateway,shortName=lp
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels="gateway.networking.k8s.io/policy=Direct"
type ListenerPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ListenerPolicySpec `json:"spec,omitempty"`
	Status PolicyStatus       `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ListenerPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ListenerPolicy `json:"items"`
}

type ListenerPolicySpec struct {
	TargetRef                     LocalPolicyTargetReference `json:"targetRef,omitempty"`
	PerConnectionBufferLimitBytes uint32                     `json:"perConnectionBufferLimitBytes,omitempty"`
}
