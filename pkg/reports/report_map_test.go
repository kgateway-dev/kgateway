package reports

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

// testCondition builds a condition with all the fields EqualReportMaps cares about,
// plus a LastTransitionTime which it must ignore.
func testCondition(condType string, status metav1.ConditionStatus, reason, message string, observedGen int64, transition time.Time) metav1.Condition {
	return metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: observedGen,
		LastTransitionTime: metav1.NewTime(transition),
	}
}

var (
	testGatewayKey = types.NamespacedName{Namespace: "default", Name: "gw"}
	testRouteKey   = types.NamespacedName{Namespace: "default", Name: "route"}
	testLSKey      = types.NamespacedName{Namespace: "default", Name: "ls"}
	testPolicyKey  = reporter.PolicyKey{Group: "example.com", Kind: "Policy", Namespace: "default", Name: "policy"}
	testParentKey  = ParentRefKey{
		Group:          gwv1.GroupVersion.Group,
		Kind:           "Gateway",
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "gw"},
	}
)

// fullReportMap builds a ReportMap that exercises every report kind (gateways,
// nested listener sets, all four route kinds and policies) with the
// given LastTransitionTime stamped on every condition. Callers vary only the
// transition time to assert it is ignored by EqualReportMaps.
func fullReportMap(transition time.Time) ReportMap {
	rm := NewReportMap()

	rm.Gateways[testGatewayKey] = &GatewayReport{
		observedGeneration: 5,
		conditions: []metav1.Condition{
			testCondition(string(gwv1.GatewayConditionAccepted), metav1.ConditionTrue, "Accepted", "accepted", 5, transition),
		},
		listeners: map[string]*ListenerReport{
			"http": {Status: gwv1.ListenerStatus{
				Name:           "http",
				AttachedRoutes: 3,
				SupportedKinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}},
				Conditions: []metav1.Condition{
					testCondition(string(gwv1.ListenerConditionAccepted), metav1.ConditionTrue, "Accepted", "accepted", 5, transition),
				},
			}},
		},
	}

	rm.ListenerSets[wellknown.XListenerSetGVK] = map[types.NamespacedName]*ListenerSetReport{
		testLSKey: {
			observedGeneration: 4,
			conditions: []metav1.Condition{
				testCondition(string(gwv1.GatewayConditionAccepted), metav1.ConditionTrue, "Accepted", "accepted", 4, transition),
			},
			listeners: map[string]*ListenerReport{
				"http": {Status: gwv1.ListenerStatus{
					Name: "http",
					Conditions: []metav1.Condition{
						testCondition(string(gwv1.ListenerConditionProgrammed), metav1.ConditionTrue, "Programmed", "programmed", 4, transition),
					},
				}},
			},
		},
	}

	routeReport := func(gen int64) *RouteReport {
		return &RouteReport{
			observedGeneration: gen,
			Parents: map[ParentRefKey]*ParentRefReport{
				testParentKey: {Conditions: []metav1.Condition{
					testCondition(string(gwv1.RouteConditionAccepted), metav1.ConditionTrue, "Accepted", "accepted", gen, transition),
				}},
			},
		}
	}
	rm.HTTPRoutes[testRouteKey] = routeReport(1)
	rm.GRPCRoutes[testRouteKey] = routeReport(2)
	rm.TCPRoutes[testRouteKey] = routeReport(3)
	rm.TLSRoutes[testRouteKey] = routeReport(4)

	rm.Policies[testPolicyKey] = &PolicyReport{
		observedGeneration: 2,
		Ancestors: map[ParentRefKey]*AncestorRefReport{
			testParentKey: {
				AttachmentState: reporter.PolicyAttachmentStateAttached,
				Conditions: []metav1.Condition{
					testCondition("Accepted", metav1.ConditionTrue, "Valid", "valid", 2, transition),
				},
			},
		},
	}

	return rm
}

func TestEqualReportMaps_EmptyMapsAreEqual(t *testing.T) {
	assert.True(t, EqualReportMaps(NewReportMap(), NewReportMap()))
}

func TestEqualReportMaps_IdenticalMapsAreEqual(t *testing.T) {
	now := time.Now()
	assert.True(t, EqualReportMaps(fullReportMap(now), fullReportMap(now)))
}

