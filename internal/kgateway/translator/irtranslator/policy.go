package irtranslator

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

func reportPolicyAcceptanceStatus(
	reporter reports.Reporter,
	ancestorRef gwv1.ParentReference,
	policies ...ir.PolicyAtt,
) {
	for _, policy := range policies {
		if policy.PolicyRef == nil {
			// Not a policy associated with a CR, can't report status on it
			return
		}

		key := reports.PolicyKey{
			Group:     policy.PolicyRef.Group,
			Kind:      policy.PolicyRef.Kind,
			Namespace: policy.PolicyRef.Namespace,
			Name:      policy.PolicyRef.Name,
		}
		// Update the initial status
		r := reporter.Policy(key, policy.Generation).AncestorRef(ancestorRef)

		if len(policy.Errors) > 0 {
			r.SetCondition(reports.PolicyCondition{
				Type:    gwv1alpha2.PolicyConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1alpha2.PolicyReasonInvalid,
				Message: policy.FormatErrors(),
			})
			return
		}

		r.SetCondition(reports.PolicyCondition{
			Type:    gwv1alpha2.PolicyConditionAccepted,
			Status:  metav1.ConditionTrue,
			Reason:  gwv1alpha2.PolicyReasonAccepted,
			Message: reports.PolicyAcceptedMsg,
		})
	}
}
