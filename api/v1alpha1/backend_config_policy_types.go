package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=backendconfigpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=backendconfigpolicies/status,verbs=get;update;patch

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway
// +kubebuilder:subresource:status
type BackendConfigPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BackendConfigPolicySpec   `json:"spec,omitempty"`
	Status            BackendConfigPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type BackendConfigPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackendConfigPolicy `json:"items"`
}

type BackendConfigPolicySpec struct {
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	TargetRefs []LocalPolicyTargetReference `json:"targetRefs,omitempty"`

	// +optional
	MaxRequestsPerConnection *int `json:"maxRequestsPerConnection,omitempty"`
}

type BackendConfigPolicyStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
