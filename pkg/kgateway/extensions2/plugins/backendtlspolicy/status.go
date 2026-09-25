package backendtlspolicy

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// BuildDesiredPolicyStatus builds the controller-owned portion of a BackendTLSPolicy's
// desired status from its typed report fragment, preserving LastTransitionTime for unchanged
// conditions. The status writer preserves other controllers' ancestors and enforces the
// Gateway API ancestor limit when it merges this desired status with the live object.
//
// Target ancestors are a fallback. The per-backend status path reports every policy against
// the target it attaches to, so that a policy no route references still gets a status; see
// reportBackendTLSPolicies. Once route translation reports a Gateway ancestor, that Gateway
// is the ancestor the Gateway API expects and the one existing tooling reads, so the target
// ancestors are dropped rather than listed beside it. A routed policy therefore reports
// exactly the ancestors it reported before target ancestors existed. The StatusSummary
// ancestor is not a target ancestor and is always kept: it carries targetRefs that do not
// resolve, which a Gateway ancestor earned by another targetRef says nothing about.
func BuildDesiredPolicyStatus(report *reports.PolicyReport, pol *gwv1.BackendTLSPolicy, controller string) *gwv1.PolicyStatus {
	currentStatus := pol.Status
	if report == nil {
		return nil
	}

	status := gwv1.PolicyStatus{
		Ancestors: make([]gwv1.PolicyAncestorStatus, 0, len(report.Ancestors)),
	}

	// Suppression is per policy, not per target: a Gateway ancestor records only the parent
	// it came from, never which targetRef earned it, so there is no way here to tell a
	// routed target apart from an unrouted one on a policy that has both. A policy mixing
	// routed and unrouted targets reports only its Gateway ancestors.
	hasGatewayAncestor := false
	for parentKey := range report.Ancestors {
		if isGatewayAncestor(parentKey) {
			hasGatewayAncestor = true
			break
		}
	}

	for parentKey, ancestorReport := range report.Ancestors {
		if hasGatewayAncestor && !isGatewayAncestor(parentKey) && !isStatusSummaryAncestor(parentKey) {
			continue
		}

		ancestorRef := gwv1.ParentReference{
			Group:     new(gwv1.Group(parentKey.Group)),
			Kind:      new(gwv1.Kind(parentKey.Kind)),
			Name:      gwv1.ObjectName(parentKey.Name),
			Namespace: nil,
		}
		if parentKey.Namespace != "" {
			ancestorRef.Namespace = new(gwv1.Namespace(parentKey.Namespace))
		}
		if parentKey.SectionName != "" {
			ancestorRef.SectionName = new(gwv1.SectionName(parentKey.SectionName))
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

// isStatusSummaryAncestor reports whether an ancestor is the policy's synthetic StatusSummary
// entry (see pluginsdk/reporter.PolicyStatusSummaryAncestorRef).
func isStatusSummaryAncestor(key reports.ParentRefKey) bool {
	return reporter.IsPolicyStatusSummaryAncestor(key.Group, key.Kind, key.Namespace, key.Name)
}

// isGatewayAncestor reports whether an ancestor names a Gateway-side parent: the Gateway
// itself, or the XListenerSet that contributed the listener a route attached to. Those are
// the ancestors route translation reports. Every other ancestor in a BackendTLSPolicy's
// report is a target ancestor from the per-backend status path.
func isGatewayAncestor(key reports.ParentRefKey) bool {
	switch {
	case key.Group == wellknown.GatewayGroup && key.Kind == wellknown.GatewayKind:
		return true
	case key.Group == wellknown.XListenerSetGroup && key.Kind == wellknown.XListenerSetKind:
		return true
	default:
		return false
	}
}
