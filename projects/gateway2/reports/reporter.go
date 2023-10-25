package reports

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ReportMap struct {
	Gateways map[string]*GatewayReport
}

type GatewayReport struct {
	// condition for the top-level gateway
	Condition metav1.Condition
	// listeners under this GW
	Listeners map[string]*ListenerReport
}

type ListenerReport struct {
	Status gwv1.ListenerStatus
}

type reporter struct {
	report *ReportMap
}

func (r *reporter) Gateway(gateway *gwv1.Gateway) GatewayReporter {
	var gr *GatewayReport
	//TODO(Law): use correct name here (include namespace, maybe cluster?)
	gr, ok := r.report.Gateways[gateway.Name]
	if !ok {
		gr = &GatewayReport{}
		r.report.Gateways[gateway.Name] = gr
	}
	return gr
}

func (r *GatewayReport) Listener(listener *gwv1.Listener) ListenerReporter {
	if r.Listeners == nil {
		r.Listeners = make(map[string]*ListenerReport)
	}
	var lr *ListenerReport
	lr, ok := r.Listeners[string(listener.Name)]
	if !ok {
		lr = &ListenerReport{}
		lr.Status.Name = listener.Name
		r.Listeners[string(listener.Name)] = lr
	}
	return lr
}

func (l *ListenerReport) SetCondition(condition metav1.Condition) {
	l.Status.Conditions = append(l.Status.Conditions, condition)
}

func (l *ListenerReport) SetSupportedKinds(rgks []gwv1.RouteGroupKind) {
	l.Status.SupportedKinds = rgks
}

func NewReporter(reportMap *ReportMap) Reporter {
	return &reporter{report: reportMap}
}

// Reports errors for GW translation
type Reporter interface {
	// returns the object reporter for the given type
	// TODO(Law): use string here instead of Gateway type
	Gateway(gateway *gwv1.Gateway) GatewayReporter

	// Route(route *gwv1.HTTPRoute) HTTPRouteReporter
}

type GatewayReporter interface {
	// report an error on the whole gateway
	// Err(format string, a ...any)

	// report an error on the given listener
	// TODO(Law): use string here instead of Listener type
	Listener(listener *gwv1.Listener) ListenerReporter

	// SetCondition(condition gwv1.ListenerConditionType, status metav1.ConditionStatus, reason gwv1.ListenerConditionReason, message string)
}

type ListenerReporter interface {
	// report an error on the listener
	// Err(format string, a ...any)

	SetCondition(metav1.Condition)

	SetSupportedKinds([]gwv1.RouteGroupKind)
}

type HTTPRouteReporter interface {
	// Err(format string, a ...any)

	// SetCondition(condition gwv1.ListenerConditionType, status metav1.ConditionStatus, reason gwv1.ListenerConditionReason, message string)
}
