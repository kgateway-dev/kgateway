package backendtlspolicy

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// BuildDesiredPolicyStatus builds the controller-owned portion of a BackendTLSPolicy's
// desired status from its typed report fragment, preserving LastTransitionTime for unchanged
// conditions. The status writer preserves other controllers' ancestors and enforces the
// Gateway API ancestor limit when it merges this desired status with the live object.
func BuildDesiredPolicyStatus(report *reports.PolicyReport, pol *gwv1.BackendTLSPolicy, controller string) *gwv1.PolicyStatus {
	currentStatus := pol.Status
	if report == nil {
		return nil
	}

	status := gwv1.PolicyStatus{
		Ancestors: make([]gwv1.PolicyAncestorStatus, 0, len(report.Ancestors)),
	}

	for parentKey, ancestorReport := range report.Ancestors {
		ancestorRef := gwv1.ParentReference{
			Group:     new(gwv1.Group(parentKey.Group)),
			Kind:      new(gwv1.Kind(parentKey.Kind)),
			Name:      gwv1.ObjectName(parentKey.Name),
			Namespace: nil,
		}
		if parentKey.Namespace != "" {
			ancestorRef.Namespace = new(gwv1.Namespace)
			*ancestorRef.Namespace = gwv1.Namespace(parentKey.Namespace)
		}
		if parentKey.SectionName != "" {
			ancestorRef.SectionName = new(gwv1.SectionName)
			*ancestorRef.SectionName = gwv1.SectionName(parentKey.SectionName)
		}

		var currentParentConditions []metav1.Condition
		currentParentRefIdx := slices.IndexFunc(currentStatus.Ancestors, func(s gwv1.PolicyAncestorStatus) bool {
			return s.ControllerName == gwv1.GatewayController(controller) &&
				reports.ParentRefEqual(s.AncestorRef, ancestorRef)
		})
		if currentParentRefIdx != -1 {
			currentParentConditions = currentStatus.Ancestors[currentParentRefIdx].Conditions
		}

		finalConditions := make([]metav1.Condition, 0, len(ancestorReport.Conditions))
		for _, condition := range ancestorReport.Conditions {
			if existing := meta.FindStatusCondition(currentParentConditions, condition.Type); existing != nil {
				finalConditions = append(finalConditions, *existing)
			}
			meta.SetStatusCondition(&finalConditions, condition)
		}

		status.Ancestors = append(status.Ancestors, gwv1.PolicyAncestorStatus{
			AncestorRef:    ancestorRef,
			ControllerName: gwv1.GatewayController(controller),
			Conditions:     finalConditions,
		})
	}

	// report.Ancestors is a map, so the loop above appends in Go's randomized iteration
	// order. The status writer's merge sorts what it publishes, but direct callers — the
	// translator's golden output — consume this list as-is and would see it reorder run to
	// run. Sort on the same key the merge and the shared policy builder use.
	slices.SortStableFunc(status.Ancestors, func(a, b gwv1.PolicyAncestorStatus) int {
		return strings.Compare(reports.ParentString(a.AncestorRef), reports.ParentString(b.AncestorRef))
	})

	return &status
}
