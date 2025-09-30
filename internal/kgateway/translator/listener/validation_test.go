package listener

import (
	"testing"

	. "github.com/onsi/gomega"
	istioprotocol "istio.io/istio/pkg/config/protocol"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/utils"
	reporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"
)

func GroupNameHelper() *gwv1.Group {
	g := gwv1.Group(gwv1.GroupName)
	return &g
}

func gwToIr(gw *gwv1.Gateway, allowedLS, deniedLS *gwxv1a1.XListenerSet) *ir.Gateway {
	out := &ir.Gateway{
		Obj:       gw,
		Listeners: make([]ir.Listener, 0, len(gw.Spec.Listeners)),
	}
	for _, l := range gw.Spec.Listeners {
		out.Listeners = append(out.Listeners, ir.Listener{
			Listener: l,
			Parent:   gw,
		})
	}
	if deniedLS != nil {
		out.DeniedListenerSets = []ir.ListenerSet{lsToIR(deniedLS)}
	}
	if allowedLS != nil {
		allowedIrLs := lsToIR(allowedLS)
		out.AllowedListenerSets = []ir.ListenerSet{allowedIrLs}
		out.Listeners = append(out.Listeners, allowedIrLs.Listeners...)
	}
	return out
}

func lsToIR(ls *gwxv1a1.XListenerSet) ir.ListenerSet {
	out := ir.ListenerSet{
		Obj:       ls,
		Listeners: make([]ir.Listener, 0, len(ls.Spec.Listeners)),
	}
	for _, l := range ls.Spec.Listeners {
		out.Listeners = append(out.Listeners, ir.Listener{
			Listener: utils.ToListener(l),
			Parent:   ls,
		})
	}
	return out
}

func TestValidate(t *testing.T) {
	gateway := simpleGw()
	listenerSet := simpleLs()
	deniedListenerSet := simpleLs()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, deniedListenerSet), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(2))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
			},
		},
	}

	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedStatuses)
	actualConditions := report.ListenerSet(deniedListenerSet).GetConditions()
	g.Expect(report.ListenerSet(deniedListenerSet).GetConditions()).To(Equal([]metav1.Condition{
		{
			Type:               string(gwv1.GatewayConditionAccepted),
			Status:             metav1.ConditionFalse,
			Reason:             string(gwv1.GatewayConditionReason(gwxv1a1.ListenerSetReasonNotAllowed)),
			LastTransitionTime: actualConditions[0].LastTransitionTime,
		},
		{
			Type:               string(gwv1.GatewayConditionProgrammed),
			Status:             metav1.ConditionFalse,
			Reason:             string(gwv1.GatewayConditionReason(gwxv1a1.ListenerSetReasonNotAllowed)),
			LastTransitionTime: actualConditions[1].LastTransitionTime,
		},
	}))
}

func TestSimpleGWNoHostname(t *testing.T) {
	gateway := simpleGwNoHostname()
	listenerSet := simpleLsNoHostname()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(2))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedStatuses)
}

func TestSimpleGWDuplicateNoHostname(t *testing.T) {
	gateway := simpleGwDuplicateNoHostname()
	listenerSet := simpleLsDuplicateNoHostname()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(1))

	expectedGwStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
			},
			// The first conflicted listener should be accepted based on listener precedence
			Conditions: []metav1.Condition{},
		},
	}
	expectedLsStatuses := map[string]gwv1.ListenerStatus{
		"http2": {
			Name: "http2",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
				{
					Type:    string(gwv1.ListenerConditionAccepted),
					Status:  metav1.ConditionFalse,
					Reason:  string(gwv1.ListenerReasonHostnameConflict),
					Message: ListenerMessageHostnameConflict,
				},
				{
					Type:    string(gwv1.ListenerConditionProgrammed),
					Status:  metav1.ConditionFalse,
					Reason:  string(gwv1.ListenerReasonHostnameConflict),
					Message: ListenerMessageHostnameConflict,
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedGwStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedLsStatuses)
}

func TestSimpleListenerWithValidRouteKind(t *testing.T) {
	gateway := simpleGwValidRouteKind()
	listenerSet := simpleLsValidRouteKind()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(2))

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
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedStatuses)
}

