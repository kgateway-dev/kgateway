package annotations

const (
	// DelegationInheritMatcher is the annotation used on a child HTTPRoute that
	// participates in a delegation chain to indicate that child route should inherit
	// the route matcher from the parent route.
	DelegationInheritMatcher = "delegation.kgateway.dev/inherit-parent-matcher"

	// DelegationInheritBackend is the annotation used on a parent route to enable
	// child routes to override policies applied to the parent route.
	DelegationEnablePolicyOverride = "delegation.kgateway.dev/enable-policy-override"

	// DelegationEnablePolicyOverrideValueAllFields is the value for the DelegationEnablePolicyOverride
	// to indicate all fields of the parent route can be overridden by child routes.
	DelegationEnablePolicyOverrideValueAllFields = "*"
)
