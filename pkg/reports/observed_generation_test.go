package reports_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	pluginreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func TestGatewayStatusRefreshesObservedGenerationFromCurrentObject(t *testing.T) {
	rm := reports.NewReportMap()
	statusReporter := reports.NewReporter(&rm)

	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "example-gateway",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "http"},
			},
		},
	}

	statusReporter.Gateway(gw)

	gw.Generation = 2
	status := rm.BuildGWStatus(context.Background(), *gw, nil)
	require.NotNil(t, status)

	for _, condition := range status.Conditions {
		require.Equal(t, int64(2), condition.ObservedGeneration)
	}
	for _, listenerStatus := range status.Listeners {
		for _, condition := range listenerStatus.Conditions {
			require.Equal(t, int64(2), condition.ObservedGeneration)
		}
	}
}

func TestRouteStatusRefreshesObservedGenerationFromCurrentObject(t *testing.T) {
	rm := reports.NewReportMap()
	statusReporter := reports.NewReporter(&rm)

	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "example-route",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Group:     new(gwv1.Group(gwv1.GroupVersion.Group)),
						Kind:      ptr.To(gwv1.Kind("Gateway")),
						Name:      "example-gateway",
						Namespace: ptr.To(gwv1.Namespace("default")),
					},
				},
			},
		},
	}

	statusReporter.Route(route).ParentRef(&route.Spec.ParentRefs[0])

	route.Generation = 2
	status := rm.BuildRouteStatus(context.Background(), route, "kgateway.dev/kgateway")
	require.NotNil(t, status)
	require.Len(t, status.Parents, 1)

	for _, condition := range status.Parents[0].Conditions {
		require.Equal(t, int64(2), condition.ObservedGeneration)
	}
}

func TestPolicyStatusRefreshesObservedGenerationOnReporterReuse(t *testing.T) {
	rm := reports.NewReportMap()
	statusReporter := reports.NewReporter(&rm)

	key := pluginreporter.PolicyKey{
		Group:     gwv1.GroupVersion.Group,
		Kind:      "BackendTLSPolicy",
		Namespace: "default",
		Name:      "tls-policy",
	}
	ancestorRef := gwv1.ParentReference{
		Group:     new(gwv1.Group(gwv1.GroupVersion.Group)),
		Kind:      ptr.To(gwv1.Kind("Gateway")),
		Name:      "example-gateway",
		Namespace: ptr.To(gwv1.Namespace("default")),
	}

	statusReporter.Policy(key, 1).AncestorRef(ancestorRef)
	statusReporter.Policy(key, 2).AncestorRef(ancestorRef)

	status := rm.BuildPolicyStatus(context.Background(), key, "kgateway.dev/kgateway", gwv1.PolicyStatus{})
	require.NotNil(t, status)
	require.Len(t, status.Ancestors, 1)

	for _, condition := range status.Ancestors[0].Conditions {
		require.Equal(t, int64(2), condition.ObservedGeneration)
	}

	requireCondition(t, status.Ancestors[0].Conditions, string(shared.PolicyConditionAccepted), metav1.ConditionFalse, string(shared.PolicyReasonPending))
	requireCondition(t, status.Ancestors[0].Conditions, string(shared.PolicyConditionAttached), metav1.ConditionFalse, string(shared.PolicyReasonPending))
}

func requireCondition(
	t *testing.T,
	conditions []metav1.Condition,
	conditionType string,
	status metav1.ConditionStatus,
	reason string,
) {
	t.Helper()

	condition := metaFindStatusCondition(conditions, conditionType)
	require.NotNil(t, condition)
	require.Equal(t, status, condition.Status)
	require.Equal(t, reason, condition.Reason)
}

func metaFindStatusCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
