package reporter

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

const (
	PolicyAcceptedMsg = "Policy accepted"

	PolicyInvalidMsg = "Policy is invalid"

	PolicyConflictWithHigherPriorityMsg = "Policy conflicts with higher priority policy"

	PolicyAttachedMsg = "Attached to all targets"

	PolicyMergedMsg = "Merged with other policies in target(s) and attached"

	PolicyOverriddenMsg = "Overridden due to conflict with higher priority policy in target(s)"

	// PolicyTargetNotFoundMsg is the Attached message reported on a policy whose targetRefs
	// name objects that do not exist.
	PolicyTargetNotFoundMsg = "Policy is not attached to targets that could not be resolved"

	// PolicyStatusSummaryAncestorName is the kind and name of the synthetic ancestor entry
	// PolicyStatusSummaryAncestorRef describes. It matches the name agentgateway uses for the
	// same entry so tooling can match one constant across both.
	PolicyStatusSummaryAncestorName = "StatusSummary"

	// RouteRuleDroppedReason is used with the Accepted=False condition when the route rule is dropped.
	RouteRuleDroppedReason = "RouteRuleDropped"

	// RouteRuleReplacedReason is used with the Accepted=False condition when the route rule is replaced
	// with a direct response.
	RouteRuleReplacedReason = "RouteRuleReplaced"

	// ListenerReplacedReason is used with the Accepted=False condition when an individual listener
	// on a Gateway or ListenerSet is replaced due to an error in a policy targeting that listener.
	ListenerReplacedReason = "ListenerReplaced"

	// GatewayReplacedReason is used with the Accepted=False condition when the entire Gateway is replaced
	// due to an error in a policy targeting the Gateway.
	GatewayReplacedReason = "GatewayReplaced"
)

// PolicyAttachmentState represents the state of a policy attachment
type PolicyAttachmentState int

const (
	// PolicyAttachmentStatePending indicates that the policy is pending attachment
	PolicyAttachmentStatePending PolicyAttachmentState = iota

	// PolicyAttachmentStateSucceeded indicates that the full policy was successfully attached
	PolicyAttachmentStateAttached PolicyAttachmentState = 1 << iota

	// PolicyAttachmentStateMerged indicates that the policy was merged with other policies and attached
	PolicyAttachmentStateMerged

	// PolicyAttachmentStateOverridden indicates that the policy conflicts with higher priority policies
	// and was fully overridden
	PolicyAttachmentStateOverridden
)

// Has checks if the existing state has the given state
func (a PolicyAttachmentState) Has(b PolicyAttachmentState) bool {
	return a&b != 0
}

type PolicyCondition struct {
	Type               string
	Status             metav1.ConditionStatus
	Reason             string
	Message            string
	ObservedGeneration int64
}

type PolicyKey struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
}

func (p PolicyKey) DisplayString() string {
	return p.Kind + "/" + p.Namespace + "/" + p.Name
}

type GatewayCondition struct {
	Type    gwv1.GatewayConditionType
	Status  metav1.ConditionStatus
	Reason  gwv1.GatewayConditionReason
	Message string
}

type ListenerCondition struct {
	Type    gwv1.ListenerConditionType
	Status  metav1.ConditionStatus
	Reason  gwv1.ListenerConditionReason
	Message string
}

type RouteCondition struct {
	Type    gwv1.RouteConditionType
	Status  metav1.ConditionStatus
	Reason  gwv1.RouteConditionReason
	Message string
}

type BackendCondition struct {
	Type    string
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

type AncestorRefReporter interface {
	SetCondition(condition PolicyCondition)
	SetAttachmentState(
		state PolicyAttachmentState,
	)
}

type PolicyReporter interface {
	AncestorRef(parentRef gwv1.ParentReference) AncestorRefReporter
}

type Reporter interface {
	Gateway(gateway *gwv1.Gateway) GatewayReporter
	ListenerSet(listenerSet client.Object) ListenerSetReporter
	Route(obj metav1.Object) RouteReporter
	Policy(ref PolicyKey, observedGeneration int64) PolicyReporter
	Backend(obj metav1.Object) BackendReporter
}

type GatewayReporter interface {
	Listener(listener *gwv1.Listener) ListenerReporter
	ListenerName(listenerName string) ListenerReporter
	SetCondition(condition GatewayCondition)
	SetAttachedListenerSets(count int32)
}

type ListenerSetReporter interface {
	Listener(listener *gwv1.Listener) ListenerReporter
	ListenerName(listenerName string) ListenerReporter
	SetCondition(condition GatewayCondition)
	SetAttachedListenerSets(count int32)
}

type ListenerReporter interface {
	SetCondition(ListenerCondition)
	SetSupportedKinds([]gwv1.RouteGroupKind)
	SetAttachedRoutes(n uint)
}

type RouteReporter interface {
	ParentRef(parentRef *gwv1.ParentReference) ParentRefReporter
}

type ParentRefReporter interface {
	SetCondition(condition RouteCondition)
}

type BackendReporter interface {
	SetCondition(condition BackendCondition)
}

// PolicyStatusSummaryAncestorRef is the ancestor under which a policy reports findings that
// belong to the policy as a whole rather than to any Gateway: today, targetRefs that do not
// resolve. A missing target has no Gateway to report under, and Gateway API gives policy status
// no home for conditions other than an ancestor entry, so a fixed synthetic entry, one per
// policy, carries them. Group and kind are explicit because the CRD schema defaults an omitted
// ancestorRef group to gateway.networking.k8s.io and kind to Gateway, which would make the
// entry read as a Gateway named StatusSummary. Namespace is omitted so it reads as
// policy-scoped.
func PolicyStatusSummaryAncestorRef() gwv1.ParentReference {
	return gwv1.ParentReference{
		Group: new(gwv1.Group(kgateway.GroupName)),
		Kind:  new(gwv1.Kind(PolicyStatusSummaryAncestorName)),
		Name:  PolicyStatusSummaryAncestorName,
	}
}

// IsPolicyStatusSummaryAncestor reports whether an ancestor, given as the fields of its ref,
// is the one PolicyStatusSummaryAncestorRef describes.
func IsPolicyStatusSummaryAncestor(group, kind, namespace, name string) bool {
	return group == kgateway.GroupName &&
		kind == PolicyStatusSummaryAncestorName &&
		namespace == "" &&
		name == PolicyStatusSummaryAncestorName
}

// IsPolicyStatusSummaryAncestorRef is IsPolicyStatusSummaryAncestor for a ParentReference.
func IsPolicyStatusSummaryAncestorRef(ref gwv1.ParentReference) bool {
	deref := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}
	return IsPolicyStatusSummaryAncestor(
		deref((*string)(ref.Group)), deref((*string)(ref.Kind)), deref((*string)(ref.Namespace)), string(ref.Name))
}