// TestEqualReportMaps_IgnoresLastTransitionTime is the central promise: a status
// rebuild that changes only condition timestamps must not be treated as a change,
// otherwise KRT would churn on every reconcile.
func TestEqualReportMaps_IgnoresLastTransitionTime(t *testing.T) {
	a := fullReportMap(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	b := fullReportMap(time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC))
	assert.True(t, EqualReportMaps(a, b), "maps differing only in LastTransitionTime must be equal")
}

// TestEqualReportMaps_DetectsConditionFieldChanges asserts the fields that DO
// matter (Status/Reason/Message/ObservedGeneration) each break equality, even
// though LastTransitionTime does not.
func TestEqualReportMaps_DetectsConditionFieldChanges(t *testing.T) {
	now := time.Now()
	cases := map[string]func(*metav1.Condition){
		"status":             func(c *metav1.Condition) { c.Status = metav1.ConditionFalse },
		"reason":             func(c *metav1.Condition) { c.Reason = "Different" },
		"message":            func(c *metav1.Condition) { c.Message = "different" },
		"observedGeneration": func(c *metav1.Condition) { c.ObservedGeneration = 999 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			a := fullReportMap(now)
			b := fullReportMap(now)
			mutate(&b.Gateways[testGatewayKey].conditions[0])
			assert.False(t, EqualReportMaps(a, b), "change to condition %s must break equality", name)
		})
	}
}

func TestEqualReportMaps_DetectsConditionAddedOrRemoved(t *testing.T) {
	now := time.Now()

	a := fullReportMap(now)
	b := fullReportMap(now)
	b.Gateways[testGatewayKey].conditions = append(b.Gateways[testGatewayKey].conditions,
		testCondition(string(gwv1.GatewayConditionProgrammed), metav1.ConditionTrue, "Programmed", "programmed", 5, now))
	assert.False(t, EqualReportMaps(a, b), "an extra condition must break equality")

	// A condition of the same type but absent on one side must also break equality.
	c := fullReportMap(now)
	c.Gateways[testGatewayKey].conditions = nil
	assert.False(t, EqualReportMaps(a, c))
}

func TestEqualReportMaps_DetectsGatewayScalarChanges(t *testing.T) {
	now := time.Now()

	og := fullReportMap(now)
	og.Gateways[testGatewayKey].observedGeneration = 6
	assert.False(t, EqualReportMaps(fullReportMap(now), og), "observedGeneration change must break equality")
}

func TestEqualReportMaps_DetectsListenerStatusChanges(t *testing.T) {
	now := time.Now()

	routes := fullReportMap(now)
	routes.Gateways[testGatewayKey].listeners["http"].Status.AttachedRoutes = 100
	assert.False(t, EqualReportMaps(fullReportMap(now), routes), "AttachedRoutes change must break equality")

	kinds := fullReportMap(now)
	kinds.Gateways[testGatewayKey].listeners["http"].Status.SupportedKinds = []gwv1.RouteGroupKind{{Kind: "GRPCRoute"}}
	assert.False(t, EqualReportMaps(fullReportMap(now), kinds), "SupportedKinds change must break equality")

	added := fullReportMap(now)
	added.Gateways[testGatewayKey].listeners["https"] = &ListenerReport{Status: gwv1.ListenerStatus{Name: "https"}}
	assert.False(t, EqualReportMaps(fullReportMap(now), added), "extra listener must break equality")
}

