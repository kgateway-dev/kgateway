package listener

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/v2/pkg/reports"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func GroupNameHelper() *gwv1.Group {
	g := gwv1.Group(gwv1.GroupName)
	return &g
}

func TestValidate(t *testing.T) {
	gateway := simpleGw()
	listeners := gateway.Spec.Listeners
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)
	gatewayReporter := reporter.Gateway(gateway)

	validListeners := validateListeners(gateway, gatewayReporter)
	g := NewWithT(t)
	g.Expect(len(validListeners)).To(Equal(1))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, gateway, listeners, report, expectedStatuses)
}

func TestSimpleGWNoHostname(t *testing.T) {
	gateway := simpleGwNoHostname()
	listeners := gateway.Spec.Listeners
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)
	gatewayReporter := reporter.Gateway(gateway)

	validListeners := validateListeners(gateway, gatewayReporter)
	g := NewWithT(t)
	g.Expect(len(validListeners)).To(Equal(1))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, gateway, listeners, report, expectedStatuses)
}

func TestSimpleGWDuplicateNoHostname(t *testing.T) {
	gateway := simpleGwDuplicateNoHostname()
	listeners := gateway.Spec.Listeners
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)
	gatewayReporter := reporter.Gateway(gateway)

	validListeners := validateListeners(gateway, gatewayReporter)
	g := NewWithT(t)
	g.Expect(len(validListeners)).To(Equal(0))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
			},
		},
		"http2": {
			Name: "http2",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, gateway, listeners, report, expectedStatuses)
}

func TestSimpleListenerWithValidRouteKind(t *testing.T) {
	gateway := simpleGwValidRouteKind()
	listeners := gateway.Spec.Listeners
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)
	gatewayReporter := reporter.Gateway(gateway)

	validListeners := validateListeners(gateway, gatewayReporter)
	g := NewWithT(t)
	g.Expect(len(validListeners)).To(Equal(1))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, gateway, listeners, report, expectedStatuses)
}

func TestSimpleListenerWithInvalidRouteKind(t *testing.T) {
	gateway := simpleGwInvalidRouteKind()
	listeners := gateway.Spec.Listeners
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)
	gatewayReporter := reporter.Gateway(gateway)

	validListeners := validateListeners(gateway, gatewayReporter)
	g := NewWithT(t)
	g.Expect(len(validListeners)).To(Equal(0))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name:           "http",
			SupportedKinds: []gwv1.RouteGroupKind{},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionResolvedRefs),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonInvalidRouteKinds),
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, gateway, listeners, report, expectedStatuses)
}

func TestMultiListener(t *testing.T) {
	gateway := simpleGwMultiListener()
	listeners := gateway.Spec.Listeners
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)
	gatewayReporter := reporter.Gateway(gateway)

	validListeners := validateListeners(gateway, gatewayReporter)
	g := NewWithT(t)
	g.Expect(len(validListeners)).To(Equal(2))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
		},
		"http2": {
			Name: "http2",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, gateway, listeners, report, expectedStatuses)
}

func TestMultiListenerExplicitRoutes(t *testing.T) {
	gateway := simpleGwMultiListenerExplicitRoutes()
	listeners := gateway.Spec.Listeners
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)
	gatewayReporter := reporter.Gateway(gateway)

	validListeners := validateListeners(gateway, gatewayReporter)
	g := NewWithT(t)
	g.Expect(len(validListeners)).To(Equal(2))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
		},
		"http2": {
			Name: "http2",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, gateway, listeners, report, expectedStatuses)
}

func TestMultiListenerWithInavlidRoute(t *testing.T) {
	gateway := simpleGwMultiListenerWithInvalidListener()
	listeners := gateway.Spec.Listeners
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)
	gatewayReporter := reporter.Gateway(gateway)

	validListeners := validateListeners(gateway, gatewayReporter)
	g := NewWithT(t)
	g.Expect(len(validListeners)).To(Equal(1))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
		},
		"http2": {
			Name:           "http2",
			SupportedKinds: []gwv1.RouteGroupKind{},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionResolvedRefs),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonInvalidRouteKinds),
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, gateway, listeners, report, expectedStatuses)
}

