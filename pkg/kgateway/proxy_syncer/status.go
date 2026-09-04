package proxy_syncer

import (
	"errors"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	backendconfigpolicyplugin "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/backendconfigpolicy"
	backendtlspolicyplugin "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/backendtlspolicy"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	reportssdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

type ObjWithAttachedPolicies interface {
	GetAttachedPolicies() ir.AttachedPolicies
	GetObjectSource() ir.ObjectSource
}

var _ ObjWithAttachedPolicies = ir.BackendObjectIR{}

// GenerateBackendPolicyReport generates a report map for all policies attached to the given backends.
// Exported for testing.
func GenerateBackendPolicyReport(in []*ir.BackendObjectIR) reports.ReportMap {
	merged := reports.NewPolicyReportMap()
	reporter := reports.NewReporter(&merged)

	// iterate all backends and aggregate all policies attached to them
	// we track each attachment point of the policy to be tracked as an
	// ancestor for reporting status
	bcpGK := wellknown.BackendConfigPolicyGVK.GroupKind()
	btpGK := wellknown.BackendTLSPolicyGVK.GroupKind()
	for _, obj := range in {
		conflictingBTP := winningBackendTLSPolicyRef(obj.GetAttachedPolicies())
		targetRef := backendAncestorRef(obj.GetObjectSource())

		// BackendTLSPolicy speaks the Gateway API's own condition vocabulary
		// (Accepted/ResolvedRefs/Conflicted) rather than kgateway's Valid/Attached, and
		// resolves conflicts the way translation does. The target ancestor reported here is
		// additive to the Gateway ancestors the translator reports for every route that uses
		// the target: it is the only status an unrouted target, or a Backend used solely by a
		// GatewayExtension, ever gets.
		reportBackendTLSPolicies(reporter, targetRef, obj.GetAttachedPolicies().Policies[btpGK])

		for gk, polAtts := range obj.GetAttachedPolicies().Policies {
			if gk == btpGK {
				continue
			}
			for _, polAtt := range polAtts {
				if polAtt.PolicyRef == nil {
					// the policyRef may be nil in the case of virtual plugins (e.g. istio settings)
					// since there's no real policy object, we don't need to generate status for it
					continue
				}

				key := reportssdk.PolicyKey{
					Group:     polAtt.PolicyRef.Group,
					Kind:      polAtt.PolicyRef.Kind,
					Namespace: polAtt.PolicyRef.Namespace,
					Name:      polAtt.PolicyRef.Name,
				}
				ancestorRef := targetRef
				if polAtt.PolicyRef.SectionName != "" {
					ancestorRef.SectionName = new(gwv1.SectionName(polAtt.PolicyRef.SectionName))
				}
				r := reporter.Policy(key, polAtt.Generation).AncestorRef(ancestorRef)
				if len(polAtt.Errors) > 0 {
					r.SetCondition(reportssdk.PolicyCondition{
						Type:    string(shared.PolicyConditionAccepted),
						Status:  metav1.ConditionFalse,
						Reason:  string(shared.PolicyReasonInvalid),
						Message: polAtt.FormatErrors(),
					})
					continue
				}

				r.SetCondition(reportssdk.PolicyCondition{
					Type:    string(shared.PolicyConditionAccepted),
					Status:  metav1.ConditionTrue,
					Reason:  string(shared.PolicyReasonValid),
					Message: reportssdk.PolicyAcceptedMsg,
				})
				r.SetCondition(reportssdk.PolicyCondition{
					Type:    string(shared.PolicyConditionAttached),
					Status:  metav1.ConditionTrue,
					Reason:  string(shared.PolicyReasonAttached),
					Message: reportssdk.PolicyAttachedMsg,
				})

				if gk == bcpGK {
					if cond, ok := backendconfigpolicyplugin.BuildOverrideCondition(polAtt, conflictingBTP); ok {
						r.SetCondition(cond)
					}
				}
			}
		}
	}

	return merged
}

// backendAncestorRef returns the ancestor ref for a policy attached to a backend: the
// target object itself. Callers add a sectionName for port-specific attachments.
func backendAncestorRef(src ir.ObjectSource) gwv1.ParentReference {
	return gwv1.ParentReference{
		Group:     new(gwv1.Group(src.Group)),
		Kind:      new(gwv1.Kind(src.Kind)),
		Namespace: new(gwv1.Namespace(src.Namespace)),
		Name:      gwv1.ObjectName(src.Name),
	}
}

// reportBackendTLSPolicies reports each BackendTLSPolicy in policies against the target it
// attaches to. The effective policy is chosen by the plugin's MergePolicies on the same
// unfiltered slice translation hands it, so the target ancestor never disagrees with the
// Gateway ancestor or with what is actually applied: an older invalid policy still takes
// precedence, and the newer valid one is Conflicted, not Accepted.
func reportBackendTLSPolicies(reporter reportssdk.Reporter, targetRef gwv1.ParentReference, policies []ir.PolicyAtt) {
	if len(policies) == 0 {
		return
	}
	effective := backendtlspolicyplugin.MergePolicies(policies)
	for _, polAtt := range policies {
		if polAtt.PolicyRef == nil {
			continue
		}
		key := reportssdk.PolicyKey{
			Group:     polAtt.PolicyRef.Group,
			Kind:      polAtt.PolicyRef.Kind,
			Namespace: polAtt.PolicyRef.Namespace,
			Name:      polAtt.PolicyRef.Name,
		}
		ancestorRef := targetRef
		if polAtt.PolicyRef.SectionName != "" {
			ancestorRef.SectionName = new(gwv1.SectionName(polAtt.PolicyRef.SectionName))
		}
		r := reporter.Policy(key, polAtt.Generation).AncestorRef(ancestorRef)
		for _, cond := range backendtlspolicyplugin.BuildPolicyConditions(polAtt, &effective) {
			r.SetCondition(cond)
		}
	}
}

// winningBackendTLSPolicyRef returns the ref of the BackendTLSPolicy whose TLS
// config will apply to a backend, or nil if no BTP is attached or none of the policies
// has a valid translation. Uses the same winner-by-creation-time-and-ref ordering used
// inside the BTP plugin MergePolicies.
func winningBackendTLSPolicyRef(attached ir.AttachedPolicies) *ir.AttachedPolicyRef {
	btps := attached.Policies[wellknown.BackendTLSPolicyGVK.GroupKind()]
	if len(btps) == 0 {
		return nil
	}
	valid := make([]ir.PolicyAtt, 0, len(btps))
	for _, p := range btps {
		if len(p.Errors) > 0 {
			continue
		}
		valid = append(valid, p)
	}
	if len(valid) == 0 {
		return nil
	}
	winner := valid[ir.WinnerPolicyIndexByCreationTimeAndRef(valid)]
	return winner.PolicyRef
}

// GenerateBackendStatusReport builds the Accepted condition for every kgateway Backend from
// its IR-construction errors, falling back to the per-client translation errors (deduplicated
// across connected clients) when there are none. IR errors take precedence because
// TranslateBackend returns them and short-circuits before the per-client policy/validation runs.
// Any plugin-contributed conditions in extraConditions (e.g. the EC2 EndpointsDiscovered
// condition) are merged onto the same Backend so all conditions share a single writer.
// Exported for testing.
func GenerateBackendStatusReport(backends []ir.BackendObjectIR, clusters []uccWithCluster, extraConditions []ir.BackendObjectStatus) reports.ReportMap {
	merged := reports.NewBackendReportMap()
	reporter := reports.NewReporter(&merged)

	backendGVK := wellknown.BackendGVK.GroupKind()

	// index plugin-contributed conditions by Backend resource name for merging below.
	extraByBackend := make(map[string][]metav1.Condition, len(extraConditions))
	for _, ec := range extraConditions {
		extraByBackend[ec.Source.ResourceName()] = append(extraByBackend[ec.Source.ResourceName()], ec.Conditions...)
	}

	// aggregate per-client translation errors per Backend generation, deduplicated by message.
	// Keying by generation ensures errors from a stale generation aren't attributed to a newer
	// one while the per-client clusters are still being recomputed.
	type backendGen struct {
		nn  types.NamespacedName
		gen int64
	}
	type errSet struct {
		msgs []string
		seen map[string]struct{}
	}
	translationErrs := make(map[backendGen]*errSet)
	for _, c := range clusters {
		if c.Error == nil || c.BackendSource.GetGroupKind() != backendGVK {
			continue
		}
		k := backendGen{
			nn:  types.NamespacedName{Namespace: c.BackendSource.Namespace, Name: c.BackendSource.Name},
			gen: c.BackendGeneration,
		}
		es, ok := translationErrs[k]
		if !ok {
			es = &errSet{seen: make(map[string]struct{})}
			translationErrs[k] = es
		}
		msg := c.Error.Error()
		if _, dup := es.seen[msg]; !dup {
			es.seen[msg] = struct{}{}
			es.msgs = append(es.msgs, msg)
		}
	}

	for i := range backends {
		backend := backends[i]
		if backend.Obj == nil {
			continue
		}
		errs := make([]error, 0, len(backend.Errors))
		errs = append(errs, backend.Errors...)
		if len(errs) == 0 {
			k := backendGen{
				nn:  types.NamespacedName{Namespace: backend.GetNamespace(), Name: backend.GetName()},
				gen: backend.Obj.GetGeneration(),
			}
			if es := translationErrs[k]; es != nil {
				slices.Sort(es.msgs)
				for _, m := range es.msgs {
					errs = append(errs, errors.New(m))
				}
			}
		}
		cond := pluginutils.BuildCondition("Backend", errs)
		backendReporter := reporter.Backend(backend.Obj)
		backendReporter.SetCondition(reportssdk.BackendCondition{
			Type:    cond.Type,
			Status:  cond.Status,
			Reason:  cond.Reason,
			Message: cond.Message,
		})

		for _, extra := range extraByBackend[backend.GetObjectSource().ResourceName()] {
			// The Accepted condition is owned by the core translation status above;
			// a plugin-contributed condition must never overwrite it (SetCondition is
			// keyed by Type, so it would otherwise silently mask translation errors).
			if extra.Type == string(kgateway.BackendConditionAccepted) {
				logger.Warn("ignoring plugin-contributed condition that collides with the reserved Accepted type",
					"backend", backend.GetObjectSource().ResourceName())
				continue
			}
			backendReporter.SetCondition(reportssdk.BackendCondition{
				Type:    extra.Type,
				Status:  extra.Status,
				Reason:  extra.Reason,
				Message: extra.Message,
			})
		}
	}

	return merged
}
