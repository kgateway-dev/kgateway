package backendconfigpolicy

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

// Condition type and reasons for BackendConfigPolicy partial overrides.
const (
	// ConditionOverridden is set on a BackendConfigPolicy ancestor when one or
	// more of its subfields were dropped in favor of a higher priority policy.
	// The condition is only emitted when there is an actual override; absence
	// means no subfield was overridden.
	ConditionOverridden = "Overridden"

	// ReasonConflictedWithBackendTLSPolicy is the Overridden=True reason used
	// when BackendConfigPolicy.spec.tls is dropped because a BackendTLSPolicy
	// also targets the same backend.
	ReasonConflictedWithBackendTLSPolicy = "ConflictedWithBackendTLSPolicy"
)

// HasTLSConfig reports whether policy IR has a TLS config set.
// Exported so the irtranslator can detect the BCP vs BTP TLS conflict without
// reaching into unexported fields.
func HasTLSConfig(polir ir.PolicyIR) bool {
	p, ok := polir.(*BackendConfigPolicyIR)
	if !ok {
		return false
	}
	return p.tlsConfig != nil
}

// BuildOverrideCondition returns the Overridden condition for a
// BackendConfigPolicy ancestor when its TLS field is being dropped in favor
// of conflictingBTP. Returns ok=false when there is no override to report.
func BuildOverrideCondition(policy ir.PolicyAtt, conflictingBTP *ir.AttachedPolicyRef) (reporter.PolicyCondition, bool) {
	if conflictingBTP == nil || !HasTLSConfig(policy.PolicyIr) {
		return reporter.PolicyCondition{}, false
	}
	return reporter.PolicyCondition{
		Type:   ConditionOverridden,
		Status: metav1.ConditionTrue,
		Reason: ReasonConflictedWithBackendTLSPolicy,
		Message: fmt.Sprintf(
			"spec.tls overridden by BackendTLSPolicy %s/%s",
			conflictingBTP.Namespace, conflictingBTP.Name,
		),
		ObservedGeneration: policy.Generation,
	}, true
}