func TestSimpleListenerWithInvalidRouteKind(t *testing.T) {
	gateway := simpleGwInvalidRouteKind()
	listenerSet := simpleLsInvalidRouteKind()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(BeEmpty())

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name:           "http",
			SupportedKinds: []gwv1.RouteGroupKind{},
			Conditions: []metav1.Condition{
				{
					Type:    string(gwv1.ListenerConditionResolvedRefs),
					Status:  metav1.ConditionFalse,
					Reason:  string(gwv1.ListenerReasonInvalidRouteKinds),
					Message: "Found invalid route kinds: [BustedRouteKind]",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedStatuses)
}

func TestHBONEProtocol(t *testing.T) {
	gateway := hboneProtocolGw()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, nil, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(1))

	expectedGwStatuses := map[string]gwv1.ListenerStatus{
		"hbone": {
			Name: "hbone",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "TCPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "TLSRoute",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedGwStatuses)
}

func TestUnsupportedProtocol(t *testing.T) {
	gateway := unsupportedProtocolGw()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, nil, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(BeEmpty())

	expectedGwStatuses := map[string]gwv1.ListenerStatus{
		"udp": {
			Name:           "udp",
			SupportedKinds: []gwv1.RouteGroupKind{},
			Conditions: []metav1.Condition{
				{
					Type:    string(gwv1.ListenerConditionAccepted),
					Status:  metav1.ConditionFalse,
					Reason:  string(gwv1.ListenerReasonUnsupportedProtocol),
					Message: "Protocol UDP is unsupported.",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedGwStatuses)
}

func TestMultiListener(t *testing.T) {
	gateway := simpleGwMultiListener()
	listenerSet := simpleLsMultiListener()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(4))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
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
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedStatuses)
}

func TestMultiListenerExplicitRoutes(t *testing.T) {
	gateway := simpleGwMultiListenerExplicitRoutes()
	listenerSet := simpleLsMultiListenerExplicitRoutes()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(4))

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
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedStatuses)
}

func TestMultiListenerWithInavlidRoute(t *testing.T) {
	gateway := simpleGwMultiListenerWithInvalidListener()
	listenerSet := simpleLsMultiListenerWithInvalidListener()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(2))

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
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedStatuses)
}

func TestProtocolConflict(t *testing.T) {
	gateway := protocolConfGw()
	listenerSet := protocolConfLs()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(1))

	expectedGwStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
			},
			// The first conflicted listener should be accepted based on listener precedence
			Conditions: []metav1.Condition{},
		},
	}
	expectedLsStatuses := map[string]gwv1.ListenerStatus{
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
					Type:    string(gwv1.ListenerConditionConflicted),
					Status:  metav1.ConditionTrue,
					Reason:  string(gwv1.ListenerReasonProtocolConflict),
					Message: ListenerMessageProtocolConflict,
				},
				{
					Type:    string(gwv1.ListenerConditionAccepted),
					Status:  metav1.ConditionFalse,
					Reason:  string(gwv1.ListenerReasonProtocolConflict),
					Message: ListenerMessageProtocolConflict,
				},
				{
					Type:    string(gwv1.ListenerConditionProgrammed),
					Status:  metav1.ConditionFalse,
					Reason:  string(gwv1.ListenerReasonProtocolConflict),
					Message: ListenerMessageProtocolConflict,
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedGwStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedLsStatuses)
}

func TestProtocolConflictInvalidRoutes(t *testing.T) {
	gateway := protocolConfGwWithInvalidRoute()
	listenerSet := protocolConfLsWithInvalidRoute()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(1))

	expectedGwStatuses := map[string]gwv1.ListenerStatus{
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
	expectedLsStatuses := map[string]gwv1.ListenerStatus{
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
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedGwStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedLsStatuses)
}

func TestActualProtocolConflictInvalidRoutes(t *testing.T) {
	gateway := actualProtocolConfGwWithInvalidRoute()
	listenerSet := actualProtocolConfLsWithInvalidRoute()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(1))

	expectedGwStatuses := map[string]gwv1.ListenerStatus{
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
			// The first conflicted listener should be accepted based on listener precedence
			Conditions: []metav1.Condition{},
		},
	}
	expectedLsStatuses := map[string]gwv1.ListenerStatus{
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
				{
					Type:   string(gwv1.ListenerConditionAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonProtocolConflict),
				},
				{
					Type:   string(gwv1.ListenerConditionProgrammed),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonProtocolConflict),
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedGwStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedLsStatuses)
}

