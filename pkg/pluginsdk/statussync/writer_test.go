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
	// Published order is canonical (ParentString), not grouped by owner.
	require.Equal(t, []gwv1.ObjectName{"gw-a", "gw-b", "their-gw"},
		[]gwv1.ObjectName{merged[0].ParentRef.Name, merged[1].ParentRef.Name, merged[2].ParentRef.Name})
	require.Equal(t, gwv1.GatewayController(otherController), merged[2].ControllerName,
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
	require.Equal(t, []gwv1.ObjectName{"gw-a", "gw-b", "their-gw"},
		[]gwv1.ObjectName{merged[0].AncestorRef.Name, merged[1].AncestorRef.Name, merged[2].AncestorRef.Name})
	require.Equal(t, gwv1.GatewayController(otherController), merged[2].ControllerName)
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
	err := RetryStatusWrite(context.Background(), func() error {
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
	err := RetryStatusWrite(ctx, func() error {
		attempts++
		cancel()
		return errors.New("transient")
	})
	require.Error(t, err)
	require.Equal(t, 1, attempts, "retries must stop once the context is canceled")
}

// The writer suppresses no-op writes with a plain equality check, so what we publish has to
// be a function of the entry set alone. If the stored order depended on the order we happened
// to read, a peer controller that rewrites the list in its own sorted order would disagree
// with us on ordering and the two of us would rewrite it back and forth forever.
func TestMergeIsCanonicalRegardlessOfExistingOrder(t *testing.T) {
	forward := []gwv1.RouteParentStatus{
		routeParent(otherController, "b-their-gw"),
		routeParent(otherController, "a-their-gw"),
	}
	reversed := []gwv1.RouteParentStatus{forward[1], forward[0]}
	desired := []gwv1.RouteParentStatus{
		routeParent(ourController, "z-our-gw"),
		routeParent(ourController, "c-our-gw"),
	}

	fromForward := MergeRouteParentStatuses(ourController, forward, desired)
	fromReversed := MergeRouteParentStatuses(ourController, reversed, desired)

	require.Equal(t, fromForward, fromReversed, "merge output must not depend on the order read")
	require.Equal(t,
		[]gwv1.ObjectName{"a-their-gw", "b-their-gw", "c-our-gw", "z-our-gw"},
		[]gwv1.ObjectName{
			fromForward[0].ParentRef.Name, fromForward[1].ParentRef.Name,
			fromForward[2].ParentRef.Name, fromForward[3].ParentRef.Name,
		},
		"published order must be the ParentString order istio and our own builders use")
}

func TestMergePolicyAncestorsIsCanonicalRegardlessOfExistingOrder(t *testing.T) {
	forward := []gwv1.PolicyAncestorStatus{
		ancestor(otherController, "b-their-gw"),
		ancestor(otherController, "a-their-gw"),
	}
	reversed := []gwv1.PolicyAncestorStatus{forward[1], forward[0]}
	desired := []gwv1.PolicyAncestorStatus{ancestor(ourController, "c-our-gw")}

	require.Equal(t,
		MergePolicyAncestorStatuses(ourController, forward, desired),
		MergePolicyAncestorStatuses(ourController, reversed, desired),
		"merge output must not depend on the order read")
}
