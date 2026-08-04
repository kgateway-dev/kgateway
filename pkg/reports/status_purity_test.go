package reports

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

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

	status := rm.BuildGWStatus(context.Background(), *gw, nil)
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

	status := rm.BuildRouteStatus(context.Background(), route, "kgateway.dev/kgateway")
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

	status := rm.BuildPolicyStatus(context.Background(), policyKey, "kgateway.dev/kgateway", gwv1.PolicyStatus{})
	require.NotNil(t, status)
	requireConditionForTest(t, status.Ancestors[0].Conditions, string(shared.PolicyConditionAttached))

	require.Equal(t, before, cloneAncestorConditionsForTest(pr), "BuildPolicyStatus must not mutate PolicyReport condition slices")
}

func TestMergeReportMapsOwnsRouteAndPolicyReports(t *testing.T) {
	routeKey := types.NamespacedName{Namespace: "default", Name: "route"}
	policyKey := pluginreporter.PolicyKey{Group: "example.com", Kind: "Policy", Namespace: "default", Name: "policy"}

	first := NewReportMap()
	firstReporter := NewReporter(&first)
	firstRoute := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: routeKey.Namespace, Name: routeKey.Name}}
	firstParent := gwv1.ParentReference{Name: "gw-a"}
	firstReporter.Route(firstRoute).ParentRef(&firstParent).SetCondition(pluginreporter.RouteCondition{
		Type:   gwv1.RouteConditionAccepted,
		Status: metav1.ConditionTrue,
		Reason: gwv1.RouteReasonAccepted,
	})
	firstAncestor := gwv1.ParentReference{Name: "policy-gw-a"}
	firstReporter.Policy(policyKey, 1).AncestorRef(firstAncestor).SetCondition(pluginreporter.PolicyCondition{
		Type:   string(shared.PolicyConditionAccepted),
		Status: metav1.ConditionTrue,
		Reason: string(shared.PolicyReasonValid),
	})

	second := NewReportMap()
	secondReporter := NewReporter(&second)
	secondRoute := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: routeKey.Namespace, Name: routeKey.Name}}
	secondParent := gwv1.ParentReference{Name: "gw-b"}
	secondReporter.Route(secondRoute).ParentRef(&secondParent).SetCondition(pluginreporter.RouteCondition{
		Type:   gwv1.RouteConditionResolvedRefs,
		Status: metav1.ConditionTrue,
		Reason: gwv1.RouteReasonResolvedRefs,
	})
	secondAncestor := gwv1.ParentReference{Name: "policy-gw-b"}
	secondReporter.Policy(policyKey, 1).AncestorRef(secondAncestor).SetAttachmentState(pluginreporter.PolicyAttachmentStateAttached)

	merged := MergeReportMaps(first, second)
	require.Len(t, merged.HTTPRoutes[routeKey].Parents, 2)
	require.Len(t, merged.Policies[policyKey].Ancestors, 2)

	for _, parent := range merged.HTTPRoutes[routeKey].Parents {
		parent.Conditions = append(parent.Conditions, metav1.Condition{Type: "mutated"})
		break
	}
	for _, ancestor := range merged.Policies[policyKey].Ancestors {
		ancestor.Conditions = append(ancestor.Conditions, metav1.Condition{Type: "mutated"})
		break
	}

	require.NotContains(t, conditionTypesFromParents(first.HTTPRoutes[routeKey]), "mutated")
	require.NotContains(t, conditionTypesFromParents(second.HTTPRoutes[routeKey]), "mutated")
	require.NotContains(t, conditionTypesFromAncestors(first.Policies[policyKey]), "mutated")
	require.NotContains(t, conditionTypesFromAncestors(second.Policies[policyKey]), "mutated")
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

func conditionTypesFromParents(rr *RouteReport) []string {
	var out []string
	for _, parent := range rr.Parents {
		for _, condition := range parent.Conditions {
			out = append(out, condition.Type)
		}
	}
	return out
}

func conditionTypesFromAncestors(pr *PolicyReport) []string {
	var out []string
	for _, ancestor := range pr.Ancestors {
		for _, condition := range ancestor.Conditions {
			out = append(out, condition.Type)
		}
	}
	return out
}
