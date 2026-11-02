package statussync

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

const (
	ourController   = "kgateway.dev/kgateway"
	otherController = "other.example/controller"
)

func parentRef(name string) gwv1.ParentReference {
	return gwv1.ParentReference{Name: gwv1.ObjectName(name)}
}

func routeParent(controller, name string) gwv1.RouteParentStatus {
	return gwv1.RouteParentStatus{
		ParentRef:      parentRef(name),
		ControllerName: gwv1.GatewayController(controller),
		Conditions: []metav1.Condition{
			{Type: string(gwv1.RouteConditionAccepted), Status: metav1.ConditionTrue},
		},
	}
}

func TestMergeRouteParentStatusesPreservesOtherControllers(t *testing.T) {
	existing := []gwv1.RouteParentStatus{
		routeParent(otherController, "their-gw"),
		routeParent(ourController, "stale-gw"),
	}
	desired := []gwv1.RouteParentStatus{
		routeParent(ourController, "gw-b"),
		routeParent(ourController, "gw-a"),
	}

	merged := MergeRouteParentStatuses(ourController, existing, desired)

	require.Len(t, merged, 3, "should keep the other controller's entry and replace ours")
	// The other controller's entry keeps its place; our new entries follow in sorted order.
	require.Equal(t, []gwv1.ObjectName{"their-gw", "gw-a", "gw-b"},
		[]gwv1.ObjectName{merged[0].ParentRef.Name, merged[1].ParentRef.Name, merged[2].ParentRef.Name})
	require.Equal(t, gwv1.GatewayController(otherController), merged[0].ControllerName,
		"the other controller's entry must be preserved")
	for _, p := range merged {
		require.NotEqual(t, gwv1.ObjectName("stale-gw"), p.ParentRef.Name,
			"our stale entry must be dropped when absent from the desired status")
	}
}

func TestMergeRouteParentStatusesClearsAllOursOnEmptyDesired(t *testing.T) {
	existing := []gwv1.RouteParentStatus{
		routeParent(ourController, "gw"),
		routeParent(otherController, "their-gw"),
	}

	merged := MergeRouteParentStatuses(ourController, existing, nil)

	require.Len(t, merged, 1)
	require.Equal(t, gwv1.GatewayController(otherController), merged[0].ControllerName)
}

func TestMergeRouteParentStatusesDropsForeignDesiredEntries(t *testing.T) {
	// Only entries owned by our controller may be published from the desired status,
	// even if the builder accidentally carried other controllers' entries into it.
	desired := []gwv1.RouteParentStatus{
		routeParent(ourController, "gw"),
		routeParent(otherController, "their-gw"),
	}

	merged := MergeRouteParentStatuses(ourController, nil, desired)

	require.Len(t, merged, 1)
	require.Equal(t, gwv1.GatewayController(ourController), merged[0].ControllerName)
}

func ancestor(controller, name string) gwv1.PolicyAncestorStatus {
	return gwv1.PolicyAncestorStatus{
		AncestorRef:    parentRef(name),
		ControllerName: gwv1.GatewayController(controller),
	}
}

func TestMergePolicyAncestorStatuses(t *testing.T) {
	existing := []gwv1.PolicyAncestorStatus{
		ancestor(otherController, "their-gw"),
		ancestor(ourController, "stale-gw"),
	}
	desired := []gwv1.PolicyAncestorStatus{
		ancestor(ourController, "gw-b"),
		ancestor(ourController, "gw-a"),
	}

	merged := MergePolicyAncestorStatuses(ourController, existing, desired)

	require.Len(t, merged, 3)
	require.Equal(t, []gwv1.ObjectName{"their-gw", "gw-a", "gw-b"},
		[]gwv1.ObjectName{merged[0].AncestorRef.Name, merged[1].AncestorRef.Name, merged[2].AncestorRef.Name})
	require.Equal(t, gwv1.GatewayController(otherController), merged[0].ControllerName)
}

