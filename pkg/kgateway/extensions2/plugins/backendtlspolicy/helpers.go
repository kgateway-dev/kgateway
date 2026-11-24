package backendtlspolicy

import (
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	kgwellknown "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	pluginreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	pluginsdkutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
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
		return "invalid CA certificate ref " + e.Ref
	}
	return fmt.Sprintf("invalid CA certificate ref %s: %v", e.Ref, e.Cause)
}

// supportedTargetKinds are the kinds a BackendTLSPolicy takes effect on: the backend kinds
// kgateway builds clusters for, and the alias kind a ServiceEntry is also addressed by.
var supportedTargetKinds = map[schema.GroupKind]struct{}{
	kgwellknown.ServiceGVK.GroupKind():      {},
	kgwellknown.BackendGVK.GroupKind():      {},
	kgwellknown.ServiceEntryGVK.GroupKind(): {},
	kgwellknown.HostnameGVK.GroupKind():     {},
}

// SplitTargetRefs separates the targetRefs a BackendTLSPolicy can attach to from those of a
// kind it cannot, such as an HTTPRoute, returning one user-facing error per unsupported ref
// in targetRefs order.
//
// Unsupported refs are kept out of the policy's attachment refs rather than recorded as policy
// errors. A ref left in place still attaches: to a route it configures nothing yet reports
// Accepted=True, and with a policy error it replaces that route with a 500. A policy error
// would also blackhole the cluster of every supported target the policy names. The proxy
// syncer reports the unsupported refs on the policy's StatusSummary ancestor instead, the
// same way it reports targetRefs that do not resolve.
func SplitTargetRefs(targetRefs []gwv1.LocalPolicyTargetReferenceWithSectionName) ([]ir.PolicyRef, []error) {
	supported := make([]gwv1.LocalPolicyTargetReferenceWithSectionName, 0, len(targetRefs))
	var unsupported []error
	for _, ref := range targetRefs {
		gk := schema.GroupKind{Group: string(ref.Group), Kind: string(ref.Kind)}
		if _, ok := supportedTargetKinds[gk]; ok {
			supported = append(supported, ref)
			continue
		}
		kind := gk.Kind
		if gk.Group != "" {
			kind = gk.Group + "/" + gk.Kind
		}
		unsupported = append(unsupported, fmt.Errorf(
			"unsupported targetRef kind %s %q: BackendTLSPolicy can only target Service, Backend, ServiceEntry or Hostname",
			kind, ref.Name,
		))
	}
	return pluginsdkutils.TargetRefsToPolicyRefsWithSectionNameV1(supported), unsupported
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
		if _, ok := errors.AsType[*InvalidKindError](err); ok {
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

		if _, ok := errors.AsType[*InvalidCACertificateRefError](err); ok {
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
