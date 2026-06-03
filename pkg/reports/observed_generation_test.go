package reports_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	pluginreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func TestBuildBackendStatusMergesAndPreservesConditions(t *testing.T) {
	rm := reports.NewReportMap()
	statusReporter := reports.NewReporter(&rm)

	backend := &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "example-backend",
			Namespace:  "default",
			Generation: 7,
		},
	}

	statusReporter.Backend(backend).SetCondition(pluginreporter.BackendCondition{
		Type:    string(kgateway.BackendConditionAccepted),
		Status:  metav1.ConditionFalse,
		Reason:  string(kgateway.BackendReasonInvalid),
		Message: "translation failed",
	})

	// current status carries a condition owned by us (with an existing
	// LastTransitionTime) and a condition owned by another controller.
	existingTransition := metav1.NewTime(time.Now().Add(-time.Minute).Truncate(time.Second))
	currentStatus := kgateway.BackendStatus{
		Conditions: []metav1.Condition{
			{
				Type:               string(kgateway.BackendConditionAccepted),
				Status:             metav1.ConditionFalse,
				Reason:             string(kgateway.BackendReasonInvalid),
				Message:            "translation failed",
				LastTransitionTime: existingTransition,
			},
			{
				Type:    "ForeignCondition",
				Status:  metav1.ConditionTrue,
				Reason:  "SomethingElse",
				Message: "owned by another controller",
			},
		},
	}

	status := rm.BuildBackendStatus(context.Background(), backend, currentStatus)
	require.NotNil(t, status)

	accepted := requireConditionExists(t, status.Conditions, string(kgateway.BackendConditionAccepted))
	require.Equal(t, metav1.ConditionFalse, accepted.Status)
	require.Equal(t, int64(7), accepted.ObservedGeneration)
	// unchanged condition should preserve its original LastTransitionTime
	require.Equal(t, existingTransition, accepted.LastTransitionTime)

	// foreign condition must be preserved
	requireConditionExists(t, status.Conditions, "ForeignCondition")
}

func requireConditionExists(t *testing.T, conditions []metav1.Condition, condType string) metav1.Condition {
	t.Helper()
	for _, c := range conditions {
		if c.Type == condType {
			return c
		}
	}
	t.Fatalf("expected condition %q to exist", condType)
	return metav1.Condition{}
}

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
						Kind:      new(gwv1.Kind("Gateway")),
						Name:      "example-gateway",
						Namespace: new(gwv1.Namespace("default")),
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
		Kind:      new(gwv1.Kind("Gateway")),
		Name:      "example-gateway",
		Namespace: new(gwv1.Namespace("default")),
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
