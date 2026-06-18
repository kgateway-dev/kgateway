package proxy_syncer

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func TestGatewayReportEqual(t *testing.T) {
	t.Run("ignores LastTransitionTime", func(t *testing.T) {
		a := makeGatewayReport(1, true)
		b := makeGatewayReport(1, false)
		if !gatewayReportEqual(a, b) {
			t.Fatal("expected gateway reports to be equal when only LastTransitionTime differs")
		}
	})

	t.Run("detects observed generation changes", func(t *testing.T) {
		a := makeGatewayReport(1, false)
		b := makeGatewayReport(2, false)
		if gatewayReportEqual(a, b) {
			t.Fatal("expected gateway reports to differ when observed generation differs")
		}
	})

	t.Run("detects attached listener set changes", func(t *testing.T) {
		a := makeGatewayReport(1, false)
		b := makeGatewayReport(1, false)
		b.SetAttachedListenerSets(2)
		if gatewayReportEqual(a, b) {
			t.Fatal("expected gateway reports to differ when attached ListenerSet count differs")
		}
	})

	t.Run("detects listener status changes", func(t *testing.T) {
		a := makeGatewayReport(1, false)
		b := makeGatewayReport(1, false)
		b.ListenerName("http").SetAttachedRoutes(3)
		if gatewayReportEqual(a, b) {
			t.Fatal("expected gateway reports to differ when listener status differs")
		}
	})
}

func TestListenerSetReportEqual(t *testing.T) {
	t.Run("ignores LastTransitionTime", func(t *testing.T) {
		a := makeListenerSetReport(1, true)
		b := makeListenerSetReport(1, false)
		if !listenerSetReportEqual(a, b) {
			t.Fatal("expected ListenerSet reports to be equal when only LastTransitionTime differs")
		}
	})

	t.Run("detects observed generation changes", func(t *testing.T) {
		a := makeListenerSetReport(1, false)
		b := makeListenerSetReport(2, false)
		if listenerSetReportEqual(a, b) {
			t.Fatal("expected ListenerSet reports to differ when observed generation differs")
		}
	})

	t.Run("detects top-level condition changes", func(t *testing.T) {
		a := makeListenerSetReport(1, false)
		b := makeListenerSetReport(1, false)
		b.SetCondition(reporter.GatewayCondition{
			Type:    gwv1.GatewayConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.GatewayReasonListenersNotValid,
			Message: "listener rejected",
		})
		if listenerSetReportEqual(a, b) {
			t.Fatal("expected ListenerSet reports to differ when top-level condition differs")
		}
	})

	t.Run("detects listener condition changes", func(t *testing.T) {
		a := makeListenerSetReport(1, false)
		b := makeListenerSetReport(1, false)
		b.ListenerName("http").SetCondition(reporter.ListenerCondition{
			Type:    gwv1.ListenerConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.ListenerReasonInvalid,
			Message: "listener invalid",
		})
		if listenerSetReportEqual(a, b) {
			t.Fatal("expected ListenerSet reports to differ when listener condition differs")
		}
	})
}

func TestBackendReportEqual(t *testing.T) {
	mk := func(generation int64, conditionMsg string) *reports.BackendReport {
		rm := reports.NewReportMap()
		r := reports.NewReporter(&rm)
		backend := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "backend", Generation: generation}}
		r.Backend(backend).SetCondition(reporter.BackendCondition{
			Type:    "Accepted",
			Status:  metav1.ConditionTrue,
			Reason:  "Accepted",
			Message: conditionMsg,
		})
		return rm.Backends[types.NamespacedName{Namespace: "default", Name: "backend"}]
	}

	t.Run("ignores LastTransitionTime", func(t *testing.T) {
		a := mk(1, "same")
		b := mk(1, "same")
		a.Conditions[0].LastTransitionTime = metav1.NewTime(time.Unix(1, 0))
		b.Conditions[0].LastTransitionTime = metav1.NewTime(time.Unix(2, 0))
		if !backendReportEqual(a, b) {
			t.Fatal("expected backend reports to be equal when only LastTransitionTime differs")
		}
	})

	t.Run("detects semantic condition changes", func(t *testing.T) {
		a := mk(1, "msg-a")
		b := mk(1, "msg-b")
		if backendReportEqual(a, b) {
			t.Fatal("expected backend reports to differ when condition message differs")
		}
	})

	t.Run("detects observed generation changes", func(t *testing.T) {
		a := mk(1, "same")
		b := mk(2, "same")
		if backendReportEqual(a, b) {
			t.Fatal("expected backend reports to differ when observed generation differs")
		}
	})
}