func TestProtocolConflict(t *testing.T) {
	gateway := protocolConfGw()
	listeners := gateway.Spec.Listeners
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)
	gatewayReporter := reporter.Gateway(gateway)

	validListeners := validateListeners(gateway, gatewayReporter)
	g := NewWithT(t)
	g.Expect(len(validListeners)).To(Equal(0))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonProtocolConflict),
				},
			},
		},
		"https": {
			Name: "https",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonProtocolConflict),
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, gateway, listeners, report, expectedStatuses)
}

func TestProtocolConflictInvalidRoutes(t *testing.T) {
	gateway := protocolConfGwWithInvalidRoute()
	listeners := gateway.Spec.Listeners
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)
	gatewayReporter := reporter.Gateway(gateway)

	validListeners := validateListeners(gateway, gatewayReporter)
	g := NewWithT(t)
	g.Expect(len(validListeners)).To(Equal(1))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name:           "http",
			SupportedKinds: []gwv1.RouteGroupKind{},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionResolvedRefs),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonInvalidRouteKinds),
				},
			},
		},
		"https": {
			Name: "https",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, gateway, listeners, report, expectedStatuses)
}

func TestActualProtocolConflictInvalidRoutes(t *testing.T) {
	gateway := actualProtocolConfGwWithInvalidRoute()
	listeners := gateway.Spec.Listeners
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)
	gatewayReporter := reporter.Gateway(gateway)

	validListeners := validateListeners(gateway, gatewayReporter)
	g := NewWithT(t)
	g.Expect(len(validListeners)).To(Equal(0))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http-with-invalid-route": {
			Name:           "http-with-invalid-route",
			SupportedKinds: []gwv1.RouteGroupKind{},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionResolvedRefs),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonInvalidRouteKinds),
				},
			},
		},
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonProtocolConflict),
				},
			},
		},
		"https": {
			Name: "https",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonProtocolConflict),
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, gateway, listeners, report, expectedStatuses)
}

func TestHostnameConflict(t *testing.T) {
	gateway := hostConfGw()
	listeners := gateway.Spec.Listeners
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)
	gatewayReporter := reporter.Gateway(gateway)

	validListeners := validateListeners(gateway, gatewayReporter)
	g := NewWithT(t)
	g.Expect(len(validListeners)).To(Equal(0))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
			},
		},
		"http2": {
			Name: "http2",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, gateway, listeners, report, expectedStatuses)
}

func TestHostnameConflictWithInvalidRoute(t *testing.T) {
	gateway := hostConfGwWithInvalidRoute()
	listeners := gateway.Spec.Listeners
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)
	gatewayReporter := reporter.Gateway(gateway)

	validListeners := validateListeners(gateway, gatewayReporter)
	g := NewWithT(t)
	g.Expect(len(validListeners)).To(Equal(1))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name:           "http",
			SupportedKinds: []gwv1.RouteGroupKind{},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionResolvedRefs),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonInvalidRouteKinds),
				},
			},
		},
		"http2": {
			Name: "http2",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, gateway, listeners, report, expectedStatuses)
}

func TestActualHostnameConflictWithInvalidRoute(t *testing.T) {
	gateway := actualHostConfGwWithInvalidRoute()
	listeners := gateway.Spec.Listeners
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)
	gatewayReporter := reporter.Gateway(gateway)

	validListeners := validateListeners(gateway, gatewayReporter)
	g := NewWithT(t)
	g.Expect(len(validListeners)).To(Equal(0))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http-with-invalid-route": {
			Name:           "http-with-invalid-route",
			SupportedKinds: []gwv1.RouteGroupKind{},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionResolvedRefs),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonInvalidRouteKinds),
				},
			},
		},
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
			},
		},
		"http2": {
			Name: "http2",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, gateway, listeners, report, expectedStatuses)
}

