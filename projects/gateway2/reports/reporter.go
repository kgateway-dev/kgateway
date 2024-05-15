package reports

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ReportMap struct {
	gateways map[types.NamespacedName]*GatewayReport
	routes   map[types.NamespacedName]*RouteReport
}

type GatewayReport struct {
	conditions         []metav1.Condition
	listeners          map[string]*ListenerReport
	observedGeneration int64
}

type ListenerReport struct {
	Status gwv1.ListenerStatus
}

type RouteReport struct {
	parents            map[ParentRefKey]*ParentRefReport
	observedGeneration int64
}

// TODO: rename to e.g. RouteParentRefReport
type ParentRefReport struct {
	Conditions []metav1.Condition
}

type ParentRefKey struct {
	Group string
	Kind  string
	types.NamespacedName
}

func NewReportMap() ReportMap {
	gr := make(map[types.NamespacedName]*GatewayReport)
	rr := make(map[types.NamespacedName]*RouteReport)
	return ReportMap{
		gateways: gr,
		routes:   rr,
	}
}

// Returns a GatewayReport for the provided Gateway, nil if there is not a report present.
// This is different than the Reporter.Gateway() method, as we need to understand when
// reports are not generated for a Gateway that has been translated.
//
// NOTE: Exported for unit testing, validation_test.go should be refactored to reduce this visibility
func (r *ReportMap) Gateway(gateway *gwv1.Gateway) *GatewayReport {
	key := client.ObjectKeyFromObject(gateway)
	return r.gateways[key]
}

func (r *ReportMap) newGatewayReport(gateway *gwv1.Gateway) *GatewayReport {
	gr := &GatewayReport{}
	gr.observedGeneration = gateway.Generation
	key := client.ObjectKeyFromObject(gateway)
	r.gateways[key] = gr
	return gr
}

// Returns a RouteReport for the provided HTTPRoute, nil if there is not a report present.
// This is different than the Reporter.Route() method, as we need to understand when
// reports are not generated for a HTTPRoute that has been translated.
func (r *ReportMap) route(route *gwv1.HTTPRoute) *RouteReport {
	key := client.ObjectKeyFromObject(route)
	return r.routes[key]
}

func (r *ReportMap) newRouteReport(route *gwv1.HTTPRoute) *RouteReport {
	rr := &RouteReport{}
	rr.observedGeneration = route.Generation
	key := client.ObjectKeyFromObject(route)
	r.routes[key] = rr
	return rr
}

func (g *GatewayReport) Listener(listener *gwv1.Listener) ListenerReporter {
	return g.listener(listener)
}

func (g *GatewayReport) listener(listener *gwv1.Listener) *ListenerReport {
	if g.listeners == nil {
		g.listeners = make(map[string]*ListenerReport)
	}
	lr := g.listeners[string(listener.Name)]
	if lr == nil {
		lr = NewListenerReport(string(listener.Name))
		g.listeners[string(listener.Name)] = lr
	}
	return lr
}

func (g *GatewayReport) GetConditions() []metav1.Condition {
	if g == nil {
		return []metav1.Condition{}
	}
	return g.conditions
}

func (g *GatewayReport) SetCondition(gc GatewayCondition) {
	condition := metav1.Condition{
		Type:    string(gc.Type),
		Status:  gc.Status,
		Reason:  string(gc.Reason),
		Message: gc.Message,
	}
	g.conditions = append(g.conditions, condition)
}

func NewListenerReport(name string) *ListenerReport {
	lr := ListenerReport{}
	lr.Status.Name = gwv1.SectionName(name)
	return &lr
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

func (l *ListenerReport) SetAttachedRoutes(n uint) {
	l.Status.AttachedRoutes = int32(n)
}

type reporter struct {
	report *ReportMap
}

func (r *reporter) Gateway(gateway *gwv1.Gateway) GatewayReporter {
	gr := r.report.Gateway(gateway)
	if gr == nil {
		gr = r.report.newGatewayReport(gateway)
	}
	return gr
}

func (r *reporter) Route(route *gwv1.HTTPRoute) HTTPRouteReporter {
	rr := r.report.route(route)
	if rr == nil {
		rr = r.report.newRouteReport(route)
	}
	return rr
}

// TODO: flesh out
func getParentRefKey(parentRef *gwv1.ParentReference) ParentRefKey {
	var group string
	if parentRef.Group != nil {
		group = string(*parentRef.Group)
	}
	var kind string
	if parentRef.Kind != nil {
		kind = string(*parentRef.Kind)
	}
	var ns string
	if parentRef.Namespace != nil {
		ns = string(*parentRef.Namespace)
	}
	return ParentRefKey{
		Group: group,
		Kind:  kind,
		NamespacedName: types.NamespacedName{
			Namespace: ns,
			Name:      string(parentRef.Name),
		},
	}
}

func (r *RouteReport) parentRef(parentRef *gwv1.ParentReference) *ParentRefReport {
	key := getParentRefKey(parentRef)
	if r.parents == nil {
		r.parents = make(map[ParentRefKey]*ParentRefReport)
	}
	var prr *ParentRefReport
	prr, ok := r.parents[key]
	if !ok {
		prr = &ParentRefReport{}
		r.parents[key] = prr
	}
	return prr
}

// parentRefs returns a list of ParentReferences associated with the RouteReport.
// It is used to update the Status of delegatee routes who may not specify
// the parentRefs field.
func (r *RouteReport) parentRefs() []gwv1.ParentReference {
	var refs []gwv1.ParentReference
	for key := range r.parents {
		parentRef := gwv1.ParentReference{
			Group:     ptr.To(gwv1.Group(key.Group)),
			Kind:      ptr.To(gwv1.Kind(key.Kind)),
			Name:      gwv1.ObjectName(key.Name),
			Namespace: ptr.To(gwv1.Namespace(key.Namespace)),
		}
		refs = append(refs, parentRef)
	}
	return refs
}

func (r *RouteReport) ParentRef(parentRef *gwv1.ParentReference) ParentRefReporter {
	return r.parentRef(parentRef)
}

func (prr *ParentRefReport) SetCondition(rc HTTPRouteCondition) {
	condition := metav1.Condition{
		Type:    string(rc.Type),
		Status:  rc.Status,
		Reason:  string(rc.Reason),
		Message: rc.Message,
	}
	prr.Conditions = append(prr.Conditions, condition)
}

func NewReporter(reportMap *ReportMap) Reporter {
	return &reporter{report: reportMap}
}

type Reporter interface {
	Gateway(gateway *gwv1.Gateway) GatewayReporter
	Route(route *gwv1.HTTPRoute) HTTPRouteReporter
}

type GatewayReporter interface {
	Listener(listener *gwv1.Listener) ListenerReporter
	SetCondition(condition GatewayCondition)
}

type ListenerReporter interface {
	SetCondition(ListenerCondition)
	SetSupportedKinds([]gwv1.RouteGroupKind)
	SetAttachedRoutes(n uint)
}

type HTTPRouteReporter interface {
	ParentRef(parentRef *gwv1.ParentReference) ParentRefReporter
}

// TODO: rename to e.g. RouteParentReporter
type ParentRefReporter interface {
	SetCondition(condition HTTPRouteCondition)
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

type HTTPRouteCondition struct {
	Type    gwv1.RouteConditionType
	Status  metav1.ConditionStatus
	Reason  gwv1.RouteConditionReason
	Message string
}
