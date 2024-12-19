package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:rbac:groups=gateway.gloo.solo.io,resources=httplistenerpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.gloo.solo.io,resources=httplistenerpolicies/status,verbs=get;update;patch

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=gateway,app.kubernetes.io/name=gateway}
// +kubebuilder:resource:categories=gateway,shortName=hlp
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels="gateway.networking.k8s.io/policy=Direct"
type HttpListenerPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HttpListenerPolicySpec `json:"spec,omitempty"`
	Status PolicyStatus           `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type HttpListenerPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HttpListenerPolicy `json:"items"`
}

type HttpListenerPolicySpec struct {
	TargetRef LocalPolicyTargetReference `json:"targetRef,omitempty"`
	Compress  bool                       `json:"compress,omitempty"`
}
