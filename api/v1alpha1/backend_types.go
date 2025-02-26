package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=backends,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=backends/status,verbs=get;update;patch

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway,shortName=be
// +kubebuilder:subresource:status
type Backend struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackendSpec   `json:"spec,omitempty"`
	Status BackendStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type BackendList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Backend `json:"items"`
}

// +kubebuilder:validation:XValidation:message="There must one and only one backend type set",rule="(has(self.aws) && !has(self.static) && !has(self.ai)) || (!has(self.aws) && has(self.static) && !has(self.ai)) || (!has(self.aws) && !has(self.static) && has(self.ai))"
// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type BackendSpec struct {
	Aws    *AwsBackend    `json:"aws,omitempty"`
	Static *StaticBackend `json:"static,omitempty"`
	AI     *AIBackend     `json:"ai,omitempty"`
}
type AwsBackend struct {
	Region    string                      `json:"region,omitempty"`
	SecretRef corev1.LocalObjectReference `json:"secretRef,omitempty"`
}
type StaticBackend struct {
	Hosts []Host `json:"hosts,omitempty"`
}

type Host struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Host string          `json:"host"`
	Port gwv1.PortNumber `json:"port"`
}

type BackendStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
