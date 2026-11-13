package irtranslator

import (
	"errors"

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
	// skipErrored leaves policies that carry errors unapplied and returns their wrapped
	// errors instead. Scopes with a fail-closed replacement (route configuration, virtual
	// host, route) set it; the others apply what the policy could still build.
	skipErrored bool
	// metadata, when set, receives the merge.<group kind> filter metadata of merged policies.
	metadata **envoycorev3.Metadata
	// reportErrored is called with each group kind's attached policies, after their
	// Accepted condition, to report policy errors beyond the policy's own status.
	reportErrored func(policies []ir.PolicyAtt)
}

// applyPolicies drives the translation passes over the policies attached at one point of
// the config tree: each group kind, in apply order, that has a translation pass goes
// through applyPolicyGroup. It returns the errors of every group kind, in order.
func applyPolicies(
	passes TranslationPassPlugins,
	attached *ir.AttachedPolicies,
	opts *policyApplyOptions,
	apply func(pass *TranslationPass, pol *ir.PolicyAtt) error,
) []error {
	var errs []error
	for _, gk := range attached.ApplyOrderedGroupKinds() {
		pass := passes[gk]
		if pass == nil {
			continue
		}
		errs = append(errs, applyPolicyGroup(gk, pass, attached.Policies[gk], opts, apply)...)
	}
	return errs
}

// applyPolicyGroup applies the policies of one group kind: it reports their Accepted
// condition, merges them, calls apply with each effective policy, records the merge origins
// and reports the attachment state, unless apply returned ir.ErrNotAttachable. It returns
// the errors apply returned and, with skipErrored, those of the policies it skipped.
func applyPolicyGroup(
	gk schema.GroupKind,
	pass *TranslationPass,
	pols []ir.PolicyAtt,
	opts *policyApplyOptions,
	apply func(pass *TranslationPass, pol *ir.PolicyAtt) error,
) []error {
	report := opts.reporter != nil
	if report {
		reportPolicyAcceptanceStatus(opts.reporter, opts.ancestorRef, pols...)
		if opts.reportErrored != nil {
			opts.reportErrored(pols)
		}
	}
	var errs []error
	policies, mergeOrigins := mergePolicies(pass, pols)
	attachable := true
	for i := range policies {
		pol := &policies[i]
		if opts.skipErrored && len(pol.Errors) > 0 {
			errs = append(errs, ir.WrapPolicyErrors(pol.PolicyRef, pol.Errors)...)
			continue
		}
		if err := apply(pass, pol); err != nil {
			errs = append(errs, err)
			if errors.Is(err, ir.ErrNotAttachable) {
				attachable = false
			}
		}
	}
	if opts.metadata != nil {
		*opts.metadata = addMergeOriginsToFilterMetadata(gk, mergeOrigins, *opts.metadata)
	}
	// A policy its plugin cannot attach here stays pending.
	if report && attachable {
		reportPolicyAttachmentStatus(opts.reporter, opts.ancestorRef, mergeOrigins, pols...)
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
