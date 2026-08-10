package reports

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	pluginreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

func TestBuildGWStatusDoesNotMutateReportMapEntry(t *testing.T) {
	rm := NewReportMap()
	rep := NewReporter(&rm)

	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "gw",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{{
				Name:     "http",
				Port:     80,
				Protocol: gwv1.HTTPProtocolType,
			}},
		},
	}

	rep.Gateway(gw).Listener(&gw.Spec.Listeners[0])
	gr := rm.GatewayNamespaceName(key(gw))
	require.NotNil(t, gr)
	beforeCond := slicesCloneConditions(gr.conditions)
	beforeListenerCond := slicesCloneConditions(gr.listeners["http"].Status.Conditions)

	status := rm.BuildGWStatus(*gw, nil)
	require.NotNil(t, status)

	require.Equal(t, beforeCond, gr.conditions, "BuildGWStatus must not mutate GatewayReport conditions")
	require.Equal(t, beforeListenerCond, gr.listeners["http"].Status.Conditions, "BuildGWStatus must not mutate ListenerReport conditions")
}

func TestBuildRouteStatusDoesNotMutateReportMapEntry(t *testing.T) {
	rm := NewReportMap()
	rep := NewReporter(&rm)

	parentRef := gwv1.ParentReference{
		Group:     new(gwv1.Group(gwv1.GroupVersion.Group)),
		Kind:      new(gwv1.Kind("Gateway")),
		Name:      "gw",
		Namespace: new(gwv1.Namespace("default")),
	}
	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "route",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{parentRef}},
		},
	}

	rep.Route(route).ParentRef(&parentRef).SetCondition(pluginreporter.RouteCondition{
		Type:   gwv1.RouteConditionAccepted,
		Status: metav1.ConditionFalse,
		Reason: gwv1.RouteReasonNoMatchingParent,
	})
	rr := rm.HTTPRoutes[types.NamespacedName{Namespace: route.Namespace, Name: route.Name}]
	before := cloneParentConditionsForTest(rr)

	status := rm.BuildRouteStatus(route, "kgateway.dev/kgateway")
	require.NotNil(t, status)

	require.Equal(t, before, cloneParentConditionsForTest(rr), "BuildRouteStatus must not mutate RouteReport condition slices")
}

func TestBuildPolicyStatusDoesNotMutateReportMapEntry(t *testing.T) {
	rm := NewReportMap()
	rep := NewReporter(&rm)

	policyKey := pluginreporter.PolicyKey{Group: "example.com", Kind: "Policy", Namespace: "default", Name: "policy"}
	ancestorRef := gwv1.ParentReference{
		Group:     new(gwv1.Group(gwv1.GroupVersion.Group)),
		Kind:      new(gwv1.Kind("Gateway")),
		Name:      "gw",
		Namespace: new(gwv1.Namespace("default")),
	}
	rep.Policy(policyKey, 1).AncestorRef(ancestorRef)
	pr := rm.Policies[policyKey]
	before := cloneAncestorConditionsForTest(pr)

	status := rm.BuildPolicyStatus(policyKey, "kgateway.dev/kgateway", gwv1.PolicyStatus{})
	require.NotNil(t, status)
	requireConditionForTest(t, status.Ancestors[0].Conditions, string(shared.PolicyConditionAttached))

	require.Equal(t, before, cloneAncestorConditionsForTest(pr), "BuildPolicyStatus must not mutate PolicyReport condition slices")
}

func TestBuildBackendStatusDoesNotMutateReportMapEntry(t *testing.T) {
	rm := NewReportMap()
	rep := NewReporter(&rm)
	backend := &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{Name: "backend", Namespace: "default", Generation: 1},
	}

	rep.Backend(backend).SetCondition(pluginreporter.BackendCondition{
		Type:   string(kgateway.BackendConditionAccepted),
		Status: metav1.ConditionTrue,
		Reason: string(kgateway.BackendReasonAccepted),
	})
	br := rm.backend(backend)
	before := cloneBackendReport(br)

	status := BuildBackendStatus(br, kgateway.BackendStatus{})
	require.NotNil(t, status)

	require.Equal(t, before, br, "BuildBackendStatus must not mutate BackendReport")
}

func TestBuildListenerSetStatusDoesNotMutateReportMapEntry(t *testing.T) {
	rm := NewReportMap()
	rep := NewReporter(&rm)
	listenerSet := &gwv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{Name: "listeners", Namespace: "default", Generation: 1},
		Spec: gwv1.ListenerSetSpec{Listeners: []gwv1.ListenerEntry{{
			Name:     "http",
			Port:     80,
			Protocol: gwv1.HTTPProtocolType,
		}}},
	}

	listenerSetReport := rep.ListenerSet(listenerSet)
	listenerSetReport.SetCondition(pluginreporter.GatewayCondition{
		Type:   gwv1.GatewayConditionAccepted,
		Status: metav1.ConditionTrue,
		Reason: gwv1.GatewayReasonAccepted,
	})
	listener := gwv1.Listener{
		Name:     listenerSet.Spec.Listeners[0].Name,
		Port:     listenerSet.Spec.Listeners[0].Port,
		Protocol: listenerSet.Spec.Listeners[0].Protocol,
	}
	listenerSetReport.Listener(&listener).SetCondition(pluginreporter.ListenerCondition{
		Type:   gwv1.ListenerConditionProgrammed,
		Status: metav1.ConditionTrue,
		Reason: gwv1.ListenerReasonProgrammed,
	})
	lsr := rm.ListenerSet(listenerSet)
	before := cloneListenerSetReport(lsr)

	status := BuildListenerSetStatus(lsr, *listenerSet)
	require.NotNil(t, status)

	require.Equal(t, before, lsr, "BuildListenerSetStatus must not mutate ListenerSetReport")
}

