package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// +kubebuilder:rbac:groups=gateway.gloo.solo.io,resources=upstreams,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.gloo.solo.io,resources=upstreams/status,verbs=get;update;patch

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=gateway,app.kubernetes.io/name=gateway}
// +kubebuilder:resource:categories=gateway,shortName=up
// +kubebuilder:subresource:status
type Upstream struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UpstreamSpec   `json:"spec,omitempty"`
	Status UpstreamStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type UpstreamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Upstream `json:"items"`
}

// +kubebuilder:validation:XValidation:message="There must one and only one upstream type set",rule="1 == (self.aws != null?1:0) + (self.static != null?1:0)"
type UpstreamSpec struct {
	Aws    *AwsUpstream    `json:"aws,omitempty"`
	Static *StaticUpstream `json:"static,omitempty"`
}
type AwsUpstream struct {
	Region    string                      `json:"region,omitempty"`
	SecretRef corev1.LocalObjectReference `json:"secretRef,omitempty"`
}
type StaticUpstream struct {
	Hosts []Host `json:"hosts,omitempty"`
}

type Host struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Host string          `json:"host"`
	Port gwv1.PortNumber `json:"port"`
}

type UpstreamStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
