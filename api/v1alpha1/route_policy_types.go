package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=routepolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=routepolicies/status,verbs=get;update;patch

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=gateway,app.kubernetes.io/name=gateway}
// +kubebuilder:resource:categories=kgateway,shortName=rp
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
	Timeout        int                  `json:"timeout,omitempty"`
	Transformation TransformationPolicy `json:"transformation,omitempty"`
}

type TransformationPolicy struct {
	// +optional
	Request *Transform `json:"request,omitempty"`
	// +optional
	Response *Transform `json:"response,omitempty"`
}

type Transform struct {

	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=16
	Set []HeaderTransformation `json:"set,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=16
	Add []HeaderTransformation `json:"add,omitempty"`

	// +optional
	// +listType=set
	// +kubebuilder:validation:MaxItems=16
	Remove []string `json:"remove,omitempty"`

	// +optional
	//
	// If empty, body will not be buffered.
	Body *BodyTransformation `json:"body,omitempty"`
}

type InjaTemplate string

type HeaderTransformation struct {
	Name  gwv1.HeaderName `json:"name,omitempty"`
	Value InjaTemplate    `json:"value,omitempty"`
}

// +kubebuilder:validation:Enum=AsString;AsJson
type BodyParseBehavior string

const (
	BodyParseBehaviorAsString BodyParseBehavior = "AsString"
	BodyParseBehaviorAsJSON   BodyParseBehavior = "AsJson"
)

type BodyTransformation struct {
	// +kubebuilder:default=AsString
	ParseAs BodyParseBehavior `json:"parseAs,omitempty"`
	Value   *InjaTemplate     `json:"value,omitempty"`
}