func TestRouteReportEqual(t *testing.T) {
	mk := func(generation int64) *reports.RouteReport {
		rm := reports.NewReportMap()
		r := reports.NewReporter(&rm)
		route := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "route", Generation: generation}}
		parent := gwv1.ParentReference{
			Group:     new(gwv1.Group("gateway.networking.k8s.io")),
			Kind:      new(gwv1.Kind("Gateway")),
			Name:      gwv1.ObjectName("gw"),
			Namespace: new(gwv1.Namespace("default")),
		}
		r.Route(route).ParentRef(&parent).SetCondition(reporter.RouteCondition{
			Type:    gwv1.RouteConditionAccepted,
			Status:  metav1.ConditionTrue,
			Reason:  gwv1.RouteReasonAccepted,
			Message: "accepted",
		})
		return rm.HTTPRoutes[types.NamespacedName{Namespace: "default", Name: "route"}]
	}

	t.Run("ignores LastTransitionTime", func(t *testing.T) {
		a := mk(1)
		b := mk(1)
		setFirstParentConditionTime(a, metav1.NewTime(time.Unix(1, 0)))
		setFirstParentConditionTime(b, metav1.NewTime(time.Unix(2, 0)))
		if !routeReportEqual(a, b) {
			t.Fatal("expected route reports to be equal when only LastTransitionTime differs")
		}
	})

	t.Run("detects observed generation changes", func(t *testing.T) {
		a := mk(1)
		b := mk(2)
		if routeReportEqual(a, b) {
			t.Fatal("expected route reports to differ when observed generation differs")
		}
	})

	t.Run("detects parent mismatch", func(t *testing.T) {
		a := mk(1)
		b := mk(1)
		for k := range b.Parents {
			delete(b.Parents, k)
			break
		}
		if routeReportEqual(a, b) {
			t.Fatal("expected route reports to differ when parent refs differ")
		}
	})
}

func TestPolicyReportEqual(t *testing.T) {
	mk := func(generation int64, state reporter.PolicyAttachmentState) *reports.PolicyReport {
		rm := reports.NewReportMap()
		r := reports.NewReporter(&rm)
		key := reporter.PolicyKey{Group: "g", Kind: "k", Namespace: "default", Name: "policy"}
		ancestor := gwv1.ParentReference{
			Group:     new(gwv1.Group("gateway.networking.k8s.io")),
			Kind:      new(gwv1.Kind("Gateway")),
			Name:      gwv1.ObjectName("gw"),
			Namespace: new(gwv1.Namespace("default")),
		}
		ar := r.Policy(key, generation).AncestorRef(ancestor)
		ar.SetCondition(reporter.PolicyCondition{
			Type:               "Accepted",
			Status:             metav1.ConditionTrue,
			Reason:             "Accepted",
			Message:            "accepted",
			ObservedGeneration: generation,
		})
		ar.SetAttachmentState(state)
		return rm.Policies[key]
	}

	t.Run("ignores LastTransitionTime", func(t *testing.T) {
		a := mk(1, reporter.PolicyAttachmentStateAttached)
		b := mk(1, reporter.PolicyAttachmentStateAttached)
		setFirstAncestorConditionTime(a, metav1.NewTime(time.Unix(1, 0)))
		setFirstAncestorConditionTime(b, metav1.NewTime(time.Unix(2, 0)))
		if !policyReportEqual(a, b) {
			t.Fatal("expected policy reports to be equal when only LastTransitionTime differs")
		}
	})

	t.Run("detects attachment state changes", func(t *testing.T) {
		a := mk(1, reporter.PolicyAttachmentStateAttached)
		b := mk(1, reporter.PolicyAttachmentStateMerged)
		if policyReportEqual(a, b) {
			t.Fatal("expected policy reports to differ when attachment state differs")
		}
	})

	t.Run("detects observed generation changes", func(t *testing.T) {
		a := mk(1, reporter.PolicyAttachmentStateAttached)
		b := mk(2, reporter.PolicyAttachmentStateAttached)
		if policyReportEqual(a, b) {
			t.Fatal("expected policy reports to differ when observed generation differs")
		}
	})
}

