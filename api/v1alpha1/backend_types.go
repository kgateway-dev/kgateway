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

// BackendType indicates the type of the backend.
type BackendType string

const (
	// BackendTypeAI is the type for AI backends.
	BackendTypeAI BackendType = "ai"
	// BackendTypeAWS is the type for AWS backends.
	BackendTypeAWS BackendType = "aws"
	// BackendTypeStatic is the type for static backends.
	BackendTypeStatic BackendType = "static"
)

// BackendSpec defines the desired state of Backend.
// +union
// +kubebuilder:validation:XValidation:message="ai backend must be nil if the type is not 'ai'",rule="!(has(self.ai) && self.type != 'ai')"
// +kubebuilder:validation:XValidation:message="ai backend must be specified when type is 'ai'",rule="!(!has(self.ai) && self.type == 'ai')"
// +kubebuilder:validation:XValidation:message="aws backend must be nil if the type is not 'aws'",rule="!(has(self.aws) && self.type != 'aws')"
// +kubebuilder:validation:XValidation:message="aws backend must be specified when type is 'aws'",rule="!(!has(self.aws) && self.type == 'aws')"
// +kubebuilder:validation:XValidation:message="static backend must be nil if the type is not 'static'",rule="!(has(self.static) && self.type != 'static')"
// +kubebuilder:validation:XValidation:message="static backend must be specified when type is 'static'",rule="!(!has(self.static) && self.type == 'static')"
type BackendSpec struct {
	// Type indicates the type of the backend to be used.
	// +unionDiscriminator
	// +kubebuilder:validation:Enum=ai;aws;static
	// +kubebuilder:validation:Required
	Type BackendType `json:"type"`
	// AI is the AI backend configuration.
	// +optional
	AI *AIBackend `json:"ai,omitempty"`
	// Aws is the AWS backend configuration.
	// +optional
	Aws *AwsBackend `json:"aws,omitempty"`
	// Static is the static backend configuration.
	// +optional
	Static *StaticBackend `json:"static,omitempty"`
}

// AwsBackend is the AWS backend configuration.
type AwsBackend struct {
	// Region is the AWS region.
	// +optional
	Region string `json:"region,omitempty"`
	// SecretRef is the secret reference for the AWS credentials.
	// +optional
	SecretRef corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// StaticBackend is the static backend configuration.
type StaticBackend struct {
	// Hosts is the list of hosts.
	// +optional
	// +kubebuilder:validation:MinItems=1
	Hosts []Host `json:"hosts,omitempty"`
}

// Host is a host and port pair.
type Host struct {
	// Host is the host name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Host string `json:"host"`
	// Port is the port number.
	Port gwv1.PortNumber `json:"port"`
}

// BackendStatus defines the observed state of Backend.
type BackendStatus struct {
	// Conditions is the list of conditions for the backend.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type BackendList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Backend `json:"items"`
}
