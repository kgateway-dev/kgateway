package shared

// Authorization defines the configuration for role-based access control.
type Authorization struct {
	// Policy specifies the Authorization rule to evaluate.
	// A policy matches when **any** of the conditions evaluates to true.
	// +required
	Policy AuthorizationPolicy `json:"policy"`

	// Action defines whether the rule allows or denies the request if matched.
	// If unspecified, the default is "Allow".
	// +kubebuilder:validation:Enum=Allow;Deny
	// +kubebuilder:default=Allow
	// +optional
	Action AuthorizationPolicyAction `json:"action,omitempty"`
}

// CELExpression represents a Common Expression Language (CEL) expression.
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=16384
// +k8s:deepcopy-gen=false
type CELExpression string

// AuthorizationPolicy defines a single Authorization rule.
// +kubebuilder:validation:XValidation:rule="has(self.matchExpressions) || has(self.allowedCIDRs) || has(self.blockedCIDRs)",message="at least one of matchExpressions, allowedCIDRs, or blockedCIDRs must be specified"
type AuthorizationPolicy struct {
	// MatchExpressions defines a set of conditions that must be satisfied for the rule to match.
	// These expression should be in the form of a Common Expression Language (CEL) expression.
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=256
	// +optional
	MatchExpressions []CELExpression `json:"matchExpressions,omitempty"`

	// AllowedCIDRs defines a list of IP address ranges in CIDR notation that are allowed.
	// These are combined with MatchExpressions using AND logic - both must match.
	// Uses the client's source IP address for matching.
	// Example: ["192.168.1.0/24", "10.0.0.0/8"]
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=256
	AllowedCIDRs []string `json:"allowedCIDRs,omitempty"`

	// BlockedCIDRs defines a list of IP address ranges in CIDR notation that are blocked.
	// If an IP matches BlockedCIDRs, it will be denied regardless of AllowedCIDRs.
	// Uses the client's source IP address for matching.
	// Example: ["192.168.1.100/32", "10.1.0.0/16"]
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=256
	BlockedCIDRs []string `json:"blockedCIDRs,omitempty"`
}

// AuthorizationPolicyAction defines the action to take when the RBACPolicies matches.
type AuthorizationPolicyAction string

const (
	// AuthorizationPolicyActionAllow defines the action to take when the RBACPolicies matches.
	AuthorizationPolicyActionAllow AuthorizationPolicyAction = "Allow"
	// AuthorizationPolicyActionDeny denies the action to take when the RBACPolicies matches.
	AuthorizationPolicyActionDeny AuthorizationPolicyAction = "Deny"
)
