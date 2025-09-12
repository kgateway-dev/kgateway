package reporter

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1alpha1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"
)

const (
	PolicyAcceptedMsg = "Policy accepted"

	PolicyInvalidMsg = "Policy is invalid"

	PolicyConflictWithHigherPriorityMsg = "Policy conflicts with higher priority policy"

	PolicyAttachedMsg = "Attached to all targets"

	PolicyMergedMsg = "Merged with other policies in target(s) and attached"

	PolicyOverriddenMsg = "Overridden due to conflict with higher priority policy in target(s)"

	// RouteRuleDroppedReason is used with the Accepted=False condition when the route rule is dropped.
	RouteRuleDroppedReason = "RouteRuleDropped"

	// RouteRuleReplacedReason is used with the Accepted=False condition when the route rule is replaced
	// with a direct response.
	RouteRuleReplacedReason = "RouteRuleReplaced"

	// ListenerReplacedReason is used with the Accepted=False condition when an individual listener
	// attached to a Gateway is replaced. This is typically due to a policy error.
	ListenerReplacedReason = "ListenerReplaced"
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
	ListenerSet(listenerSet *gwxv1alpha1.XListenerSet) ListenerSetReporter
	Route(obj metav1.Object) RouteReporter
	Policy(ref PolicyKey, observedGeneration int64) PolicyReporter
}

type GatewayReporter interface {
	Listener(listener *gwv1.Listener) ListenerReporter
	ListenerName(listenerName string) ListenerReporter
	SetCondition(condition GatewayCondition)
}

type ListenerSetReporter interface {
	Listener(listener *gwv1.Listener) ListenerReporter
	ListenerName(listenerName string) ListenerReporter
	SetCondition(condition GatewayCondition)
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
