package backendtlspolicy

import (
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	pluginreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

const resolvedRefsMessage = "Resolved all references"

type InvalidKindError struct {
	Group string
	Kind  string
}

func (e *InvalidKindError) Error() string {
	if e == nil {
		return ""
	}
	if e.Group == "" {
		return fmt.Sprintf("unsupported certificate reference kind %q", e.Kind)
	}
	return fmt.Sprintf("unsupported certificate reference kind %q in group %q", e.Kind, e.Group)
}

type InvalidCACertificateRefError struct {
	Ref   string
	Cause error
}

func (e *InvalidCACertificateRefError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("invalid CA certificate ref %s", e.Ref)
	}
	return fmt.Sprintf("invalid CA certificate ref %s: %v", e.Ref, e.Cause)
}

func MergePolicies(policies []ir.PolicyAtt) ir.PolicyAtt {
	if len(policies) == 0 {
		return ir.PolicyAtt{}
	}

	winnerIdx := winnerIndex(policies)
	return policies[winnerIdx]
}

func BuildPolicyConditions(policy ir.PolicyAtt, effectivePolicy *ir.PolicyAtt) []pluginreporter.PolicyCondition {
	if len(policy.Errors) > 0 {
		return conditionsForErrors(policy)
	}

	conds := []pluginreporter.PolicyCondition{
		{
			Type:               string(gwv1.BackendTLSPolicyConditionResolvedRefs),
			Status:             metav1.ConditionTrue,
			Reason:             string(gwv1.BackendTLSPolicyReasonResolvedRefs),
			Message:            resolvedRefsMessage,
			ObservedGeneration: policy.Generation,
		},
	}

	if effectivePolicy != nil && !samePolicy(policy, *effectivePolicy) {
		conds = append(conds, pluginreporter.PolicyCondition{
			Type:               string(gwv1.PolicyConditionAccepted),
			Status:             metav1.ConditionFalse,
			Reason:             string(gwv1.PolicyReasonConflicted),
			Message:            pluginreporter.PolicyConflictWithHigherPriorityMsg,
			ObservedGeneration: policy.Generation,
		})
		return conds
	}

	conds = append(conds, pluginreporter.PolicyCondition{
		Type:               string(gwv1.PolicyConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(gwv1.PolicyReasonAccepted),
		Message:            pluginreporter.PolicyAcceptedMsg,
		ObservedGeneration: policy.Generation,
	})

	return conds
}

func conditionsForErrors(policy ir.PolicyAtt) []pluginreporter.PolicyCondition {
	reason := string(gwv1.PolicyReasonInvalid)
	var resolvedRefsCondition *pluginreporter.PolicyCondition

	for _, err := range policy.Errors {
		var invalidKindErr *InvalidKindError
		if errors.As(err, &invalidKindErr) {
			resolvedRefsCondition = &pluginreporter.PolicyCondition{
				Type:               string(gwv1.BackendTLSPolicyConditionResolvedRefs),
				Status:             metav1.ConditionFalse,
				Reason:             string(gwv1.BackendTLSPolicyReasonInvalidKind),
				Message:            err.Error(),
				ObservedGeneration: policy.Generation,
			}
			reason = string(gwv1.BackendTLSPolicyReasonNoValidCACertificate)
			break
		}

		var invalidRefErr *InvalidCACertificateRefError
		if errors.As(err, &invalidRefErr) {
			resolvedRefsCondition = &pluginreporter.PolicyCondition{
				Type:               string(gwv1.BackendTLSPolicyConditionResolvedRefs),
				Status:             metav1.ConditionFalse,
				Reason:             string(gwv1.BackendTLSPolicyReasonInvalidCACertificateRef),
				Message:            err.Error(),
				ObservedGeneration: policy.Generation,
			}
			reason = string(gwv1.BackendTLSPolicyReasonNoValidCACertificate)
			break
		}
	}

	conds := make([]pluginreporter.PolicyCondition, 0, 2)
	if resolvedRefsCondition != nil {
		conds = append(conds, *resolvedRefsCondition)
	}
	conds = append(conds, pluginreporter.PolicyCondition{
		Type:               string(gwv1.PolicyConditionAccepted),
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            policy.FormatErrors(),
		ObservedGeneration: policy.Generation,
	})

	return conds
}

func winnerIndex(policies []ir.PolicyAtt) int {
	return ir.WinnerPolicyIndexByCreationTimeAndRef(policies)
}

func samePolicy(a, b ir.PolicyAtt) bool {
	if a.PolicyRef == nil || b.PolicyRef == nil {
		return a.Generation == b.Generation && a.PolicyIr.CreationTime().Equal(b.PolicyIr.CreationTime())
	}
	return a.Generation == b.Generation &&
		a.PolicyRef.Group == b.PolicyRef.Group &&
		a.PolicyRef.Kind == b.PolicyRef.Kind &&
		a.PolicyRef.Namespace == b.PolicyRef.Namespace &&
		a.PolicyRef.Name == b.PolicyRef.Name &&
		a.PolicyRef.SectionName == b.PolicyRef.SectionName
}