func TestHostnameConflictWithExtraGoodListener(t *testing.T) {
	gateway := hostConfGw2()
	listeners := gateway.Spec.Listeners
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)
	gatewayReporter := reporter.Gateway(gateway)

	validListeners := validateListeners(gateway, gatewayReporter)
	g := NewWithT(t)
	g.Expect(len(validListeners)).To(Equal(1))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
			},
		},
		"http2": {
			Name: "http2",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
			},
		},
		"http3": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, gateway, listeners, report, expectedStatuses)
}

func svc(ns string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "foo",
		},
	}
}

func getNN(obj client.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

func httpRoute(routeNs, backendNs string) gwv1.HTTPRoute {
	return gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: routeNs,
			Name:      "test",
		},
		Spec: gwv1.HTTPRouteSpec{
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name:      "foo",
									Namespace: (*gwv1.Namespace)(&backendNs),
								},
							},
						},
					},
				},
			},
		},
	}
}

// func TestRouteValidation(t *testing.T) {
// 	scheme := scheme.NewScheme()
// 	builder := fake.NewClientBuilder().WithScheme(scheme)
// 	query.IterateIndices(func(o client.Object, f string, fun client.IndexerFunc) error {
// 		builder.WithIndex(o, f, fun)
// 		return nil
// 	})
// 	fakeClient := fake.NewFakeClient(svc("default"))
// 	gq := query.NewData(fakeClient, scheme)

// 	report, _, routeMap := buildReporter()

// 	routes := []gwv1.HTTPRoute{httpRoute("default", "default")}
// 	validateRoutes(gq, report, routes)
// 	g := NewWithT(t)

// 	expectedStatuses := map[types.NamespacedName]*reports.RouteReport{}
// 	assertExpectedRouteStatuses(t, g, routes, routeMap, expectedStatuses)
// }

// func TestRouteValidationFailBackendNotFound(t *testing.T) {
// 	scheme := scheme.NewScheme()
// 	builder := fake.NewClientBuilder().WithScheme(scheme)
// 	query.IterateIndices(func(o client.Object, f string, fun client.IndexerFunc) error {
// 		builder.WithIndex(o, f, fun)
// 		return nil
// 	})
// 	fakeClient := fake.NewFakeClient()
// 	gq := query.NewData(fakeClient, scheme)

// 	report, _, routeMap := buildReporter()

// 	route := httpRoute("default", "default")
// 	routes := []gwv1.HTTPRoute{route}
// 	validateRoutes(gq, report, routes)
// 	g := NewWithT(t)

// 	expectedStatuses := map[types.NamespacedName]*reports.RouteReport{
// 		getNN(&route): {
// 			Conditions: []metav1.Condition{
// 				{
// 					Type:   string(gwv1.RouteConditionResolvedRefs),
// 					Status: metav1.ConditionFalse,
// 					Reason: string(gwv1.RouteReasonBackendNotFound),
// 				},
// 			},
// 		},
// 	}
// 	assertExpectedRouteStatuses(t, g, routes, routeMap, expectedStatuses)
// }

// func TestRouteValidationFailRefNotPermitted(t *testing.T) {
// 	scheme := scheme.NewScheme()
// 	builder := fake.NewClientBuilder().WithScheme(scheme)
// 	query.IterateIndices(func(o client.Object, f string, fun client.IndexerFunc) error {
// 		builder.WithIndex(o, f, fun)
// 		return nil
// 	})
// 	fakeClient := builder.WithObjects(svc("default2")).Build()
// 	gq := query.NewData(fakeClient, scheme)

// 	report, _, routeMap := buildReporter()

// 	route := httpRoute("default", "default2")
// 	routes := []gwv1.HTTPRoute{route}
// 	validateRoutes(gq, report, routes)
// 	g := NewWithT(t)