func setFirstParentConditionTime(r *reports.RouteReport, ts metav1.Time) {
	for _, parent := range r.Parents {
		if len(parent.Conditions) > 0 {
			parent.Conditions[0].LastTransitionTime = ts
		}
		return
	}
}

func makeGatewayReport(generation int64, forceTransition bool) *reports.GatewayReport {
	rm := reports.NewReportMap()
	statusReporter := reports.NewReporter(&rm)
	listener := gwv1.Listener{Name: "http"}
	gateway := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "default",
			Name:       "gw",
			Generation: generation,
		},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{listener},
		},
	}

	gatewayReporter := statusReporter.Gateway(gateway)
	if forceTransition {
		gatewayReporter.SetCondition(reporter.GatewayCondition{
			Type:    gwv1.GatewayConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.GatewayReasonListenersNotValid,
			Message: "before",
		})
		time.Sleep(time.Millisecond)
	}
	gatewayReporter.SetCondition(reporter.GatewayCondition{
		Type:    gwv1.GatewayConditionAccepted,
		Status:  metav1.ConditionTrue,
		Reason:  gwv1.GatewayReasonAccepted,
		Message: "accepted",
	})
	gatewayReporter.SetAttachedListenerSets(1)

	listenerReporter := gatewayReporter.Listener(&listener)
	if forceTransition {
		listenerReporter.SetCondition(reporter.ListenerCondition{
			Type:    gwv1.ListenerConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.ListenerReasonInvalid,
			Message: "before",
		})
		time.Sleep(time.Millisecond)
	}
	listenerReporter.SetCondition(reporter.ListenerCondition{
		Type:    gwv1.ListenerConditionAccepted,
		Status:  metav1.ConditionTrue,
		Reason:  gwv1.ListenerReasonAccepted,
		Message: "accepted",
	})
	listenerReporter.SetSupportedKinds([]gwv1.RouteGroupKind{{
		Group: new(gwv1.Group(wellknown.HTTPRouteGVK.Group)),
		Kind:  gwv1.Kind(wellknown.HTTPRouteGVK.Kind),
	}})
	listenerReporter.SetAttachedRoutes(2)

	return rm.Gateways[types.NamespacedName{Namespace: "default", Name: "gw"}]
}

func makeListenerSetReport(generation int64, forceTransition bool) *reports.ListenerSetReport {
	rm := reports.NewReportMap()
	statusReporter := reports.NewReporter(&rm)
	listener := gwv1.Listener{Name: "http"}
	listenerSet := &gwv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "default",
			Name:       "ls",
			Generation: generation,
		},
	}

	listenerSetReporter := statusReporter.ListenerSet(listenerSet)
	if forceTransition {
		listenerSetReporter.SetCondition(reporter.GatewayCondition{
			Type:    gwv1.GatewayConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.GatewayReasonListenersNotValid,
			Message: "before",
		})
		time.Sleep(time.Millisecond)
	}
	listenerSetReporter.SetCondition(reporter.GatewayCondition{
		Type:    gwv1.GatewayConditionAccepted,
		Status:  metav1.ConditionTrue,
		Reason:  gwv1.GatewayReasonAccepted,
		Message: "accepted",
	})

	listenerReporter := listenerSetReporter.Listener(&listener)
	if forceTransition {
		listenerReporter.SetCondition(reporter.ListenerCondition{
			Type:    gwv1.ListenerConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.ListenerReasonInvalid,
			Message: "before",
		})
		time.Sleep(time.Millisecond)
	}
	listenerReporter.SetCondition(reporter.ListenerCondition{
		Type:    gwv1.ListenerConditionAccepted,
		Status:  metav1.ConditionTrue,
		Reason:  gwv1.ListenerReasonAccepted,
		Message: "accepted",
	})
	listenerReporter.SetSupportedKinds([]gwv1.RouteGroupKind{{
		Group: new(gwv1.Group(wellknown.HTTPRouteGVK.Group)),
		Kind:  gwv1.Kind(wellknown.HTTPRouteGVK.Kind),
	}})
	listenerReporter.SetAttachedRoutes(2)

	return rm.ListenerSet(listenerSet)
}

func setFirstAncestorConditionTime(r *reports.PolicyReport, ts metav1.Time) {
	for _, ancestor := range r.Ancestors {
		if len(ancestor.Conditions) > 0 {
			ancestor.Conditions[0].LastTransitionTime = ts
		}
		return
	}
}