func TestHostnameConflict(t *testing.T) {
	gateway := hostConfGw()
	listenerSet := hostConfLs()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(1))

	expectedGwStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
			},
			// The first conflicted listener should be accepted based on listener precedence
			Conditions: []metav1.Condition{},
		},
	}
	expectedLsStatuses := map[string]gwv1.ListenerStatus{
		"http2": {
			Name: "http2",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
				{
					Type:   string(gwv1.ListenerConditionAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
				{
					Type:   string(gwv1.ListenerConditionProgrammed),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedGwStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedLsStatuses)
}

func TestHostnameConflictWithInvalidRoute(t *testing.T) {
	gateway := hostConfGwWithInvalidRoute()
	listenerSet := hostConfLsWithInvalidRoute()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(1))

	expectedGwStatuses := map[string]gwv1.ListenerStatus{
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
	expectedLsStatuses := map[string]gwv1.ListenerStatus{
		"http2": {
			Name: "http2",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedGwStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedLsStatuses)
}

func TestActualHostnameConflictWithInvalidRoute(t *testing.T) {
	gateway := actualHostConfGwWithInvalidRoute()
	listenerSet := actualHostConfLsWithInvalidRoute()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(1))

	expectedGwStatuses := map[string]gwv1.ListenerStatus{
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
			// The first conflicted listener should be accepted based on listener precedence
			Conditions: []metav1.Condition{},
		},
	}
	expectedLsStatuses := map[string]gwv1.ListenerStatus{
		"http2": {
			Name: "http2",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
				{
					Type:   string(gwv1.ListenerConditionAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
				{
					Type:   string(gwv1.ListenerConditionProgrammed),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedGwStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedLsStatuses)
}

func TestHostnameConflictWithExtraGoodListener(t *testing.T) {
	gateway := hostConfGw2()
	listenerSet := hostConfLs2()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(3))

	expectedGwStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
			},
			// The first conflicted listener should be accepted based on listener precedence
			Conditions: []metav1.Condition{},
		},
		"http3": {
			Name: "http3",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
			},
		},
	}
	expectedLsStatuses := map[string]gwv1.ListenerStatus{
		"http2": {
			Name: "http2",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
				{
					Type:   string(gwv1.ListenerConditionAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
				{
					Type:   string(gwv1.ListenerConditionProgrammed),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
			},
		},
		"http4": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedGwStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedLsStatuses)
}

func TestValidTCPRouteListener(t *testing.T) {
	gateway := simpleGwTCPRoute()
	listenerSet := simpleLsTCPRoute()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(2))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"tcp": {
			Name: "tcp",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "TCPRoute",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedStatuses)
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedStatuses)
}

func TestInvalidRouteKindOnTCPListener(t *testing.T) {
	gateway := simpleGwInvalidTCPRouteKind()
	listenerSet := simpleLsInvalidTCPRouteKind()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(BeEmpty())

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"tcp": {
			Name:           "tcp",
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
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedStatuses)
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedStatuses)
}

func TestTCPProtocolConflict(t *testing.T) {
	gateway := tcpProtocolConflictGw()
	listenerSet := tcpProtocolConflictLs()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(1))

	expectedGwStatuses := map[string]gwv1.ListenerStatus{
		"tcp": {
			Name: "tcp",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "TCPRoute",
				},
			},
			// The first conflicted listener should be accepted based on listener precedence
			Conditions: []metav1.Condition{},
		},
	}
	expectedLsStatuses := map[string]gwv1.ListenerStatus{
		"http": {
			Name: "http",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "HTTPRoute",
				},
				{
					Group: GroupNameHelper(),
					Kind:  "GRPCRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonProtocolConflict),
				},
				{
					Type:   string(gwv1.ListenerConditionAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonProtocolConflict),
				},
				{
					Type:   string(gwv1.ListenerConditionProgrammed),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonProtocolConflict),
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedGwStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedLsStatuses)
}

func TestTCPHostnameConflict(t *testing.T) {
	gateway := tcpHostnameConflictGw()
	listenerSet := tcpHostnameConflictLs()
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, listenerSet, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(1))

	expectedGwStatuses := map[string]gwv1.ListenerStatus{
		"tcp": {
			Name: "tcp",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "TCPRoute",
				},
			},
			// The first conflicted listener should be accepted based on listener precedence
			Conditions: []metav1.Condition{},
		},
	}
	expectedLsStatuses := map[string]gwv1.ListenerStatus{
		"tcp2": {
			Name: "tcp2",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "TCPRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
				{
					Type:   string(gwv1.ListenerConditionAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
				{
					Type:   string(gwv1.ListenerConditionProgrammed),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonHostnameConflict),
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, report.Gateway(gateway), gateway.Spec.Listeners, expectedGwStatuses)
	assertExpectedListenerStatuses(t, g, report.ListenerSet(listenerSet), utils.ToListenerSlice(listenerSet.Spec.Listeners), expectedLsStatuses)
}

func simpleGwTCPRoute() *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "tcp-gateway",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
			Listeners: []gwv1.Listener{
				{
					Name:     "tcp",
					Port:     8080,
					Protocol: gwv1.TCPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{
								Kind: "TCPRoute",
							},
						},
					},
				},
			},
		},
	}
}