func TestMergePolicyAncestorStatusesClearsAllOursOnEmptyDesired(t *testing.T) {
	existing := []gwv1.PolicyAncestorStatus{
		ancestor(ourController, "gw"),
	}

	merged := MergePolicyAncestorStatuses(ourController, existing, nil)

	require.Empty(t, merged, "publishing an empty desired list must clear our stale entries")
}

func TestCompareParentReferenceCanonicalizesDefaults(t *testing.T) {
	group := gwv1.Group(gwv1.GroupName)
	kind := gwv1.Kind("Gateway")
	explicit := gwv1.ParentReference{Group: &group, Kind: &kind, Name: "gw"}
	implicit := gwv1.ParentReference{Name: "gw"}

	require.Zero(t, compareParentReference(explicit, implicit),
		"nil group/kind must compare equal to their explicit defaults")
}

func TestMergePolicyAncestorStatusesCapsAtGatewayAPILimit(t *testing.T) {
	// The API server enforces MaxItems=16 on PolicyStatus.ancestors; a merged list that
	// exceeds it would be rejected on every write and never self-heal.
	existing := make([]gwv1.PolicyAncestorStatus, 0, reports.MaxPolicyStatusAncestors)
	for i := range reports.MaxPolicyStatusAncestors {
		existing = append(existing, ancestor(otherController, fmt.Sprintf("their-gw-%02d", i)))
	}
	desired := []gwv1.PolicyAncestorStatus{ancestor(ourController, "our-gw")}

	merged := MergePolicyAncestorStatuses(ourController, existing, desired)

	require.Len(t, merged, reports.MaxPolicyStatusAncestors,
		"merged list must not exceed the Gateway API ancestors limit")
	for _, a := range merged {
		require.Equal(t, gwv1.GatewayController(otherController), a.ControllerName,
			"other controllers' entries must never be dropped in favor of ours")
	}
}

func TestMergePolicyAncestorStatusesCapKeepsOursWhileRoomRemains(t *testing.T) {
	existing := make([]gwv1.PolicyAncestorStatus, 0, reports.MaxPolicyStatusAncestors-1)
	for i := range reports.MaxPolicyStatusAncestors - 1 {
		existing = append(existing, ancestor(otherController, fmt.Sprintf("their-gw-%02d", i)))
	}
	desired := []gwv1.PolicyAncestorStatus{
		ancestor(ourController, "our-gw-b"),
		ancestor(ourController, "our-gw-a"),
	}

	merged := MergePolicyAncestorStatuses(ourController, existing, desired)

	require.Len(t, merged, reports.MaxPolicyStatusAncestors)
	ours := []gwv1.ObjectName{}
	for _, a := range merged {
		if a.ControllerName == gwv1.GatewayController(ourController) {
			ours = append(ours, a.AncestorRef.Name)
		}
	}
	require.Equal(t, []gwv1.ObjectName{"our-gw-a"}, ours,
		"our first sorted entry fills the remaining slot; the rest of ours are truncated")
}

func TestMergeRouteParentStatusesCapsAtGatewayAPILimit(t *testing.T) {
	// The API server enforces MaxItems=32 on RouteStatus.parents.
	existing := make([]gwv1.RouteParentStatus, 0, reports.MaxRouteStatusParents)
	for i := range reports.MaxRouteStatusParents {
		existing = append(existing, routeParent(otherController, fmt.Sprintf("their-gw-%02d", i)))
	}
	desired := []gwv1.RouteParentStatus{routeParent(ourController, "our-gw")}

	merged := MergeRouteParentStatuses(ourController, existing, desired)

	require.Len(t, merged, reports.MaxRouteStatusParents,
		"merged list must not exceed the Gateway API parents limit")
	for _, p := range merged {
		require.Equal(t, gwv1.GatewayController(otherController), p.ControllerName,
			"other controllers' entries must never be dropped in favor of ours")
	}
}

