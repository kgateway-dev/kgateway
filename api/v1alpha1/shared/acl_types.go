package shared

// ACLAction defines whether to allow or deny traffic.
// +kubebuilder:validation:Enum=allow;deny
type ACLAction string

const (
	ACLActionAllow ACLAction = "allow"
	ACLActionDeny  ACLAction = "deny"
)

// ACLRule defines an IP/CIDR-based ACL rule.
type ACLRule struct {
	// Name is an optional rule identifier emitted as blocked-by dynamic metadata on deny.
	// +optional
	// +kubebuilder:validation:MaxLength=256
	Name *string `json:"name,omitempty"`

	// CIDRs is a list of IP addresses or CIDR ranges (e.g. "10.0.0.0/8", "2001:db8::/32", "192.168.1.1", "::1").
	// Bare IPs without a prefix are treated as /32 for IPv4 and /128 for IPv6.
	// All entries share the same name and action.
	// +required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=256
	CIDRs []IPOrCIDR `json:"cidrs"`

	// Action determines what to do when a client IP matches this rule.
	// +required
	Action ACLAction `json:"action"`
}

// ACLResponseHeader defines a response header to include in deny responses.
type ACLResponseHeader struct {
	// Name is the header name.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Name string `json:"name"`

	// Value is the header value.
	// +required
	// +kubebuilder:validation:MaxLength=4096
	Value string `json:"value"`
}

// ACLDenyResponse customizes the response sent when a request is denied.
// +kubebuilder:validation:AtLeastOneOf=statusCode;headers;blockedByHeaderName
type ACLDenyResponse struct {
	// StatusCode is the HTTP status code returned on deny. Defaults to 403.
	// +optional
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:Maximum=599
	StatusCode *int32 `json:"statusCode,omitempty"`

	// Headers are additional response headers to attach on every deny.
	// +optional
	// +kubebuilder:validation:MaxItems=16
	Headers []ACLResponseHeader `json:"headers,omitempty"`

	// BlockedByHeaderName, when set, adds a response header with this name on every deny.
	// The header value mirrors the blocked-by dynamic metadata: the matched rule's name,
	// "rule" for an unnamed rule, or "default" for a default-action deny.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	BlockedByHeaderName *string `json:"blockedByHeaderName,omitempty"`
}

// ACLPolicy defines IP-based access control rules evaluated on every HTTP request.
// The filter uses longest-prefix matching so rule order does not matter.
type ACLPolicy struct {
	// DefaultAction is the action to take when no rule matches the client IP.
	// +required
	DefaultAction ACLAction `json:"defaultAction"`

	// Rules is a list of IP/CIDR-based rules. Longest-prefix match wins regardless of rule order.
	// +optional
	// +kubebuilder:validation:MaxItems=256
	Rules []ACLRule `json:"rules,omitempty"`

	// DenyResponse customizes the HTTP response sent when a request is denied.
	// +optional
	DenyResponse *ACLDenyResponse `json:"denyResponse,omitempty"`
}