func simpleLsTCPRoute() *gwxv1a1.XListenerSet {
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "tcp-listenerset",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
				{
					Name:     "tcp",
					Port:     8081,
					Protocol: gwv1.TCPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{
								Kind: "TCPRoute",
							},
						},
					},
				},
			},
		},
	}
}

func simpleGwInvalidTCPRouteKind() *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "tcp-invalid-gateway",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
			Listeners: []gwv1.Listener{
				{
					Name:     "tcp",
					Port:     8080,
					Protocol: gwv1.TCPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{
								Kind: "InvalidRouteKind",
							},
						},
					},
				},
			},
		},
	}
}

func simpleLsInvalidTCPRouteKind() *gwxv1a1.XListenerSet {
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "tcp-invalid-listenerset",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
				{
					Name:     "tcp",
					Port:     8081,
					Protocol: gwv1.TCPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{
								Kind: "InvalidRouteKind",
							},
						},
					},
				},
			},
		},
	}
}

func tcpProtocolConflictGw() *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "tcp-conflict-gateway",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
			Listeners: []gwv1.Listener{
				{
					Name:     "tcp",
					Port:     8080,
					Protocol: gwv1.TCPProtocolType,
				},
			},
		},
	}
}

func tcpProtocolConflictLs() *gwxv1a1.XListenerSet {
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "tcp-conflict-listenerset",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
				{
					Name:     "http",
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
			},
		},
	}
}

func tcpHostnameConflictGw() *gwv1.Gateway {
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "tcp-hostname-conflict-gateway",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
			Listeners: []gwv1.Listener{
				{
					Name:     "tcp",
					Port:     8080,
					Protocol: gwv1.TCPProtocolType,
					Hostname: &hostname,
				},
			},
		},
	}
}

func tcpHostnameConflictLs() *gwxv1a1.XListenerSet {
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "tcp-hostname-conflict-listenerset",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
				{
					Name:     "tcp2",
					Port:     8080,
					Protocol: gwv1.TCPProtocolType,
					Hostname: &hostname,
				},
			},
		},
	}
}

func TestValidTLSRouteListener(t *testing.T) {
	gateway := simpleGwTLSRoute()
	listeners := gateway.Spec.Listeners
	report := reports.NewReportMap()
	reporter := reports.NewReporter(&report)

	validListeners := validateGateway(gwToIr(gateway, nil, nil), reporter)
	g := NewWithT(t)
	g.Expect(validListeners).To(HaveLen(1))

	expectedStatuses := map[string]gwv1.ListenerStatus{
		"tls": {
			Name: "tls",
			SupportedKinds: []gwv1.RouteGroupKind{
				{
					Group: GroupNameHelper(),
					Kind:  "TLSRoute",
				},
			},
		},
	}
	assertExpectedListenerStatuses(t, g, reporter.Gateway(gateway), listeners, expectedStatuses)
}

