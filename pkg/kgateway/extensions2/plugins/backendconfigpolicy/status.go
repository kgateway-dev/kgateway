package backendconfigpolicy

import (
	"context"
	"fmt"

	"istio.io/istio/pkg/kube/kclient"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
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

func getPolicyStatusFn(
	cl kclient.Client[*kgateway.BackendConfigPolicy],
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
	cl kclient.Client[*kgateway.BackendConfigPolicy],
) pluginsdk.PatchPolicyStatusFn {
	return func(ctx context.Context, nn types.NamespacedName, policyStatus gwv1.PolicyStatus) error {
		cur := cl.Get(nn.Name, nn.Namespace)
		if cur == nil {
			return pluginsdk.ErrNotFound
		}
		if _, err := cl.UpdateStatus(&kgateway.BackendConfigPolicy{
			ObjectMeta: pluginsdk.CloneObjectMetaForStatus(cur.ObjectMeta),
			Status:     policyStatus,
		}); err != nil {
			if errors.IsConflict(err) {
				logger.Debug("error updating stale status", "ref", nn, "error", err)
				return nil // let the conflicting Status update trigger a KRT event to requeue the updated object
			}
			return fmt.Errorf("error updating status for BackendConfigPolicy %s: %w", nn.String(), err)
		}
		return nil
	}
}
