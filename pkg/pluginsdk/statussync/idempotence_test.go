package statussync

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// applyRouteStatus is the ApplyStatusFn for an HTTPRoute: it models what the API server
// stores after a status write.
func applyRouteStatus(current *gwv1.HTTPRoute, status gwv1.RouteStatus) *gwv1.HTTPRoute {
	next := current.DeepCopy()
	next.Status.RouteStatus = *status.DeepCopy()
	return next
}

// routeWriterWithDesired builds a writer with the production merge and the supplied desired
// status builder, so the tests below differ only in the builder's behavior.
func routeWriterWithDesired(desired func(*gwv1.HTTPRoute) (gwv1.RouteStatus, bool)) Writer[*gwv1.HTTPRoute, gwv1.RouteStatus] {
	return Writer[*gwv1.HTTPRoute, gwv1.RouteStatus]{
		Name:      "httpRoute",
		Desired:   desired,
		GetStatus: func(o *gwv1.HTTPRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
		Merge: func(current *gwv1.HTTPRoute, d gwv1.RouteStatus) gwv1.RouteStatus {
			return gwv1.RouteStatus{Parents: MergeRouteParentStatuses(ourController, current.Status.Parents, d.Parents)}
		},
	}
}

func acceptedCondition(at time.Time) metav1.Condition {
	return metav1.Condition{
		Type:               string(gwv1.RouteConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(gwv1.RouteReasonAccepted),
		LastTransitionTime: metav1.NewTime(at),
	}
}

// routeWithParents returns a route whose live status holds the given parents.
func routeWithParents(parents ...gwv1.RouteParentStatus) *gwv1.HTTPRoute {
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
		Status:     gwv1.HTTPRouteStatus{RouteStatus: gwv1.RouteStatus{Parents: parents}},
	}
}

// A builder that preserves LastTransitionTime for unchanged conditions — what every
// production builder does via meta.SetStatusCondition — reaches a fixed point after one
// write, which is what lets the informer echo terminate.
func TestCheckWriterIdempotentAcceptsATimestampPreservingBuilder(t *testing.T) {
	writer := routeWriterWithDesired(func(current *gwv1.HTTPRoute) (gwv1.RouteStatus, bool) {
		var conditions []metav1.Condition
		// Seed from the live conditions first, exactly as the production builders do, so
		// SetStatusCondition only moves the timestamp when the condition actually changed.
		for _, p := range current.Status.Parents {
			if string(p.ControllerName) == ourController && p.ParentRef.Name == "gw" {
				conditions = slices.Clone(p.Conditions)
			}
		}
		meta.SetStatusCondition(&conditions, acceptedCondition(time.Now()))
		return gwv1.RouteStatus{Parents: []gwv1.RouteParentStatus{{
			ParentRef:      parentRef("gw"),
			ControllerName: ourController,
			Conditions:     conditions,
		}}}, true
	})

	// A stale timestamp on a condition whose content is unchanged: the write must happen
	// once (the parent list is otherwise empty) and then settle.
	current := routeWithParents()

	require.NoError(t, CheckWriterIdempotent(writer, current, applyRouteStatus))
}

// The regression this harness exists for: a builder that stamps time.Now() unconditionally
// republishes a different status for every echo of its own write.
func TestCheckWriterIdempotentRejectsRegeneratedTimestamps(t *testing.T) {
	writer := routeWriterWithDesired(func(*gwv1.HTTPRoute) (gwv1.RouteStatus, bool) {
		return gwv1.RouteStatus{Parents: []gwv1.RouteParentStatus{{
			ParentRef:      parentRef("gw"),
			ControllerName: ourController,
			// No lookup of the live condition, so the timestamp moves on every build.
			Conditions: []metav1.Condition{acceptedCondition(time.Now())},
		}}}, true
	})

	err := CheckWriterIdempotent(writer, routeWithParents(), applyRouteStatus)

	require.ErrorContains(t, err, "not idempotent")
}

// A writer that reorders entries it does not own on every pass is the same failure in a
// different disguise: this is what an address- or parent-order disagreement with a peer
// controller looks like from our side.
func TestCheckWriterIdempotentRejectsUnstableForeignEntryOrder(t *testing.T) {
	writer := Writer[*gwv1.HTTPRoute, gwv1.RouteStatus]{
		Name: "httpRoute",
		Desired: func(*gwv1.HTTPRoute) (gwv1.RouteStatus, bool) {
			return gwv1.RouteStatus{Parents: []gwv1.RouteParentStatus{routeParent(ourController, "gw")}}, true
		},
		GetStatus: func(o *gwv1.HTTPRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
		Merge: func(current *gwv1.HTTPRoute, d gwv1.RouteStatus) gwv1.RouteStatus {
			// Reverse rather than sort: reversal has no fixed point, so it keeps flipping
			// for as long as the writer runs.
			foreign := slices.Clone(current.Status.Parents)
			slices.Reverse(foreign)
			return gwv1.RouteStatus{Parents: append(foreign, d.Parents...)}
		},
	}
	current := routeWithParents(
		routeParent(otherController, "their-gw-a"),
		routeParent(otherController, "their-gw-b"),
	)

	err := CheckWriterIdempotent(writer, current, applyRouteStatus)

	require.ErrorContains(t, err, "not idempotent")
}

// Nothing published means nothing to converge from: a writer that declines to write is
// trivially stable, and reporting it as a failure would make the harness unusable for the
// resources the pipeline deliberately skips.
func TestCheckWriterIdempotentIgnoresWritersThatPublishNothing(t *testing.T) {
	tests := map[string]Writer[*gwv1.HTTPRoute, gwv1.RouteStatus]{
		"no desired status": routeWriterWithDesired(func(*gwv1.HTTPRoute) (gwv1.RouteStatus, bool) {
			return gwv1.RouteStatus{}, false
		}),
		"already up to date": routeWriterWithDesired(func(*gwv1.HTTPRoute) (gwv1.RouteStatus, bool) {
			return gwv1.RouteStatus{Parents: []gwv1.RouteParentStatus{routeParent(ourController, "gw")}}, true
		}),
	}

	for name, writer := range tests {
		t.Run(name, func(t *testing.T) {
			current := routeWithParents(routeParent(ourController, "gw"))
			require.NoError(t, CheckWriterIdempotent(writer, current, applyRouteStatus))
		})
	}
}

// A harness whose apply does not store what the writer published would fail every writer,
// for a reason that has nothing to do with the writer. Report that separately.
func TestCheckWriterIdempotentRejectsAnApplyThatDropsTheStatus(t *testing.T) {
	writer := routeWriterWithDesired(func(*gwv1.HTTPRoute) (gwv1.RouteStatus, bool) {
		return gwv1.RouteStatus{Parents: []gwv1.RouteParentStatus{routeParent(ourController, "gw")}}, true
	})

	err := CheckWriterIdempotent(writer, routeWithParents(), func(current *gwv1.HTTPRoute, _ gwv1.RouteStatus) *gwv1.HTTPRoute {
		return current
	})

	require.ErrorContains(t, err, "apply did not store the published status")
}
