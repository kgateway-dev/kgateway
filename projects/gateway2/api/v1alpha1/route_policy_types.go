package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

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

// not sure why i need to copy this; codegen fails if i dont
type LocalPolicyTargetReference struct {
	// Group is the group of the target resource.
	Group gwv1.Group `json:"group"`

	// Kind is kind of the target resource.
	Kind gwv1.Kind `json:"kind"`

	// Name is the name of the target resource.
	Name gwv1.ObjectName `json:"name"`
}

type RoutePolicySpec struct {
	TargetRef LocalPolicyTargetReference `json:"targetRef,omitempty"`
	Timeout   int                        `json:"timeout,omitempty"`
}

type PolicyStatus struct {
	//
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +kubebuilder:validation:MaxItems=16
	Ancestors []PolicyAncestorStatus `json:"ancestors"`
}

type PolicyAncestorStatus struct {
	// AncestorRef corresponds with a ParentRef in the spec that this
	// PolicyAncestorStatus struct describes the status of.
	AncestorRef gwv1.ParentReference `json:"ancestorRef"`

	// ControllerName is a domain/path string that indicates the name of the
	// controller that wrote this status. This corresponds with the
	// controllerName field on GatewayClass.
	//
	// Example: "example.net/gateway-controller".
	//
	// The format of this field is DOMAIN "/" PATH, where DOMAIN and PATH are
	// valid Kubernetes names
	// (https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names).
	//
	// Controllers MUST populate this field when writing status. Controllers should ensure that
	// entries to status populated with their ControllerName are cleaned up when they are no
	// longer necessary.
	ControllerName string `json:"controllerName"`

	// Conditions describes the status of the Policy with respect to the given Ancestor.
	//
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

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
	Compress bool `json:"compress,omitempty"`
}

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
