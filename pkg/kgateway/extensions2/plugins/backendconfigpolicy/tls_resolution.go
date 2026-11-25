package backendconfigpolicy

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

// tlsMergeField is the MergeOrigins key mergeTLSConfig records for spec.tls.
const tlsMergeField = "tls"

// TLSSource is the kind of policy that supplies a backend's upstream TLS.
type TLSSource int

const (
	// TLSSourceNone means no attached policy supplies TLS.
	TLSSourceNone TLSSource = iota
	// TLSSourceBackendTLSPolicy means a BackendTLSPolicy owns TLS. Its config is applied, or the
	// cluster is blackholed when the oldest BackendTLSPolicy is invalid.
	TLSSourceBackendTLSPolicy
	// TLSSourceBackendConfigPolicy means a BackendConfigPolicy supplies spec.tls.
	TLSSourceBackendConfigPolicy
)

// TLSResolution says which policy supplies a backend's upstream TLS and which
// BackendConfigPolicies had their spec.tls dropped. Translation and status both
// derive from it, so the applied TLS and the reported Overridden condition agree.
type TLSResolution struct {
	Source TLSSource
	// Winner is the policy that supplies TLS, nil for TLSSourceNone.
	Winner *ir.AttachedPolicyRef
	// WinnerValid is false when Winner has errors. Only a BackendTLSPolicy can win while
	// invalid; translation then blackholes the cluster instead of applying any TLS.
	WinnerValid bool
	// overridden holds the IDs of BackendConfigPolicies that set spec.tls but do not supply it.
	overridden map[string]struct{}
}

// ResolveTLS decides which attached policy supplies a backend's upstream TLS:
//   - Any attached BackendTLSPolicy wins over every BackendConfigPolicy spec.tls. Among
//     BackendTLSPolicies the oldest wins, valid or not, as in the BackendTLSPolicy plugin's
//     MergePolicies; an invalid winner blackholes the cluster rather than falling back.
//   - Otherwise the BackendConfigPolicy that the merge credits with spec.tls wins (higher
//     weight, then older). When any BackendConfigPolicy is invalid, the merge applies nothing
//     and no BackendConfigPolicy is reported as overridden.
func ResolveTLS(attached ir.AttachedPolicies) TLSResolution {
	bcps := attached.Policies[wellknown.BackendConfigPolicyGVK.GroupKind()]

	if btp := winningBackendTLSPolicy(attached); btp != nil {
		res := TLSResolution{Source: TLSSourceBackendTLSPolicy, Winner: btp.PolicyRef, WinnerValid: len(btp.Errors) == 0}
		for _, p := range bcps {
			if setsTLS(p) {
				res.markOverridden(p)
			}
		}
		return res
	}

	var withTLS []ir.PolicyAtt
	for _, p := range bcps {
		if setsTLS(p) {
			withTLS = append(withTLS, p)
		}
	}
	switch len(withTLS) {
	case 0:
		return TLSResolution{}
	case 1:
		// Nothing to merge against; skip the full merge, which runs once per backend.
		if anyErrors(bcps) {
			return TLSResolution{}
		}
		return TLSResolution{Source: TLSSourceBackendConfigPolicy, Winner: withTLS[0].PolicyRef, WinnerValid: true}
	}

	merged := mergePolicies(bcps)
	if len(merged.Errors) > 0 {
		return TLSResolution{}
	}
	owners := merged.MergeOrigins.Get(tlsMergeField)
	if len(owners) != 1 {
		return TLSResolution{}
	}
	res := TLSResolution{Source: TLSSourceBackendConfigPolicy, WinnerValid: true}
	for _, p := range withTLS {
		if p.PolicyRef != nil && p.PolicyRef.ID() == owners[0] {
			res.Winner = p.PolicyRef
			continue
		}
		res.markOverridden(p)
	}
	return res
}

// OverrideCondition returns the Overridden condition for a BackendConfigPolicy attached to
// the resolved backend, or ok=false when its spec.tls is unset or in effect.
func (r TLSResolution) OverrideCondition(policy ir.PolicyAtt) (reporter.PolicyCondition, bool) {
	if policy.PolicyRef == nil || r.Winner == nil {
		return reporter.PolicyCondition{}, false
	}
	if _, ok := r.overridden[policy.PolicyRef.ID()]; !ok {
		return reporter.PolicyCondition{}, false
	}
	reason, kind := ReasonConflictedWithBackendConfigPolicy, wellknown.BackendConfigPolicyGVK.Kind
	if r.Source == TLSSourceBackendTLSPolicy {
		reason, kind = ReasonConflictedWithBackendTLSPolicy, wellknown.BackendTLSPolicyKind
	}
	msg := fmt.Sprintf("spec.tls overridden by %s %s/%s", kind, r.Winner.Namespace, r.Winner.Name)
	if !r.WinnerValid {
		msg += ", which is invalid; requests to this backend fail until it is fixed or removed"
	}
	return reporter.PolicyCondition{
		Type:               ConditionOverridden,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: policy.Generation,
	}, true
}

func (r *TLSResolution) markOverridden(p ir.PolicyAtt) {
	if p.PolicyRef == nil {
		return
	}
	if r.overridden == nil {
		r.overridden = map[string]struct{}{}
	}
	r.overridden[p.PolicyRef.ID()] = struct{}{}
}

// winningBackendTLSPolicy returns the BackendTLSPolicy that owns TLS for a backend, or nil
// when none is attached. It uses the same oldest-wins ordering as the BackendTLSPolicy
// plugin's MergePolicies and, like it, does not skip invalid policies.
func winningBackendTLSPolicy(attached ir.AttachedPolicies) *ir.PolicyAtt {
	btps := attached.Policies[wellknown.BackendTLSPolicyGVK.GroupKind()]
	if len(btps) == 0 {
		return nil
	}
	return &btps[ir.WinnerPolicyIndexByCreationTimeAndRef(btps)]
}

// backendTLSPolicyOwnsTLS reports whether a BackendTLSPolicy, rather than any
// BackendConfigPolicy spec.tls, decides the backend's upstream TLS.
func backendTLSPolicyOwnsTLS(backend ir.BackendObjectIR) bool {
	return winningBackendTLSPolicy(backend.AttachedPolicies) != nil
}

func setsTLS(p ir.PolicyAtt) bool {
	pol, ok := p.PolicyIr.(*BackendConfigPolicyIR)
	return ok && pol.tlsConfig != nil
}

func anyErrors(pols []ir.PolicyAtt) bool {
	for _, p := range pols {
		if len(p.Errors) > 0 {
			return true
		}
	}
	return false
}
