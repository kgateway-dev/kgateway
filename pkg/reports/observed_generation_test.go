package reports_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	pluginreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// BuildGWStatus must stamp the generation the report was built for, not the
// live object's generation. The status syncer is triggered only by report
// changes and reads the live Gateway from a separate (controller-runtime)
// cache than the one translation uses (the istio KRT cache). If it stamped the
// live generation, a skew between those two caches could freeze
// observedGeneration: the report-change trigger fires once, the stale live read
// stamps the wrong generation, and nothing re-triggers the sync. Sourcing the
// generation from the report keeps the trigger and the stamp consistent.
func TestGatewayStatusStampsObservedGenerationFromReport(t *testing.T) {
	newGateway := func(generation int64) *gwv1.Gateway {
		return &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "example-gateway",
				Namespace:  "default",
				Generation: generation,
			},
			Spec: gwv1.GatewaySpec{
				Listeners: []gwv1.Listener{
					{Name: "http"},
				},
			},
		}
	}

	// Report ahead of the live object: translation observed generation 2 but the
	// syncer's cache still sees generation 1. This is the freeze case that
	// regressed in #14295 - the published status must reflect the report's
	// generation (2), not the stale live read (1).
	t.Run("report ahead of stale live object", func(t *testing.T) {
		rm := reports.NewReportMap()
		statusReporter := reports.NewReporter(&rm)

		gw := newGateway(2)
		statusReporter.Gateway(gw)

		// Simulate the syncer's cache lagging behind translation's.
		gw.Generation = 1
		status := rm.BuildGWStatus(context.Background(), *gw, nil)
		require.NotNil(t, status)

		requireAllObservedGenerations(t, status, 2)
	})

	// Report behind the live object: the syncer's cache already sees generation
	// 2, but translation has only processed generation 1. observedGeneration must
	// stay at the generation actually translated (1) rather than prematurely
	// claiming a generation whose status has not been computed yet.
	t.Run("report behind live object", func(t *testing.T) {
		rm := reports.NewReportMap()
		statusReporter := reports.NewReporter(&rm)

		gw := newGateway(1)
		statusReporter.Gateway(gw)

		gw.Generation = 2
		status := rm.BuildGWStatus(context.Background(), *gw, nil)
		require.NotNil(t, status)

		requireAllObservedGenerations(t, status, 1)
	})
}

func requireAllObservedGenerations(t *testing.T, status *gwv1.GatewayStatus, want int64) {
	t.Helper()

	for _, condition := range status.Conditions {
		require.Equal(t, want, condition.ObservedGeneration)
	}
	for _, listenerStatus := range status.Listeners {
		for _, condition := range listenerStatus.Conditions {
			require.Equal(t, want, condition.ObservedGeneration)
		}
	}
}

// BuildRouteStatus must stamp the generation the report was built for, not the
// live object's generation, for the same cross-cache reason as Gateway status:
// the route is re-read by the syncer from a separate cache than translation
// used, and stamping the live read can freeze observedGeneration on a skew.
func TestRouteStatusStampsObservedGenerationFromReport(t *testing.T) {
	newRoute := func(generation int64) *gwv1.HTTPRoute {
		return &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "example-route",
				Namespace:  "default",
				Generation: generation,
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
	}

	requireParentObservedGeneration := func(t *testing.T, status *gwv1.RouteStatus, want int64) {
		t.Helper()
		require.NotNil(t, status)
		require.Len(t, status.Parents, 1)
		for _, condition := range status.Parents[0].Conditions {
			require.Equal(t, want, condition.ObservedGeneration)
		}
	}

	// Report ahead of a stale live read: translation observed generation 2 but
	// the syncer's cache still returns generation 1. The published status must
	// reflect the report's generation (2).
	t.Run("report ahead of stale live object", func(t *testing.T) {
		rm := reports.NewReportMap()
		statusReporter := reports.NewReporter(&rm)

		route := newRoute(2)
		statusReporter.Route(route).ParentRef(&route.Spec.ParentRefs[0])

		route.Generation = 1
		status := rm.BuildRouteStatus(context.Background(), route, "kgateway.dev/kgateway")
		requireParentObservedGeneration(t, status, 2)
	})

	// Report behind the live read: observedGeneration must stay at the generation
	// actually translated (1) rather than prematurely claiming generation 2.
	t.Run("report behind live object", func(t *testing.T) {
		rm := reports.NewReportMap()
		statusReporter := reports.NewReporter(&rm)

		route := newRoute(1)
		statusReporter.Route(route).ParentRef(&route.Spec.ParentRefs[0])

		route.Generation = 2
		status := rm.BuildRouteStatus(context.Background(), route, "kgateway.dev/kgateway")
		requireParentObservedGeneration(t, status, 1)
	})
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

// BuildListenerSetStatus must stamp the generation the report was built for, not
// the live object's, for the same cross-cache reason as Gateway and Route
// status.
func TestListenerSetStatusStampsObservedGenerationFromReport(t *testing.T) {
	newListenerSet := func(generation int64) *gwv1.ListenerSet {
		ls := &gwv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test",
				Namespace:  "default",
				Generation: generation,
			},
		}
		ls.Spec.Listeners = []gwv1.ListenerEntry{
			{Name: "http"},
		}
		return ls
	}

	requireAllObservedGenerations := func(t *testing.T, status *gwv1.ListenerSetStatus, want int64) {
		t.Helper()
		require.NotNil(t, status)
		for _, condition := range status.Conditions {
			require.Equal(t, want, condition.ObservedGeneration)
		}
		for _, listenerStatus := range status.Listeners {
			for _, condition := range listenerStatus.Conditions {
				require.Equal(t, want, condition.ObservedGeneration)
			}
		}
	}

	// Report ahead of a stale live read: the published status must reflect the
	// report's generation (2), not the stale live read (1).
	t.Run("report ahead of stale live object", func(t *testing.T) {
		rm := reports.NewReportMap()
		statusReporter := reports.NewReporter(&rm)

		ls := newListenerSet(2)
		statusReporter.ListenerSet(ls)

		ls.Generation = 1
		status := rm.BuildListenerSetStatus(context.Background(), *ls)
		requireAllObservedGenerations(t, status, 2)
	})

	// Report behind the live read: observedGeneration must stay at the generation
	// actually translated (1).
	t.Run("report behind live object", func(t *testing.T) {
		rm := reports.NewReportMap()
		statusReporter := reports.NewReporter(&rm)

		ls := newListenerSet(1)
		statusReporter.ListenerSet(ls)

		ls.Generation = 2
		status := rm.BuildListenerSetStatus(context.Background(), *ls)
		requireAllObservedGenerations(t, status, 1)
	})
}