// 	expectedStatuses := map[types.NamespacedName]*reports.RouteReport{
// 		getNN(&route): {
// 			Conditions: []metav1.Condition{
// 				{
// 					Type:   string(gwv1.RouteConditionResolvedRefs),
// 					Status: metav1.ConditionFalse,
// 					Reason: string(gwv1.RouteReasonRefNotPermitted),
// 				},
// 			},
// 		},
// 	}
// 	assertExpectedRouteStatuses(t, g, routes, routeMap, expectedStatuses)
// }

// func assertExpectedRouteStatuses(
// 	t *testing.T,
// 	g Gomega,
// 	routes []gwv1.HTTPRoute,
// 	routeMap map[types.NamespacedName]*reports.RouteReport,
// 	expectedStatuses map[types.NamespacedName]*reports.RouteReport,
// ) {
// 	for _, route := range routes {
// 		routeKey := types.NamespacedName{
// 			Namespace: route.Namespace,
// 			Name:      route.Name,
// 		}
// 		routeReport := routeMap[routeKey]
// 		expectedStatus := expectedStatuses[routeKey]
// 		if expectedStatus == nil {
// 			g.Expect(routeReport).To(BeNil())
// 			continue
// 		}

// 		g.Expect(len(routeReport.Conditions)).To(Equal(len(expectedStatus.Conditions)))
// 		for _, eCond := range expectedStatus.Conditions {
// 			for _, aCond := range routeReport.Conditions {
// 				if eCond.Type == aCond.Type {
// 					g.Expect(aCond.Status).To(Equal(eCond.Status))
// 					g.Expect(aCond.Reason).To(Equal(eCond.Reason))
// 				}
// 			}
// 		}
// 	}
// }

func assertExpectedListenerStatuses(
	t *testing.T,
	g Gomega,
	gw *gwv1.Gateway,
	listeners []gwv1.Listener,
	reportMap reports.ReportMap,
	expectedStatuses map[string]gwv1.ListenerStatus,
) {
	if len(listeners) != len(expectedStatuses) {
		t.Fatalf("incorrect test setup, `expectedStatuses` MUST be set for all listeners, %d listeners, %d expectedStatuses",
			len(listeners),
			len(expectedStatuses))
	}
	gatewayReport := reportMap.Gateway(gw)

	for _, listener := range listeners {
		listenerName := string(listener.Name)
		listenerReporter := gatewayReport.Listener(&listener)
		actualReport := listenerReporter.(*reports.ListenerReport)
		expectedStatus := expectedStatuses[listenerName]
		g.Expect(actualReport.Status.Name).To(BeEquivalentTo(listenerName))
		g.Expect(actualReport.Status.SupportedKinds).To(BeEquivalentTo(expectedStatus.SupportedKinds))
		g.Expect(len(actualReport.Status.Conditions)).To(Equal(len(expectedStatus.Conditions)))
		for _, eCond := range expectedStatus.Conditions {
			for _, aCond := range actualReport.Status.Conditions {
				if eCond.Type == aCond.Type {
					g.Expect(aCond.Status).To(Equal(eCond.Status))
					g.Expect(aCond.Reason).To(Equal(eCond.Reason))
				}
			}
		}
	}
}

func simpleGw() *gwv1.Gateway {
	hostname := gwv1.Hostname("solo.io")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "solo",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
			},
		},
	}
}

func simpleGwNoHostname() *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "solo",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
			},
		},
	}
}

func simpleGwDuplicateNoHostname() *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "solo",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
				{
					Name:     "http2",
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
			},
		},
	}
}

func simpleGwValidRouteKind() *gwv1.Gateway {
	hostname := gwv1.Hostname("solo.io")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "solo",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{
								Kind: "HTTPRoute",
							},
						},
					},
				},
			},
		},
	}
}

func simpleGwInvalidRouteKind() *gwv1.Gateway {
	hostname := gwv1.Hostname("solo.io")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "solo",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{
								Kind: "BustedRouteKind",
							},
						},
					},
				},
			},
		},
	}
}

