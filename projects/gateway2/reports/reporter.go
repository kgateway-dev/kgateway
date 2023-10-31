package reports

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ReportMap struct {
	Gateways map[string]*GatewayReport
	Routes   map[types.NamespacedName]*RouteReport
}

type GatewayReport struct {
	Conditions []metav1.Condition
	Listeners  map[string]*ListenerReport
}

type RouteReport struct {
	Conditions []HTTPRouteCondition
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

func (r *reporter) Route(route *gwv1.HTTPRoute) HTTPRouteReporter {
	var rr *RouteReport
	key := types.NamespacedName{
		Namespace: route.Namespace,
		Name:      route.Name,
	}
	rr, ok := r.report.Routes[key]
	if !ok {
		rr = &RouteReport{}
		r.report.Routes[key] = rr
	}
	return rr
}

func (r *RouteReport) SetCondition(rc HTTPRouteCondition) {
	r.Conditions = append(r.Conditions, rc)
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

func (g *GatewayReport) SetCondition(gc GatewayCondition) {
	condition := metav1.Condition{
		Type:    string(gc.Type),
		Status:  gc.Status,
		Reason:  string(gc.Reason),
		Message: gc.Message,
	}
	g.Conditions = append(g.Conditions, condition)
}

func (l *ListenerReport) SetCondition(lc ListenerCondition) {
	condition := metav1.Condition{
		Type:    string(lc.Type),
		Status:  lc.Status,
		Reason:  string(lc.Reason),
		Message: lc.Message,
	}
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

	Route(route *gwv1.HTTPRoute) HTTPRouteReporter
}

type GatewayReporter interface {
	// report an error on the whole gateway
	// Err(format string, a ...any)

	// report an error on the given listener
	// TODO(Law): use string here instead of Listener type
	Listener(listener *gwv1.Listener) ListenerReporter

	SetCondition(condition GatewayCondition)
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

type ListenerReporter interface {
	// report an error on the listener
	// Err(format string, a ...any)

	SetCondition(ListenerCondition)

	SetSupportedKinds([]gwv1.RouteGroupKind)
}

type HTTPRouteCondition struct {
	Type    gwv1.RouteConditionType
	Status  metav1.ConditionStatus
	Reason  gwv1.RouteConditionReason
	Message string
}

type HTTPRouteReporter interface {
	// Err(format string, a ...any)

	SetCondition(condition HTTPRouteCondition)
}
