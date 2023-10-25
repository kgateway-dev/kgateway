package listener

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/projects/gateway2/reports"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func buildReporter() (reports.Reporter, map[string]*reports.GatewayReport) {
	gr := make(map[string]*reports.GatewayReport)
	r := reports.ReportMap{
		Gateways: gr,
	}
	report := reports.NewReporter(&r)
	return report, gr
}

func GroupNameHelper() *gwv1.Group {
	g := gwv1.Group(gwv1.GroupName)
	return &g
}
func TestValidate(t *testing.T) {
	gateway := simpleGw()
	listeners := gateway.Spec.Listeners
	report, gatewayMap := buildReporter()
	gatewayReporter := report.Gateway(gateway)

	validListeners := validateListeners(listeners, gatewayReporter)
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
	assertExpectedListenerStatuses(t, g, gateway.Name, listeners, gatewayMap, expectedStatuses)
}

func TestSimpleListenerWithValidRouteKind(t *testing.T) {
	gateway := simpleGwValidRouteKind()
	listeners := gateway.Spec.Listeners
	report, gatewayMap := buildReporter()
	gatewayReporter := report.Gateway(gateway)

	validListeners := validateListeners(listeners, gatewayReporter)
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
	assertExpectedListenerStatuses(t, g, gateway.Name, listeners, gatewayMap, expectedStatuses)
}

func TestSimpleListenerWithInvalidRouteKind(t *testing.T) {
	gateway := simpleGwInvalidRouteKind()
	listeners := gateway.Spec.Listeners
	report, gatewayMap := buildReporter()
	gatewayReporter := report.Gateway(gateway)

	validListeners := validateListeners(listeners, gatewayReporter)
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
	assertExpectedListenerStatuses(t, g, gateway.Name, listeners, gatewayMap, expectedStatuses)
}

func TestMultiListener(t *testing.T) {
	gateway := simpleGwMultiListener()
	listeners := gateway.Spec.Listeners
	report, gatewayMap := buildReporter()
	gatewayReporter := report.Gateway(gateway)

	validListeners := validateListeners(listeners, gatewayReporter)
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
	assertExpectedListenerStatuses(t, g, gateway.Name, listeners, gatewayMap, expectedStatuses)
}

func TestMultiListenerExplicitRoutes(t *testing.T) {
	gateway := simpleGwMultiListenerExplicitRoutes()
	listeners := gateway.Spec.Listeners
	report, gatewayMap := buildReporter()
	gatewayReporter := report.Gateway(gateway)

	validListeners := validateListeners(listeners, gatewayReporter)
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
	assertExpectedListenerStatuses(t, g, gateway.Name, listeners, gatewayMap, expectedStatuses)
}

func TestMultiListenerWithInavlidRoute(t *testing.T) {
	gateway := simpleGwMultiListenerWithInvalidListener()
	listeners := gateway.Spec.Listeners
	report, gatewayMap := buildReporter()
	gatewayReporter := report.Gateway(gateway)

	validListeners := validateListeners(listeners, gatewayReporter)
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
	assertExpectedListenerStatuses(t, g, gateway.Name, listeners, gatewayMap, expectedStatuses)
}

func TestProtocolConflict(t *testing.T) {
	gateway := protocolConfGw()
	listeners := gateway.Spec.Listeners
	report, gatewayMap := buildReporter()
	gatewayReporter := report.Gateway(gateway)

	validListeners := validateListeners(listeners, gatewayReporter)
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
	assertExpectedListenerStatuses(t, g, gateway.Name, listeners, gatewayMap, expectedStatuses)
}

func TestProtocolConflictInvalidRoutes(t *testing.T) {
	gateway := protocolConfGwWithInvalidRoute()
	listeners := gateway.Spec.Listeners
	report, gatewayMap := buildReporter()
	gatewayReporter := report.Gateway(gateway)

	validListeners := validateListeners(listeners, gatewayReporter)
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
	assertExpectedListenerStatuses(t, g, gateway.Name, listeners, gatewayMap, expectedStatuses)
}

func TestActualProtocolConflictInvalidRoutes(t *testing.T) {
	gateway := actualProtocolConfGwWithInvalidRoute()
	listeners := gateway.Spec.Listeners
	report, gatewayMap := buildReporter()
	gatewayReporter := report.Gateway(gateway)

	validListeners := validateListeners(listeners, gatewayReporter)
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
	assertExpectedListenerStatuses(t, g, gateway.Name, listeners, gatewayMap, expectedStatuses)
}

func TestHostnameConflict(t *testing.T) {
	gateway := hostConfGw()
	listeners := gateway.Spec.Listeners
	report, gatewayMap := buildReporter()
	gatewayReporter := report.Gateway(gateway)

	validListeners := validateListeners(listeners, gatewayReporter)
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
	assertExpectedListenerStatuses(t, g, gateway.Name, listeners, gatewayMap, expectedStatuses)
}

func TestHostnameConflictWithInvalidRoute(t *testing.T) {
	gateway := hostConfGwWithInvalidRoute()
	listeners := gateway.Spec.Listeners
	report, gatewayMap := buildReporter()
	gatewayReporter := report.Gateway(gateway)

	validListeners := validateListeners(listeners, gatewayReporter)
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
	assertExpectedListenerStatuses(t, g, gateway.Name, listeners, gatewayMap, expectedStatuses)
}

func TestActualHostnameConflictWithInvalidRoute(t *testing.T) {
	gateway := actualHostConfGwWithInvalidRoute()
	listeners := gateway.Spec.Listeners
	report, gatewayMap := buildReporter()
	gatewayReporter := report.Gateway(gateway)

	validListeners := validateListeners(listeners, gatewayReporter)
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
	assertExpectedListenerStatuses(t, g, gateway.Name, listeners, gatewayMap, expectedStatuses)
}

func TestHostnameConflictWithExtraGoodListener(t *testing.T) {
	gateway := hostConfGw2()
	listeners := gateway.Spec.Listeners
	report, gatewayMap := buildReporter()
	gatewayReporter := report.Gateway(gateway)

	validListeners := validateListeners(listeners, gatewayReporter)
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
	assertExpectedListenerStatuses(t, g, gateway.Name, listeners, gatewayMap, expectedStatuses)
}

func assertExpectedListenerStatuses(
	t *testing.T,
	g Gomega,
	gatewayName string,
	listeners []gwv1.Listener,
	gatewayMap map[string]*reports.GatewayReport,
	expectedStatuses map[string]gwv1.ListenerStatus,
) {
	gatewayReport := gatewayMap[gatewayName]
	g.Expect(len(gatewayReport.Listeners)).To(Equal(len(expectedStatuses)))

	for _, listener := range listeners {
		listenerName := string(listener.Name)
		actualReport := gatewayReport.Listeners[listenerName]

		expectedStatus, ok := expectedStatuses[listenerName]
		if !ok {
			t.Fatalf("didn't receive an expected status for listener '%s'", listenerName)
		}

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
