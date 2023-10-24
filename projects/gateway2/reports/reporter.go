package reports

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// TODO: this is a stub to make the code compile
type ResultsMap = map[string]string

type reporter struct {
	results *ResultsMap
}

func newReporter(results interface{}) Reporter {
	panic("implement me")
}

// Reports errors for GW translation
type Reporter interface {
	// returns the object reporter for the given type
	Gateway(gateway *apiv1.Gateway) GatewayReporter

	Route(route *apiv1.HTTPRoute) HTTPRouteReporter
}

type GatewayReporter interface {
	// report an error on the whole gateway
	Err(format string, a ...any)

	// report an error on the given listener
	Listener(listener *apiv1.Listener) ListenerReporter

	SetCondition(condition apiv1.ListenerConditionType, status metav1.ConditionStatus, reason apiv1.ListenerConditionReason, message string)
}

type ListenerReporter interface {
	// report an error on the listener
	Err(format string, a ...any)

	// TODO: If a set of Listeners contains Listeners that are not distinct, then those Listeners are Conflicted, and the implementation MUST set the “Conflicted” condition in the Listener Status to “True”.

	SetCondition(condition apiv1.ListenerConditionType, status metav1.ConditionStatus, reason apiv1.ListenerConditionReason, message string)
}

type HTTPRouteReporter interface {
	// report an error on the listener
	Err(format string, a ...any)

	// TODO: If a set of Listeners contains Listeners that are not distinct, then those Listeners are Conflicted, and the implementation MUST set the “Conflicted” condition in the Listener Status to “True”.
	SetCondition(condition apiv1.ListenerConditionType, status metav1.ConditionStatus, reason apiv1.ListenerConditionReason, message string)
}
