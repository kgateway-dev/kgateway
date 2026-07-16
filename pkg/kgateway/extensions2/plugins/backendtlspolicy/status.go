package backendtlspolicy

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	pluginreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// BuildDesiredPolicyStatus builds a BackendTLSPolicy's desired status from the merged
// report map, preserving LastTransitionTime for unchanged conditions and ancestors owned
// by other controllers.
func BuildDesiredPolicyStatus(rm reports.ReportMap, pol *gwv1.BackendTLSPolicy, controller string) *gwv1.PolicyStatus {
	key := pluginreporter.PolicyKey{
		Group:     gwv1.GroupName,
		Kind:      "BackendTLSPolicy",
		Namespace: pol.GetNamespace(),
		Name:      pol.GetName(),
	}
	currentStatus := pol.Status
	report := rm.Policies[key]
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

	for _, ancestor := range currentStatus.Ancestors {
		if ancestor.ControllerName != gwv1.GatewayController(controller) {
			status.Ancestors = append(status.Ancestors, ancestor)
		}
	}

	slices.SortStableFunc(status.Ancestors, func(a, b gwv1.PolicyAncestorStatus) int {
		return strings.Compare(reports.ParentString(a.AncestorRef), reports.ParentString(b.AncestorRef))
	})

	if len(status.Ancestors) > reports.MaxPolicyStatusAncestors {
		// Gateway API caps PolicyStatus.ancestors at 16 real entries. We can't
		// invent a synthetic ancestor entry here, so log the truncation explicitly.
		logger.Warn(
			"truncating BackendTLSPolicy status ancestors to Gateway API limit",
			"policy", key.DisplayString(),
			"controller", controller,
			"total_ancestors", len(status.Ancestors),
			"dropped_ancestors", len(status.Ancestors)-reports.MaxPolicyStatusAncestors,
		)
		status.Ancestors = status.Ancestors[:reports.MaxPolicyStatusAncestors]
	}

	return &status
}
