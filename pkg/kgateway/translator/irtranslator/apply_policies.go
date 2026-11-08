package irtranslator

import (
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

// policyApplyOptions describes how applyPolicies treats one attachment point. The zero
// value applies every attached policy, errored ones included, and reports no status and
// writes no metadata.
type policyApplyOptions struct {
	// reporter receives, against ancestorRef, each policy's Accepted condition and its
	// attachment state. A nil reporter reports nothing.
	reporter    reporter.Reporter
	ancestorRef gwv1.ParentReference
	// acceptanceOnly reports the Accepted condition but no attachment state.
	acceptanceOnly bool
	// skipErrored leaves policies that carry errors unapplied and returns their wrapped
	// errors instead. Scopes with a fail-closed replacement (route configuration, virtual
	// host, route) set it; the others apply what the policy could still build.
	skipErrored bool
	// metadata, when set, receives the merge.<group kind> filter metadata of merged policies.
	metadata **envoycorev3.Metadata
	// reportErrored is called with each group kind's attached policies, after their
	// Accepted condition, to report policy errors beyond the policy's own status.
	reportErrored func(policies []ir.PolicyAtt)
	// selectPolicies, when set, replaces the attached policies of a group kind before they
	// are merged. It returns whether to report their status (overriding the reporter check)
	// and false in ok to leave the group kind out.
	selectPolicies func(gk schema.GroupKind, pass *TranslationPass, policies []ir.PolicyAtt) (selected []ir.PolicyAtt, report, ok bool)
}

// applyPolicies drives the translation passes over the policies attached at one point of
// the config tree. For each group kind, in apply order, that has a translation pass, it
// reports the policies' Accepted condition, merges them, calls apply with each effective
// policy, records the merge origins and reports the attachment state. It returns the errors
// apply returned and, with skipErrored, those of the policies it skipped, in order.
func applyPolicies(
	passes TranslationPassPlugins,
	attached *ir.AttachedPolicies,
	opts *policyApplyOptions,
	apply func(pass *TranslationPass, pol *ir.PolicyAtt) error,
) []error {
	var errs []error
	for _, gk := range attached.ApplyOrderedGroupKinds() {
		pols := attached.Policies[gk]
		pass := passes[gk]
		if pass == nil {
			continue
		}
		report := opts.reporter != nil
		if opts.selectPolicies != nil {
			var ok bool
			pols, report, ok = opts.selectPolicies(gk, pass, pols)
			if !ok {
				continue
			}
		}
		if report {
			reportPolicyAcceptanceStatus(opts.reporter, opts.ancestorRef, pols...)
			if opts.reportErrored != nil {
				opts.reportErrored(pols)
			}
		}
		policies, mergeOrigins := mergePolicies(pass, pols)
		for i := range policies {
			pol := &policies[i]
			if opts.skipErrored && len(pol.Errors) > 0 {
				errs = append(errs, ir.WrapPolicyErrors(pol.PolicyRef, pol.Errors)...)
				continue
			}
			if err := apply(pass, pol); err != nil {
				errs = append(errs, err)
			}
		}
		if opts.metadata != nil {
			*opts.metadata = addMergeOriginsToFilterMetadata(gk, mergeOrigins, *opts.metadata)
		}
		if report && !opts.acceptanceOnly {
			reportPolicyAttachmentStatus(opts.reporter, opts.ancestorRef, mergeOrigins, pols...)
		}
	}
	return errs
}

func mergePolicies(pass *TranslationPass, policies []ir.PolicyAtt) ([]ir.PolicyAtt, ir.MergeOrigins) {
	if pass.MergePolicies != nil {
		mergedPolicy := pass.MergePolicies(policies)
		merged := [1]ir.PolicyAtt{mergedPolicy}
		return merged[:], mergedPolicy.MergeOrigins
	}

	return policies, nil
}