func slicesCloneConditions(in []metav1.Condition) []metav1.Condition {
	return append([]metav1.Condition(nil), in...)
}

func cloneParentConditionsForTest(rr *RouteReport) map[string][]metav1.Condition {
	out := map[string][]metav1.Condition{}
	for key, parent := range rr.Parents {
		out[key.String()] = slicesCloneConditions(parent.Conditions)
	}
	return out
}

func cloneAncestorConditionsForTest(pr *PolicyReport) map[string][]metav1.Condition {
	out := map[string][]metav1.Condition{}
	for key, ancestor := range pr.Ancestors {
		out[key.String()] = slicesCloneConditions(ancestor.Conditions)
	}
	return out
}

func requireConditionForTest(t *testing.T, conditions []metav1.Condition, conditionType string) {
	t.Helper()
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return
		}
	}
	t.Fatalf("expected condition %q", conditionType)
}

// Translation always reports against the normalized route type, but the status writer reads
// the live object through a client typed to whichever API version the cluster serves, and
// hands that object to the builder. Every such type must be understood: a missing case
// returns nil, which the writer cannot distinguish from "nothing to report", so it publishes
// an empty status and the merge clears every parent we own — erasing status instead of
// writing it.
func TestBuildRouteStatusSupportsEveryWrittenRouteType(t *testing.T) {
	parentRef := gwv1.ParentReference{
		Group:     new(gwv1.Group(gwv1.GroupVersion.Group)),
		Kind:      new(gwv1.Kind("Gateway")),
		Name:      "gw",
		Namespace: new(gwv1.Namespace("default")),
	}
	om := metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 1}
	common := gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{parentRef}}

	tests := map[string]struct {
		// reportedAs is the normalized type translation builds the report from.
		reportedAs client.Object
		// writtenAs is the served-version type the status writer reads and passes back in.
		writtenAs client.Object
	}{
		"HTTPRoute v1": {
			reportedAs: &gwv1.HTTPRoute{ObjectMeta: om, Spec: gwv1.HTTPRouteSpec{CommonRouteSpec: common}},
			writtenAs:  &gwv1.HTTPRoute{ObjectMeta: om, Spec: gwv1.HTTPRouteSpec{CommonRouteSpec: common}},
		},
		"GRPCRoute v1": {
			reportedAs: &gwv1.GRPCRoute{ObjectMeta: om, Spec: gwv1.GRPCRouteSpec{CommonRouteSpec: common}},
			writtenAs:  &gwv1.GRPCRoute{ObjectMeta: om, Spec: gwv1.GRPCRouteSpec{CommonRouteSpec: common}},
		},
		"TCPRoute written as v1alpha2": {
			reportedAs: &gwv1a2.TCPRoute{ObjectMeta: om, Spec: gwv1a2.TCPRouteSpec{CommonRouteSpec: common}},
			writtenAs:  &gwv1a2.TCPRoute{ObjectMeta: om, Spec: gwv1a2.TCPRouteSpec{CommonRouteSpec: common}},
		},
		"TCPRoute written as v1": {
			reportedAs: &gwv1a2.TCPRoute{ObjectMeta: om, Spec: gwv1a2.TCPRouteSpec{CommonRouteSpec: common}},
			writtenAs:  &gwv1.TCPRoute{ObjectMeta: om, Spec: gwv1.TCPRouteSpec{CommonRouteSpec: common}},
		},
		"TLSRoute written as v1alpha2": {
			reportedAs: &gwv1a2.TLSRoute{ObjectMeta: om, Spec: gwv1a2.TLSRouteSpec{CommonRouteSpec: common}},
			writtenAs:  &gwv1a2.TLSRoute{ObjectMeta: om, Spec: gwv1a2.TLSRouteSpec{CommonRouteSpec: common}},
		},
		"TLSRoute written as v1": {
			reportedAs: &gwv1a2.TLSRoute{ObjectMeta: om, Spec: gwv1a2.TLSRouteSpec{CommonRouteSpec: common}},
			writtenAs:  &gwv1.TLSRoute{ObjectMeta: om, Spec: gwv1.TLSRouteSpec{CommonRouteSpec: common}},
		},
		// Selected as the write version on Gateway API v1.4.x with experimental features on.
		"TLSRoute written as v1alpha3": {
			reportedAs: &gwv1a2.TLSRoute{ObjectMeta: om, Spec: gwv1a2.TLSRouteSpec{CommonRouteSpec: common}},
			writtenAs:  &gwv1a3.TLSRoute{ObjectMeta: om, Spec: gwv1.TLSRouteSpec{CommonRouteSpec: common}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rm := NewReportMap()
			NewReporter(&rm).Route(tc.reportedAs).ParentRef(&parentRef)
			report := rm.route(tc.reportedAs)
			require.NotNil(t, report, "translation must produce a report for the normalized type")

			status := BuildRouteStatus(report, tc.writtenAs, "kgateway.dev/kgateway")

			require.NotNil(t, status, "an unsupported route type erases status instead of writing it")
			require.Len(t, status.Parents, 1)
			require.Equal(t, gwv1.GatewayController("kgateway.dev/kgateway"), status.Parents[0].ControllerName)
		})
	}
}
