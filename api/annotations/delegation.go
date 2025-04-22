package annotations

const (
	// DelegationInheritMatcher is the annotation used on a child HTTPRoute that
	// participates in a delegation chain to indicate that child route should inherit
	// the route matcher from the parent route.
	DelegationInheritMatcher = "delegation.kgateway.dev/inherit-parent-matcher"

	// DelegationInheritedPolicyPriority is the annotation used on an HTTPRoute to specify
	// the priority of policies attached to the route that are inherited by delegatee(child) routes.
	DelegationInheritedPolicyPriority = "delegation.kgateway.dev/inherited-policy-priority"
)

// DelegationInheritedPolicyPriorityValue is the value for the DelegationInheritedPolicyPriority annotation
type DelegationInheritedPolicyPriorityValue string

const (
	// DelegationInheritedPolicyPriorityPreferParent is the value for the DelegationInheritedPolicyPriority
	// annotation to indicate that the delegatee(child) route should prefer policies attached to the parent route
	// such that parent policies are prioritized over policies directly attached to child routes in case of conflicts.
	DelegationInheritedPolicyPriorityPreferParent DelegationInheritedPolicyPriorityValue = "PreferParent"

	// DelegationInheritedPolicyPriorityPreferChild is the value for the DelegationInheritedPolicyPriority
	// annotation to indicate that the delegatee(child) route should prefer policies attached to the child route
	// such that child policies are prioritized over policies attached to parent routes in case of conflicts.
	DelegationInheritedPolicyPriorityPreferChild DelegationInheritedPolicyPriorityValue = "PreferChild"
)
