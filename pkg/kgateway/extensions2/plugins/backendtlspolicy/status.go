package backendtlspolicy

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"istio.io/istio/pkg/kube/kclient"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	pluginreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func getPolicyStatusFn(
	cl kclient.Client[*gwv1.BackendTLSPolicy],
) pluginsdk.GetPolicyStatusFn {
	return func(ctx context.Context, nn types.NamespacedName) (gwv1.PolicyStatus, error) {
		res := cl.Get(nn.Name, nn.Namespace)
		if res == nil {
			return gwv1.PolicyStatus{}, pluginsdk.ErrNotFound
		}
		return res.Status, nil
	}
}

func patchPolicyStatusFn(
	cl kclient.Client[*gwv1.BackendTLSPolicy],
) pluginsdk.PatchPolicyStatusFn {
	return func(ctx context.Context, nn types.NamespacedName, policyStatus gwv1.PolicyStatus) error {
		cur := cl.Get(nn.Name, nn.Namespace)
		if cur == nil {
			return pluginsdk.ErrNotFound
		}
		if _, err := cl.UpdateStatus(&gwv1.BackendTLSPolicy{
			ObjectMeta: pluginsdk.CloneObjectMetaForStatus(cur.ObjectMeta),
			Status:     policyStatus,
		}); err != nil {
			if errors.IsConflict(err) {
				logger.Debug("error updating stale status", "ref", nn, "error", err)
				return nil // let the conflicting Status update trigger a KRT event to requeue the updated object
			}
			return fmt.Errorf("error updating status for BackendTLSPolicy %s: %w", nn, err)
		}
		return nil
	}
}

func buildPolicyStatusFn() pluginsdk.BuildPolicyStatusFn {
	return func(
		_ context.Context,
		rm reports.ReportMap,
		key pluginreporter.PolicyKey,
		controller string,
		currentStatus gwv1.PolicyStatus,
	) *gwv1.PolicyStatus {
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
}
