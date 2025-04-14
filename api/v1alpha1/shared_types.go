package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// Select the object to attach the policy to.
// The object must be in the same namespace as the policy.
// You can target only one object at a time.
type LocalPolicyTargetReference struct {
	// The API group of the target resource.
	// For Kubernetes Gateway API resources, the group is `gateway.networking.k8s.io`.
	Group gwv1.Group `json:"group"`

	// The API kind of the target resource,
	// such as Gateway or HTTPRoute.
	Kind gwv1.Kind `json:"kind"`

	// The name of the target resource.
	Name gwv1.ObjectName `json:"name"`
}

// LocalPolicyTargetReferenceWithSectionName identifies an API object to apply a
// direct policy to. This should be used as part of Policy resources that can
// target single resources. For more information on how this policy attachment
// mode works, and a sample Policy resource, refer to the policy attachment
// documentation for Gateway API.
//
// Note: This should only be used for direct policy attachment when references
// to SectionName are actually needed. In all other cases,
// LocalPolicyTargetReference should be used.
type LocalPolicyTargetReferenceWithSectionName struct {
	LocalPolicyTargetReference `json:",inline"`

	// SectionName is the name of a section within the target resource. When
	// unspecified, this targetRef targets the entire resource. In the following
	// resources, SectionName is interpreted as the following:
	//
	// * Gateway: Listener name
	// * HTTPRoute: HTTPRouteRule name
	// * Service: Port name
	//
	// If a SectionName is specified, but does not exist on the targeted object,
	// the Policy must fail to attach, and the policy implementation should record
	// a `ResolvedRefs` or similar Condition in the Policy's status.
	//
	// +optional
	SectionName *gwv1.SectionName `json:"sectionName,omitempty"`
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
