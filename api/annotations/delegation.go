package annotations

const (
	// InheritMatcherAnnotation is the annotation used on a child HTTPRoute that
	// participates in a delegation chain to indicate that child route should inherit
	// the route matcher from the parent route.
	DelegationInheritMatcherAnnotation = "delegation.kgateway.dev/inherit-parent-matcher"
)