func simpleGwTLSRoute() *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "tls-gateway",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
			Listeners: []gwv1.Listener{
				{
					Name:     "tls",
					Port:     443,
					Protocol: gwv1.TLSProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{
							{
								Kind: "TLSRoute",
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

// 		g.Expect(routeReport.Conditions).To(HaveLen(len(expectedStatus.Conditions)))
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
	gatewayReport reporter.GatewayReporter,
	listeners []gwv1.Listener,
	expectedStatuses map[string]gwv1.ListenerStatus,
) {
	if len(listeners) != len(expectedStatuses) {
		t.Fatalf("incorrect test setup, `expectedStatuses` MUST be set for all listeners, %d listeners, %d expectedStatuses",
			len(listeners),
			len(expectedStatuses))
	}

	for _, listener := range listeners {
		listenerName := string(listener.Name)
		listenerReporter := gatewayReport.Listener(&listener)
		actualReport := listenerReporter.(*reports.ListenerReport)
		expectedStatus := expectedStatuses[listenerName]
		g.Expect(actualReport.Status.Name).To(BeEquivalentTo(listenerName))
		g.Expect(actualReport.Status.SupportedKinds).To(BeEquivalentTo(expectedStatus.SupportedKinds))
		g.Expect(actualReport.Status.Conditions).To(HaveLen(len(expectedStatus.Conditions)))
		for _, eCond := range expectedStatus.Conditions {
			for _, aCond := range actualReport.Status.Conditions {
				if eCond.Type == aCond.Type {
					g.Expect(aCond.Status).To(Equal(eCond.Status))
					g.Expect(aCond.Reason).To(Equal(eCond.Reason))
					if eCond.Message != "" {
						g.Expect(aCond.Message).To(Equal(eCond.Message))
					}
				}
			}
		}
	}
}

func simpleGw() *gwv1.Gateway {
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
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

func simpleLs() *gwxv1a1.XListenerSet {
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8081,
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
			GatewayClassName: "kgateway",
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

func simpleLsNoHostname() *gwxv1a1.XListenerSet {
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
				{
					Name:     "http",
					Port:     8081,
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
			GatewayClassName: "kgateway",
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

func simpleLsDuplicateNoHostname() *gwxv1a1.XListenerSet {
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
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
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
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

func simpleLsValidRouteKind() *gwxv1a1.XListenerSet {
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8081,
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
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
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

func simpleLsInvalidRouteKind() *gwxv1a1.XListenerSet {
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8081,
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
	hostname := gwv1.Hostname("kgateway.dev")
	hostname2 := gwv1.Hostname("test.kgateway.dev")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
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

func simpleLsMultiListener() *gwxv1a1.XListenerSet {
	hostname := gwv1.Hostname("kgateway.dev")
	hostname2 := gwv1.Hostname("test.kgateway.dev")
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8081,
					Protocol: gwv1.HTTPProtocolType,
				},
				{
					Name:     "http2",
					Hostname: &hostname2,
					Port:     8081,
					Protocol: gwv1.HTTPProtocolType,
				},
			},
		},
	}
}

func simpleGwMultiListenerExplicitRoutes() *gwv1.Gateway {
	hostname := gwv1.Hostname("kgateway.dev")
	hostname2 := gwv1.Hostname("test.kgateway.dev")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
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

func simpleLsMultiListenerExplicitRoutes() *gwxv1a1.XListenerSet {
	hostname := gwv1.Hostname("kgateway.dev")
	hostname2 := gwv1.Hostname("test.kgateway.dev")
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8081,
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
					Port:     8081,
					Protocol: gwv1.HTTPProtocolType,
				},
			},
		},
	}
}

func simpleGwMultiListenerWithInvalidListener() *gwv1.Gateway {
	hostname := gwv1.Hostname("kgateway.dev")
	hostname2 := gwv1.Hostname("test.kgateway.dev")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
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

func simpleLsMultiListenerWithInvalidListener() *gwxv1a1.XListenerSet {
	hostname := gwv1.Hostname("kgateway.dev")
	hostname2 := gwv1.Hostname("test.kgateway.dev")
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     8081,
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
					Port:     8081,
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
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
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

func protocolConfLs() *gwxv1a1.XListenerSet {
	hostname2 := gwv1.Hostname("test.kgateway.dev")
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
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
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
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
			},
		},
	}
}

func protocolConfLsWithInvalidRoute() *gwxv1a1.XListenerSet {
	hostname2 := gwv1.Hostname("test.kgateway.dev")
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
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
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
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
			},
		},
	}
}

func actualProtocolConfLsWithInvalidRoute() *gwxv1a1.XListenerSet {
	hostname2 := gwv1.Hostname("test.kgateway.dev")
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
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
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
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

func hostConfLs() *gwxv1a1.XListenerSet {
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
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
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
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
			},
		},
	}
}

func hostConfLsWithInvalidRoute() *gwxv1a1.XListenerSet {
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
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
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
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
			},
		},
	}
}

func actualHostConfLsWithInvalidRoute() *gwxv1a1.XListenerSet {
	hostname := gwv1.Hostname("kgateway.dev")
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
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
	hostname := gwv1.Hostname("kgateway.dev")
	hostname2 := gwv1.Hostname("test.kgateway.dev")
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
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

func hostConfLs2() *gwxv1a1.XListenerSet {
	hostname := gwv1.Hostname("kgateway.dev")
	hostname4 := gwv1.Hostname("ls.kgateway.dev")
	return &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwxv1a1.ListenerSetSpec{
			Listeners: []gwxv1a1.ListenerEntry{
				{
					Name:     "http2",
					Hostname: &hostname,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
				{
					Name:     "http4",
					Hostname: &hostname4,
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
			},
		},
	}
}

func unsupportedProtocolGw() *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "unsupported-protocol-gateway",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
			Listeners: []gwv1.Listener{
				{
					Name:     "udp",
					Port:     8080,
					Protocol: gwv1.UDPProtocolType,
				},
			},
		},
	}
}

func hboneProtocolGw() *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "hbone-protocol-gateway",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
			Listeners: []gwv1.Listener{
				{
					Name:     "hbone",
					Port:     8080,
					Protocol: gwv1.ProtocolType(istioprotocol.HBONE),
				},
			},
		},
	}
}
