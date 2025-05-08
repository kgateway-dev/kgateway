package proxy_syncer

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	reportssdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

type ObjWithAttachedPolicies interface {
	GetAttachedPolicies() ir.AttachedPolicies
	GetObjectSource() ir.ObjectSource
}

var _ ObjWithAttachedPolicies = ir.BackendObjectIR{}

func generatePolicyReport[T ObjWithAttachedPolicies](in []T) reports.ReportMap {
	merged := reports.NewReportMap()
	reporter := reports.NewReporter(&merged)

	// iterate all backends and aggregate all policies attached to them
	// we track each attachment point of the policy to be tracked as an
	// ancestor for reporting status
	for _, obj := range in {
		for _, polAtts := range obj.GetAttachedPolicies().Policies {
			for _, polAtt := range polAtts {
				if polAtt.PolicyRef == nil {
					// the policyRef may be nil in the case of virtual plugins (e.g. istio settings)
					// since there's no real policy object, we don't need to generate status for it
					continue
				}

				key := reports.PolicyKey{
					Group:     polAtt.PolicyRef.Group,
					Kind:      polAtt.PolicyRef.Kind,
					Namespace: polAtt.PolicyRef.Namespace,
					Name:      polAtt.PolicyRef.Name,
				}
				ancestorRef := gwv1.ParentReference{
					Group:     ptr.To(gwv1.Group(obj.GetObjectSource().Group)),
					Kind:      ptr.To(gwv1.Kind(obj.GetObjectSource().Kind)),
					Namespace: ptr.To(gwv1.Namespace(obj.GetObjectSource().Namespace)),
					Name:      gwv1.ObjectName(obj.GetObjectSource().Name),
				}
				// Update the initial status
				r := reporter.Policy(key, polAtt.Generation).AncestorRef(ancestorRef)

				if len(polAtt.Errors) > 0 {
					r.SetCondition(reportssdk.PolicyCondition{
						Type:    gwv1alpha2.PolicyConditionAccepted,
						Status:  metav1.ConditionFalse,
						Reason:  gwv1alpha2.PolicyReasonInvalid,
						Message: polAtt.FormatErrors(),
					})
				} else {
					r.SetCondition(reportssdk.PolicyCondition{
						Type:    gwv1alpha2.PolicyConditionAccepted,
						Status:  metav1.ConditionTrue,
						Reason:  gwv1alpha2.PolicyReasonAccepted,
						Message: reportssdk.PolicyAcceptedAndAttachedMsg,
					})
				}
			}
		}
	}

	return merged
}
