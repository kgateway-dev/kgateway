package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// not sure why i need to copy this; codegen fails if i dont
type LocalPolicyTargetReference struct {
	// Group is the group of the target resource.
	Group gwv1.Group `json:"group"`

	// Kind is kind of the target resource.
	Kind gwv1.Kind `json:"kind"`

	// Name is the name of the target resource.
	Name gwv1.ObjectName `json:"name"`
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
