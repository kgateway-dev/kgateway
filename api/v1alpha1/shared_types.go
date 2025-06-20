package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// Select the object to attach the policy by Group, Kind, and Name.
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

// Select the object to attach the policy by Group, Kind, Name and SectionName.
// The object must be in the same namespace as the policy.
// You can target only one object at a time.
type LocalPolicyTargetReferenceWithSectionName struct {
	LocalPolicyTargetReference `json:",inline"`

	// The section name of the target resource.
	// +optional
	SectionName *gwv1.SectionName `json:"sectionName,omitempty"`
}

// Select the object to attach the policy by Group, Kind, and its labels.
// The object must be in the same namespace as the policy and match the
// specified labels.
type LocalPolicyTargetSelector struct {
	// The API group of the target resource.
	// For Kubernetes Gateway API resources, the group is `gateway.networking.k8s.io`.
	Group gwv1.Group `json:"group"`

	// The API kind of the target resource,
	// such as Gateway or HTTPRoute.
	Kind gwv1.Kind `json:"kind"`

	// Label selector to select the target resource.
	MatchLabels map[string]string `json:"matchLabels"`
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

// Specifies the way to match a string.
// +kubebuilder:validation:XValidation:message="exactly one of Exact, Prefix, Suffix, Contains, or SafeRegex must be set",rule="[has(self.exact), has(self.prefix), has(self.suffix), has(self.contains), has(self.safeRegex)].filter(x, x).size() == 1"
type StringMatcher struct {
	// The input string must match exactly the string specified here.
	// Example: abc matches the value abc
	Exact *string `json:"exact,omitempty"`

	// The input string must have the prefix specified here.
	// Note: empty prefix is not allowed, please use regex instead.
	// Example: abc matches the value abc.xyz
	Prefix *string `json:"prefix,omitempty"`

	// The input string must have the suffix specified here.
	// Note: empty prefix is not allowed, please use regex instead.
	// Example: abc matches the value xyz.abc
	Suffix *string `json:"suffix,omitempty"`

	// The input string must contain the substring specified here.
	// Example: abc matches the value xyz.abc.def
	Contains *string `json:"contains,omitempty"`

	// The input string must match the Google RE2 regular expression specified here.
	// See https://github.com/google/re2/wiki/Syntax for the syntax.
	SafeRegex *string `json:"safeRegex,omitempty"`

	// If true, indicates the exact/prefix/suffix/contains matching should be
	// case insensitive. This has no effect on the regex match.
	// For example, the matcher data will match both input string Data and data if this
	// option is set to true.
	// +kubebuilder:default=false
	IgnoreCase bool `json:"ignoreCase"`
}
