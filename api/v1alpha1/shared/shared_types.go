package shared

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// Control-plane Authorization rules not specific to policies:
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create

// Select the object by Name and Namespace.
// You can target only one object at a time.
type NamespacedObjectReference struct {
	// The name of the target resource.
	// +required
	Name gwv1.ObjectName `json:"name"`

	// The namespace of the target resource.
	// If not set, defaults to the namespace of the parent object.
	// +optional
	Namespace *gwv1.Namespace `json:"namespace,omitempty"`
}

// Select the object to attach the policy by Group, Kind, and Name.
// The object must be in the same namespace as the policy.
// You can target only one object at a time.
type LocalPolicyTargetReference struct {
	// The API group of the target resource.
	// For Kubernetes Gateway API resources, the group is `gateway.networking.k8s.io`.
	// +required
	Group gwv1.Group `json:"group"`

	// The API kind of the target resource,
	// such as Gateway or HTTPRoute.
	// +required
	Kind gwv1.Kind `json:"kind"`

	// The name of the target resource.
	// +required
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

// LocalPolicyTargetSelector selects the object to attach the policy by Group, Kind, and MatchLabels.
// The object must be in the same namespace as the policy and match the
// specified labels.
// Do not use targetSelectors when reconciliation times are critical, especially if you
// have a large number of policies that target the same resource.
// Instead, use targetRefs to attach the policy.
type LocalPolicyTargetSelector struct {
	// The API group of the target resource.
	// For Kubernetes Gateway API resources, the group is `gateway.networking.k8s.io`.
	// +required
	Group gwv1.Group `json:"group"`

	// The API kind of the target resource,
	// such as Gateway or HTTPRoute.
	// +required
	Kind gwv1.Kind `json:"kind"`

	// Label selector to select the target resource.
	// +required
	MatchLabels map[string]string `json:"matchLabels"`
}

// LocalPolicyTargetSelectorWithSectionName the object to attach the policy by Group, Kind, MatchLabels, and optionally SectionName.
// The object must be in the same namespace as the policy and match the
// specified labels.
// Do not use targetSelectors when reconciliation times are critical, especially if you
// have a large number of policies that target the same resource.
// Instead, use targetRefs to attach the policy.
type LocalPolicyTargetSelectorWithSectionName struct {
	LocalPolicyTargetSelector `json:",inline"`

	// The section name of the target resource.
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
	// +required
	Ancestors []PolicyAncestorStatus `json:"ancestors"`
}

type PolicyAncestorStatus struct {
	// AncestorRef corresponds with a ParentRef in the spec that this
	// PolicyAncestorStatus struct describes the status of.
	// +required
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
	// +required
	ControllerName string `json:"controllerName"`

	// Conditions describes the status of the Policy with respect to the given Ancestor.
	//
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// Specifies the way to match a string.
// +kubebuilder:validation:ExactlyOneOf=exact;prefix;suffix;contains;safeRegex
type StringMatcher struct {
	// The input string must match exactly the string specified here.
	// Example: abc matches the value abc
	// +optional
	Exact *string `json:"exact,omitempty"`

	// The input string must have the prefix specified here.
	// Note: empty prefix is not allowed, please use regex instead.
	// Example: abc matches the value abc.xyz
	// +optional
	Prefix *string `json:"prefix,omitempty"`

	// The input string must have the suffix specified here.
	// Note: empty prefix is not allowed, please use regex instead.
	// Example: abc matches the value xyz.abc
	// +optional
	Suffix *string `json:"suffix,omitempty"`

	// The input string must contain the substring specified here.
	// Example: abc matches the value xyz.abc.def
	// +optional
	Contains *string `json:"contains,omitempty"`

	// The input string must match the Google RE2 regular expression specified here.
	// See https://github.com/google/re2/wiki/Syntax for the syntax.
	// +optional
	SafeRegex *string `json:"safeRegex,omitempty"`

	// If true, indicates the exact/prefix/suffix/contains matching should be
	// case insensitive. This has no effect on the regex match.
	// For example, the matcher data will match both input string Data and data if this
	// option is set to true.
	// +optional
	IgnoreCase *bool `json:"ignoreCase,omitempty"`
}

// HeaderModifiers can be used to define the policy to modify request and response headers.
// +kubebuilder:validation:AtLeastOneOf=request;response
type HeaderModifiers struct {
	// Request modifies request headers.
	// +optional
	Request *HTTPHeaderFilter `json:"request,omitempty"`

	// Response modifies response headers.
	// +optional
	Response *HTTPHeaderFilter `json:"response,omitempty"`
}

// HTTPHeaderFilter defines header mutation rules, supporting both inline string values and
// Kubernetes Secret-backed values.
// +kubebuilder:validation:AtLeastOneOf=set;add;remove
type HTTPHeaderFilter struct {
	// Set overwrites the request with the given header (name, value) before the action.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=16
	Set []HTTPHeader `json:"set,omitempty"`

	// Add adds the given header(s) (name, value) to the request before the action.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=16
	Add []HTTPHeader `json:"add,omitempty"`

	// Remove the given header(s) from the HTTP request before the action.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MaxItems=16
	Remove []string `json:"remove,omitempty"`
}

// HTTPHeader represents a single header name/value pair. Exactly one of value or secretRef must
// be set. When secretRef is used and name is omitted, the secret key is used as the header name.
// +kubebuilder:validation:ExactlyOneOf=value;secretRef
// +kubebuilder:validation:XValidation:rule="has(self.value) ? has(self.name) : true",message="name is required when using an inline value"
type HTTPHeader struct {
	// Name is the header field name. Required when value is set.
	// When secretRef is used and name is omitted, the secret key is used as the header name.
	// +optional
	Name *gwv1.HTTPHeaderName `json:"name,omitempty"`

	// Value is an inline string value for the header. Mutually exclusive with secretRef.
	// +optional
	Value *string `json:"value,omitempty"`

	// SecretRef sources the header value from a key in a Kubernetes Secret.
	// The namespace must be specified explicitly. Mutually exclusive with value.
	// +optional
	SecretRef *SecretKeyRef `json:"secretRef,omitempty"`
}

// SecretKeyRef identifies a specific key within a Kubernetes Secret.
type SecretKeyRef struct {
	// Name is the name of the Kubernetes Secret.
	// +required
	Name gwv1.ObjectName `json:"name"`

	// Key is the key within the Secret's data map whose value will be used as the header value.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Key string `json:"key"`

	// Namespace is the namespace of the Secret. Cross-namespace references require a ReferenceGrant
	// in the target namespace permitting access from the policy's namespace.
	// +required
	Namespace gwv1.Namespace `json:"namespace"`
}

// CIDR can be used wherever an address range in CIDR notation is expected.
// Note: The regex for the IP validation patterns was taken from https://www.ditig.com/validating-ipv4-and-ipv6-addresses-with-regexp
// +kubebuilder:validation:Format=cidr
// +kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}\/([0-9]|[1-2][0-9]|3[0-2])$|^((?:[0-9A-Fa-f]{1,4}:){7}[0-9A-Fa-f]{1,4}|(?:[0-9A-Fa-f]{1,4}:){1,7}:|:(?::[0-9A-Fa-f]{1,4}){1,7}|(?:[0-9A-Fa-f]{1,4}:){1,6}:[0-9A-Fa-f]{1,4}|(?:[0-9A-Fa-f]{1,4}:){1,5}(?::[0-9A-Fa-f]{1,4}){1,2}|(?:[0-9A-Fa-f]{1,4}:){1,4}(?::[0-9A-Fa-f]{1,4}){1,3}|(?:[0-9A-Fa-f]{1,4}:){1,3}(?::[0-9A-Fa-f]{1,4}){1,4}|(?:[0-9A-Fa-f]{1,4}:){1,2}(?::[0-9A-Fa-f]{1,4}){1,5}|[0-9A-Fa-f]{1,4}:(?:(?::[0-9A-Fa-f]{1,4}){1,6})|:(?:(?::[0-9A-Fa-f]{1,4}){1,6}))\/(12[0-8]|1[0-1][0-9]|[1-9][0-9]|[0-9])$`
type CIDR string

// IPOrCIDR accepts either a bare IP address or an address range in CIDR notation.
// A bare IP without a prefix length is treated as /32 for IPv4 and /128 for IPv6.
// Note: The regex for the IP validation patterns was taken from https://www.ditig.com/validating-ipv4-and-ipv6-addresses-with-regexp
// +kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}(\/([0-9]|[1-2][0-9]|3[0-2]))?$|^((?:[0-9A-Fa-f]{1,4}:){7}[0-9A-Fa-f]{1,4}|(?:[0-9A-Fa-f]{1,4}:){1,7}:|:(?::[0-9A-Fa-f]{1,4}){1,7}|(?:[0-9A-Fa-f]{1,4}:){1,6}:[0-9A-Fa-f]{1,4}|(?:[0-9A-Fa-f]{1,4}:){1,5}(?::[0-9A-Fa-f]{1,4}){1,2}|(?:[0-9A-Fa-f]{1,4}:){1,4}(?::[0-9A-Fa-f]{1,4}){1,3}|(?:[0-9A-Fa-f]{1,4}:){1,3}(?::[0-9A-Fa-f]{1,4}){1,4}|(?:[0-9A-Fa-f]{1,4}:){1,2}(?::[0-9A-Fa-f]{1,4}){1,5}|[0-9A-Fa-f]{1,4}:(?:(?::[0-9A-Fa-f]{1,4}){1,6})|:(?:(?::[0-9A-Fa-f]{1,4}){1,6}))(\/(12[0-8]|1[0-1][0-9]|[1-9][0-9]|[0-9]))?$`
type IPOrCIDR string
