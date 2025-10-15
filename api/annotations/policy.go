package annotations

const (
	// InheritedPolicyPriority is the annotation used on a Gateway or parent HTTPRoute to specify
	// the priority of corresponding policies attached that are inherited by attached routes or child routes respectively.
	InheritedPolicyPriority = "kgateway.dev/inherited-policy-priority"

	// PolicyPrecedenceWeight is an annotation that can be set on a policy CR to specify the weight of
	// the policy as an integer value (negative values are allowed).
	// Policies with higher weight implies higher priority, and are evaluated before policies with lower weight.
	// By default, policies have a weight of 0.
	// The policy's weight is relevant to policy prioritization during policy merging, such that higher priority
	// policies are preferred during a merge conflict or when ordering policies during a merge.
	// Note: for policies that are implemented using GatewayExtensions (such as extAuth, etcProc), the weight specified on the GatewayExtension
	// will be used instead.
	PolicyPrecedenceWeight = "kgateway.dev/policy-weight"

	// DisableIstioAutoMTLS, if present on any backend object (Backend, K8s Service, ServiceEntry, etc.),
	// disables Istio auto-mTLS for that specific backend.
	// This is useful for cases where you want to disable Istio auto-mTLS for a specific backend, but still use other TLS mechanisms
	// (by applying a BackendConfigPolicy or BackendTLSPolicy).
	DisableIstioAutoMTLS = "kgateway.dev/disable-istio-auto-mtls"

	// HTTPRedirectStatusCode is an annotation that can be set on an HTTPRoute to specify the HTTP status code for the RequestRedirect
	// filter. The value must be one of 301, 302, 303, 307, 308.
	// By default, this annotation will override the statusCode field on the RequestRedirect filter for all route rules using the RequestRedirect filter.
	// E.g., kgateway.dev/http-redirect-status-code: "307" will set the status code to 307 for all RequestRedirect filters on the HTTPRoute.
	// To set a different status code for individual route rules, the value must be a comma-separated list of rule-name=status-code pairs.
	// E.g., kgateway.dev/http-redirect-status-code: "rule1=307,rule2=308" will set the status code to 307 for filter on rule1 and 308 for filter on rule2.
	HTTPRedirectStatusCode = "kgateway.dev/http-redirect-status-code"
)

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
