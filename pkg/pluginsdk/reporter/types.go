package reporter

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	PolicyAcceptedMsg = "Policy accepted"

	PolicyAcceptedAndAttachedMsg = "Policy accepted and attached"
)

type PolicyCondition struct {
	Type    gwv1alpha2.PolicyConditionType
	Status  metav1.ConditionStatus
	Reason  gwv1alpha2.PolicyConditionReason
	Message string
}

type PolicyKey struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
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
}

type PolicyReporter interface {
	AncestorRef(parentRef gwv1.ParentReference) AncestorRefReporter
}

type Reporter interface {
	Gateway(gateway *gwv1.Gateway) GatewayReporter
	Route(obj metav1.Object) RouteReporter
	Policy(ref PolicyKey, observedGeneration int64) PolicyReporter
}

type GatewayReporter interface {
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