// TODO(Law): need to test & validate against duplicate Listener.Name fields?
func simpleGwMultiListener() *gwv1.Gateway {
	hostname := gwv1.Hostname("solo.io")
	hostname2 := gwv1.Hostname("test.solo.io")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "solo",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
				{
					Name:     "http2",
					Hostname: &hostname2,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
			},
		},
	}
}

func simpleGwMultiListenerExplicitRoutes() *gwv1.Gateway {
	hostname := gwv1.Hostname("solo.io")
	hostname2 := gwv1.Hostname("test.solo.io")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "solo",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{
								Kind: "HTTPRoute",
							},
						},
					},
				},
				{
					Name:     "http2",
					Hostname: &hostname2,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
			},
		},
	}
}

func simpleGwMultiListenerWithInvalidListener() *gwv1.Gateway {
	hostname := gwv1.Hostname("solo.io")
	hostname2 := gwv1.Hostname("test.solo.io")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "solo",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{
								Kind: "HTTPRoute",
							},
						},
					},
				},
				{
					Name:     "http2",
					Hostname: &hostname2,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{
								Kind: "VeryBadRoute",
							},
						},
					},
				},
			},
		},
	}
}

func protocolConfGw() *gwv1.Gateway {
	hostname := gwv1.Hostname("solo.io")
	hostname2 := gwv1.Hostname("test.solo.io")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "solo",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
				{
					Name:     "https",
					Hostname: &hostname2,
					Port:     8080,
					Protocol: gwv1.HTTPSProtocolType,
				},
			},
		},
	}
}

// TODO: Test multiple bad route kinds (and figure out how this fits into spec...)
func protocolConfGwWithInvalidRoute() *gwv1.Gateway {
	hostname := gwv1.Hostname("solo.io")
	hostname2 := gwv1.Hostname("test.solo.io")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "solo",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{
								Kind: "VeryBadRoute",
							},
						},
					},
				},
				{
					Name:     "https",
					Hostname: &hostname2,
					Port:     8080,
					Protocol: gwv1.HTTPSProtocolType,
				},
			},
		},
	}
}

func actualProtocolConfGwWithInvalidRoute() *gwv1.Gateway {
	hostname := gwv1.Hostname("solo.io")
	hostname2 := gwv1.Hostname("test.solo.io")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "solo",
			Listeners: []gwv1.Listener{
				{
					Name:     "http-with-invalid-route",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{
								Kind: "VeryBadRoute",
							},
						},
					},
				},
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{
								Kind: "HTTPRoute",
							},
						},
					},
				},
				{
					Name:     "https",
					Hostname: &hostname2,
					Port:     8080,
					Protocol: gwv1.HTTPSProtocolType,
				},
			},
		},
	}
}

func hostConfGw() *gwv1.Gateway {
	hostname := gwv1.Hostname("solo.io")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "solo",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
				{
					Name:     "http2",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
			},
		},
	}
}

func hostConfGwWithInvalidRoute() *gwv1.Gateway {
	hostname := gwv1.Hostname("solo.io")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "solo",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{
								Kind: "VeryBadRoute",
							},
						},
					},
				},
				{
					Name:     "http2",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
			},
		},
	}
}

func actualHostConfGwWithInvalidRoute() *gwv1.Gateway {
	hostname := gwv1.Hostname("solo.io")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "solo",
			Listeners: []gwv1.Listener{
				{
					Name:     "http-with-invalid-route",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{
								Kind: "VeryBadRoute",
							},
						},
					},
				},
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{
								Kind: "HTTPRoute",
							},
						},
					},
				},
				{
					Name:     "http2",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
			},
		},
	}
}

func hostConfGw2() *gwv1.Gateway {
	hostname := gwv1.Hostname("solo.io")
	hostname2 := gwv1.Hostname("test.solo.io")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "solo",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
				{
					Name:     "http2",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
				{
					Name:     "http3",
					Hostname: &hostname2,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
			},
		},
	}
}
