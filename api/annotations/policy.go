package annotations

// InheritedPolicyPriority is the annotation used on a Gateway or parent HTTPRoute to specify
// the priority of corresponding policies attached that are inherited by attached routes or child routes respectively.
const InheritedPolicyPriority = "kgateway.dev/inherited-policy-priority"

// InheritedPolicyPriorityValue is the value for the InheritedPolicyPriority annotation
type InheritedPolicyPriorityValue string

const (
	// ShallowMergePreferParent is the value for the InheritedPolicyPriority annotation to indicate that
	// inherited parent policies (attached to the Gateway or parent HTTPRoute) should be shallow merged and
	// preferred over policies directly attached to child routes in case of conflicts.
	ShallowMergePreferParent InheritedPolicyPriorityValue = "ShallowMergePreferParent"

	// ShallowMergePreferChild is the value for the InheritedPolicyPriority annotation to indicate that
	// policies attached to the child route should be shallow merged and preferred over inherited parent policies
	// (attached to the Gateway or parent HTTPRoute) in case of conflicts.
	ShallowMergePreferChild InheritedPolicyPriorityValue = "ShallowMergePreferChild"

	// DeepMergePreferParent is the value for the InheritedPolicyPriority annotation to indicate that
	// inherited parent policies (attached to the Gateway or parent HTTPRoute) should be deep merged and
	// preferred over policies directly attached to child routes in case of conflicts.
	DeepMergePreferParent InheritedPolicyPriorityValue = "DeepMergePreferParent"

	// DeepMergePreferChild is the value for the InheritedPolicyPriority annotation to indicate that
	// policies attached to the child route should be deep merged and preferred over inherited parent policies
	// (attached to the Gateway or parent HTTPRoute) in case of conflicts.
	DeepMergePreferChild InheritedPolicyPriorityValue = "DeepMergePreferChild"
)
