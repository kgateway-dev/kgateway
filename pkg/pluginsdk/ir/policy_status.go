package ir

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

// PolicyKey returns the key the status reports of the policy ref names are filed under.
func (ref *AttachedPolicyRef) PolicyKey() reporter.PolicyKey {
	return reporter.PolicyKey{
		Group:     ref.Group,
		Kind:      ref.Kind,
		Namespace: ref.Namespace,
		Name:      ref.Name,
	}
}

// PolicyKey returns the key the status reports of the wrapped policy are filed under.
func (p PolicyWrapper) PolicyKey() reporter.PolicyKey {
	return reporter.PolicyKey{
		Group:     p.Group,
		Kind:      p.Kind,
		Namespace: p.Namespace,
		Name:      p.Name,
	}
}

// AncestorReporter returns the reporter for att's status on ancestor, recording att's
// generation as the policy's observed one. It returns nil when att names no policy object (an
// attachment by extension ref, a virtual policy, or the result of MergePolicies), which has no
// status to report.
func (att PolicyAtt) AncestorReporter(rp reporter.Reporter, ancestor gwv1.ParentReference) reporter.AncestorRefReporter {
	if att.PolicyRef == nil {
		return nil
	}
	return rp.Policy(att.PolicyRef.PolicyKey(), att.Generation).AncestorRef(ancestor)
}

// AcceptedCondition returns att's Accepted condition: False with reason Invalid and att's
// errors as the message when it has any, otherwise True with reason Valid. Either way it is
// stamped with att's generation.
func (att PolicyAtt) AcceptedCondition() reporter.PolicyCondition {
	if len(att.Errors) > 0 {
		return reporter.PolicyCondition{
			Type:               string(shared.PolicyConditionAccepted),
			Status:             metav1.ConditionFalse,
			Reason:             string(shared.PolicyReasonInvalid),
			Message:            att.FormatErrors(),
			ObservedGeneration: att.Generation,
		}
	}
	return reporter.PolicyCondition{
		Type:               string(shared.PolicyConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(shared.PolicyReasonValid),
		Message:            reporter.PolicyAcceptedMsg,
		ObservedGeneration: att.Generation,
	}
}

// ReportPolicyAccepted reports att's AcceptedCondition on ancestor and returns the ancestor's
// reporter for any further conditions. It returns nil, reporting nothing, when att names no
// policy object.
func ReportPolicyAccepted(rp reporter.Reporter, att PolicyAtt, ancestor gwv1.ParentReference) reporter.AncestorRefReporter {
	r := att.AncestorReporter(rp, ancestor)
	if r != nil {
		r.SetCondition(att.AcceptedCondition())
	}
	return r
}