// TestEqualReportMaps_ListenerSetGVKMaps covers the nested
// GVK -> NamespacedName -> report map comparison for ListenerSets.
func TestEqualReportMaps_ListenerSetGVKMaps(t *testing.T) {
	now := time.Now()

	t.Run("equal when content matches", func(t *testing.T) {
		assert.True(t, EqualReportMaps(fullReportMap(now), fullReportMap(now)))
	})

	t.Run("different GVK key", func(t *testing.T) {
		b := fullReportMap(now)
		report := b.ListenerSets[wellknown.XListenerSetGVK][testLSKey]
		delete(b.ListenerSets, wellknown.XListenerSetGVK)
		otherGVK := schema.GroupVersionKind{Group: "other.io", Version: "v1", Kind: "OtherListenerSet"}
		b.ListenerSets[otherGVK] = map[types.NamespacedName]*ListenerSetReport{testLSKey: report}
		assert.False(t, EqualReportMaps(fullReportMap(now), b), "moving a report under a different GVK must break equality")
	})

	t.Run("different NamespacedName within GVK", func(t *testing.T) {
		b := fullReportMap(now)
		report := b.ListenerSets[wellknown.XListenerSetGVK][testLSKey]
		delete(b.ListenerSets[wellknown.XListenerSetGVK], testLSKey)
		b.ListenerSets[wellknown.XListenerSetGVK][types.NamespacedName{Namespace: "default", Name: "other-ls"}] = report
		assert.False(t, EqualReportMaps(fullReportMap(now), b), "different listener set name must break equality")
	})

	t.Run("condition change within listener set", func(t *testing.T) {
		b := fullReportMap(now)
		b.ListenerSets[wellknown.XListenerSetGVK][testLSKey].conditions[0].Status = metav1.ConditionFalse
		assert.False(t, EqualReportMaps(fullReportMap(now), b))
	})

	t.Run("ignores LastTransitionTime within listener set", func(t *testing.T) {
		a := fullReportMap(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
		b := fullReportMap(now)
		// Wipe every difference except the listener-set timestamps, which differ via fullReportMap.
		assert.True(t, EqualReportMaps(a, b))
	})

	t.Run("extra GVK entry", func(t *testing.T) {
		b := fullReportMap(now)
		extraGVK := schema.GroupVersionKind{Group: "other.io", Version: "v1", Kind: "OtherListenerSet"}
		b.ListenerSets[extraGVK] = map[types.NamespacedName]*ListenerSetReport{
			testLSKey: {observedGeneration: 1},
		}
		assert.False(t, EqualReportMaps(fullReportMap(now), b))
	})
}

func TestEqualReportMaps_DetectsRouteChanges(t *testing.T) {
	now := time.Now()

	gen := fullReportMap(now)
	gen.HTTPRoutes[testRouteKey].observedGeneration = 100
	assert.False(t, EqualReportMaps(fullReportMap(now), gen), "route observedGeneration change must break equality")

	cond := fullReportMap(now)
	cond.TCPRoutes[testRouteKey].Parents[testParentKey].Conditions[0].Reason = "Different"
	assert.False(t, EqualReportMaps(fullReportMap(now), cond), "parent condition change must break equality")

	parent := fullReportMap(now)
	parent.GRPCRoutes[testRouteKey].Parents[ParentRefKey{Kind: "Gateway", NamespacedName: types.NamespacedName{Name: "extra"}}] = &ParentRefReport{}
	assert.False(t, EqualReportMaps(fullReportMap(now), parent), "extra parent must break equality")
}

func TestEqualReportMaps_DetectsPolicyChanges(t *testing.T) {
	now := time.Now()

	gen := fullReportMap(now)
	gen.Policies[testPolicyKey].observedGeneration = 100
	assert.False(t, EqualReportMaps(fullReportMap(now), gen), "policy observedGeneration change must break equality")

	state := fullReportMap(now)
	state.Policies[testPolicyKey].Ancestors[testParentKey].AttachmentState = reporter.PolicyAttachmentStateMerged
	assert.False(t, EqualReportMaps(fullReportMap(now), state), "ancestor AttachmentState change must break equality")

	cond := fullReportMap(now)
	cond.Policies[testPolicyKey].Ancestors[testParentKey].Conditions[0].Message = "different"
	assert.False(t, EqualReportMaps(fullReportMap(now), cond), "ancestor condition change must break equality")
}

// TestEqualReportMaps_NilReportPointers verifies that a nil report value compares
// distinct from a populated one but equal to another nil under the same key.
func TestEqualReportMaps_NilReportPointers(t *testing.T) {
	a := NewReportMap()
	a.Gateways[testGatewayKey] = nil
	b := NewReportMap()
	b.Gateways[testGatewayKey] = nil
	assert.True(t, EqualReportMaps(a, b), "two nil reports under the same key must be equal")

	c := fullReportMap(time.Now())
	d := NewReportMap()
	d.Gateways[testGatewayKey] = nil
	assert.False(t, EqualReportMaps(c, d), "nil vs populated report must not be equal")
}