func TestRetryStatusWriteRetriesTransientErrors(t *testing.T) {
	attempts := 0
	err := retryStatusWrite(context.Background(), func() error {
		attempts++
		if attempts < 3 {
			return errors.New("transient")
		}
		return nil
	})
	require.NoError(t, err, "a write succeeding within the retry budget must not surface an error")
	require.Equal(t, 3, attempts, "transient failures must be retried")
}

func TestRetryStatusWriteStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	err := retryStatusWrite(ctx, func() error {
		attempts++
		cancel()
		return errors.New("transient")
	})
	require.Error(t, err)
	require.Equal(t, 1, attempts, "retries must stop once the context is canceled")
}

func parentNames(parents []gwv1.RouteParentStatus) []gwv1.ObjectName {
	names := make([]gwv1.ObjectName, 0, len(parents))
	for _, p := range parents {
		names = append(names, p.ParentRef.Name)
	}
	return names
}

// The writer suppresses no-op writes with a plain equality check, and the Gateway API defines
// no order for status.parents. Peers do move their own entries (to the front, to the end) when
// they write, so a merge that renormalized the order would spend a write undoing every such
// move, and could provoke the peer into moving its entry back. A reorder we did not make must
// therefore never, on its own, produce a different merged list.
func TestMergePreservesLiveOrder(t *testing.T) {
	desired := []gwv1.RouteParentStatus{
		routeParent(ourController, "m-our-gw"),
		routeParent(ourController, "c-our-gw"),
	}
	for name, live := range map[string][]gwv1.RouteParentStatus{
		"sorted": {
			routeParent(otherController, "a-their-gw"),
			routeParent(ourController, "c-our-gw"),
			routeParent(ourController, "m-our-gw"),
			routeParent(otherController, "z-their-gw"),
		},
		"peer moved its entry to the front": {
			routeParent(otherController, "z-their-gw"),
			routeParent(otherController, "a-their-gw"),
			routeParent(ourController, "c-our-gw"),
			routeParent(ourController, "m-our-gw"),
		},
		"peer moved its entry to the end": {
			routeParent(ourController, "c-our-gw"),
			routeParent(ourController, "m-our-gw"),
			routeParent(otherController, "z-their-gw"),
			routeParent(otherController, "a-their-gw"),
		},
		"ours interleaved and out of order": {
			routeParent(ourController, "m-our-gw"),
			routeParent(otherController, "z-their-gw"),
			routeParent(ourController, "c-our-gw"),
			routeParent(otherController, "a-their-gw"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, live, MergeRouteParentStatuses(ourController, live, desired),
				"an order-only difference must not produce a write")
		})
	}
}

// The capture that motivated keeping the live order: several controllers share a route, each
// placing its own entry its own way on every write. Whatever the others do, once we have
// written, a pass over the list they leave behind must be a no-op for us.
func TestMergeConvergesWithPeersThatMoveTheirEntries(t *testing.T) {
	const frontMover, endMover = "front.example/controller", "end.example/controller"
	moveOwn := func(controller string, toFront bool, live []gwv1.RouteParentStatus) []gwv1.RouteParentStatus {
		var own, rest []gwv1.RouteParentStatus
		for _, p := range live {
			if string(p.ControllerName) == controller {
				own = append(own, p)
			} else {
				rest = append(rest, p)
			}
		}
		if toFront {
			return append(own, rest...)
		}
		return append(rest, own...)
	}
	desired := []gwv1.RouteParentStatus{routeParent(ourController, "our-gw")}

	live := []gwv1.RouteParentStatus{
		routeParent(frontMover, "front-gw"),
		routeParent(endMover, "end-gw"),
	}
	live = MergeRouteParentStatuses(ourController, live, desired)
	for range 3 {
		live = moveOwn(frontMover, true, live)
		require.Equal(t, live, MergeRouteParentStatuses(ourController, live, desired),
			"a peer moving its entry to the front must not make us write")
		live = moveOwn(endMover, false, live)
		require.Equal(t, live, MergeRouteParentStatuses(ourController, live, desired),
			"a peer moving its entry to the end must not make us write")
	}
}

