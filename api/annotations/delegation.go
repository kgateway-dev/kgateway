package annotations

const (
	// DelegationInheritMatcher is the annotation used on a child HTTPRoute that
	// participates in a delegation chain to indicate that child route should inherit
	// the route matcher from the parent route.
	DelegationInheritMatcher = "delegation.kgateway.dev/inherit-parent-matcher"
)