func TestMergeReplacesOursInPlace(t *testing.T) {
	live := []gwv1.RouteParentStatus{
		routeParent(otherController, "z-their-gw"),
		routeParent(ourController, "m-our-gw"),
		routeParent(ourController, "stale-gw"),
		routeParent(otherController, "a-their-gw"),
	}
	updated := routeParent(ourController, "m-our-gw")
	updated.Conditions[0].Status = metav1.ConditionFalse
	desired := []gwv1.RouteParentStatus{
		routeParent(ourController, "new-b-gw"),
		updated,
		routeParent(ourController, "new-a-gw"),
	}

	merged := MergeRouteParentStatuses(ourController, live, desired)

	require.Equal(t,
		[]gwv1.ObjectName{"z-their-gw", "m-our-gw", "a-their-gw", "new-a-gw", "new-b-gw"},
		parentNames(merged),
		"existing entries keep their place, stale ones are removed, new ones are appended in sorted order")
	require.Equal(t, metav1.ConditionFalse, merged[1].Conditions[0].Status,
		"our existing entry must be replaced with its desired counterpart")
}

func TestMergeIsDeterministicForNewEntries(t *testing.T) {
	desired := []gwv1.RouteParentStatus{
		routeParent(ourController, "z-our-gw"),
		routeParent(ourController, "c-our-gw"),
	}
	reversed := []gwv1.RouteParentStatus{desired[1], desired[0]}

	require.Equal(t,
		MergeRouteParentStatuses(ourController, nil, desired),
		MergeRouteParentStatuses(ourController, nil, reversed),
		"where new entries land must not depend on the order the builder produced them in")
}

func TestMergePolicyAncestorsPreservesLiveOrder(t *testing.T) {
	live := []gwv1.PolicyAncestorStatus{
		ancestor(otherController, "b-their-gw"),
		ancestor(ourController, "c-our-gw"),
		ancestor(otherController, "a-their-gw"),
	}
	desired := []gwv1.PolicyAncestorStatus{ancestor(ourController, "c-our-gw")}

	require.Equal(t, live, MergePolicyAncestorStatuses(ourController, live, desired),
		"an order-only difference must not produce a write")
}

func TestOwnsAnyEntryMatchesTheMergeOwnershipPredicate(t *testing.T) {
	require.False(t, OwnsAnyRouteParent(ourController, nil))
	require.False(t, OwnsAnyRouteParent(ourController, []gwv1.RouteParentStatus{}))
	require.False(t, OwnsAnyRouteParent(ourController, []gwv1.RouteParentStatus{
		{ParentRef: gwv1.ParentReference{Name: "gw"}, ControllerName: otherController},
	}))
	require.True(t, OwnsAnyRouteParent(ourController, []gwv1.RouteParentStatus{
		{ParentRef: gwv1.ParentReference{Name: "theirs"}, ControllerName: otherController},
		{ParentRef: gwv1.ParentReference{Name: "ours"}, ControllerName: ourController},
	}))

	require.False(t, OwnsAnyPolicyAncestor(ourController, nil))
	require.False(t, OwnsAnyPolicyAncestor(ourController, []gwv1.PolicyAncestorStatus{
		{AncestorRef: gwv1.ParentReference{Name: "gw"}, ControllerName: otherController},
	}))
	require.True(t, OwnsAnyPolicyAncestor(ourController, []gwv1.PolicyAncestorStatus{
		{AncestorRef: gwv1.ParentReference{Name: "gw"}, ControllerName: ourController},
	}))

	// The predicate has to agree with the merge, or a writer declines to publish an empty
	// status for entries the merge would in fact have cleared.
	ours := []gwv1.RouteParentStatus{
		{ParentRef: gwv1.ParentReference{Name: "gw"}, ControllerName: ourController},
	}
	require.Empty(t, MergeRouteParentStatuses(string(ourController), ours, nil),
		"the merge clears exactly the entries OwnsAnyRouteParent reports we own")
}
