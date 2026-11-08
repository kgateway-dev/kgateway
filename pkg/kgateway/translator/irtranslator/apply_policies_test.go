package irtranslator

import (
	"errors"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

type testPolicyIR struct {
	ir.PolicyIR
	name string
}

var (
	applyTestGK       = schema.GroupKind{Group: "test", Kind: "Policy"}
	applyTestAncestor = gwv1.ParentReference{
		Group: new(gwv1.Group("gateway.networking.k8s.io")),
		Kind:  new(gwv1.Kind("Gateway")),
		Name:  "gw",
	}
)

func applyTestPolicy(name string, errs ...error) ir.PolicyAtt {
	return ir.PolicyAtt{
		GroupKind: applyTestGK,
		PolicyIr:  &testPolicyIR{name: name},
		PolicyRef: &ir.AttachedPolicyRef{Group: applyTestGK.Group, Kind: applyTestGK.Kind, Namespace: "default", Name: name},
		Errors:    errs,
	}
}

func applyTestAttached(pols ...ir.PolicyAtt) *ir.AttachedPolicies {
	return &ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{applyTestGK: pols}}
}

// applyTestAncestorReport returns the status reported for a policy against applyTestAncestor,
// or nil when none was.
func applyTestAncestorReport(rm reports.ReportMap, name string) *reports.AncestorRefReport {
	pr := rm.PolicyReport(reporter.PolicyKey{Group: applyTestGK.Group, Kind: applyTestGK.Kind, Namespace: "default", Name: name})
	if pr == nil || len(pr.Ancestors) == 0 {
		return nil
	}
	return pr.AncestorRef(applyTestAncestor).(*reports.AncestorRefReport)
}

func appliedNames(t *testing.T, apply *[]string) func(*TranslationPass, *ir.PolicyAtt) error {
	t.Helper()
	return func(_ *TranslationPass, pol *ir.PolicyAtt) error {
		*apply = append(*apply, pol.PolicyIr.(*testPolicyIR).name)
		return nil
	}
}

func TestApplyPoliciesSkipsGroupKindsWithoutPass(t *testing.T) {
	var applied []string
	errs := applyPolicies(TranslationPassPlugins{}, applyTestAttached(applyTestPolicy("a")), &policyApplyOptions{}, appliedNames(t, &applied))
	assert.Empty(t, errs)
	assert.Empty(t, applied, "a group kind without a translation pass must not be applied")
}

func TestApplyPoliciesZeroOptions(t *testing.T) {
	passes := TranslationPassPlugins{applyTestGK: &TranslationPass{}}
	var applied []string
	errs := applyPolicies(passes, applyTestAttached(applyTestPolicy("a"), applyTestPolicy("b", errors.New("bad"))),
		&policyApplyOptions{}, appliedNames(t, &applied))
	assert.Empty(t, errs)
	assert.Equal(t, []string{"a", "b"}, applied, "without skipErrored, errored policies are applied too")
}

func TestApplyPoliciesSkipErrored(t *testing.T) {
	passes := TranslationPassPlugins{applyTestGK: &TranslationPass{}}
	applyErr := errors.New("apply failed")
	var applied []string
	errs := applyPolicies(passes, applyTestAttached(applyTestPolicy("a", errors.New("bad")), applyTestPolicy("b")),
		&policyApplyOptions{skipErrored: true},
		func(pass *TranslationPass, pol *ir.PolicyAtt) error {
			applied = append(applied, pol.PolicyIr.(*testPolicyIR).name)
			return applyErr
		})
	assert.Equal(t, []string{"b"}, applied, "the errored policy must not be applied")
	require.Len(t, errs, 2)
	assert.ErrorContains(t, errs[0], "bad", "the skipped policy's errors come first, in apply order")
	assert.ErrorIs(t, errs[1], applyErr)
}

func TestApplyPoliciesReportsStatusAndMergeOrigins(t *testing.T) {
	a, b := applyTestPolicy("a"), applyTestPolicy("b")
	origins := ir.MergeOrigins{}
	origins.SetOne("field", a.PolicyRef, nil)
	passes := TranslationPassPlugins{applyTestGK: &TranslationPass{
		MergePolicies: func(pols []ir.PolicyAtt) ir.PolicyAtt {
			return ir.PolicyAtt{GroupKind: applyTestGK, PolicyIr: &testPolicyIR{name: "merged"}, MergeOrigins: origins}
		},
	}}
	rm := reports.NewReportMap()
	var metadata *envoycorev3.Metadata
	var applied []string
	errs := applyPolicies(passes, applyTestAttached(a, b), &policyApplyOptions{
		reporter:    reports.NewReporter(&rm),
		ancestorRef: applyTestAncestor,
		metadata:    &metadata,
	}, appliedNames(t, &applied))
	assert.Empty(t, errs)
	assert.Equal(t, []string{"merged"}, applied, "a merging pass applies the merged policy once")
	require.NotNil(t, metadata.GetFilterMetadata()["merge."+applyTestGK.String()], "merge origins must be recorded")

	for name, state := range map[string]reporter.PolicyAttachmentState{
		"a": reporter.PolicyAttachmentStateAttached,
		"b": reporter.PolicyAttachmentStateOverridden,
	} {
		report := applyTestAncestorReport(rm, name)
		require.NotNil(t, report, "policy %s must be reported", name)
		accepted := meta.FindStatusCondition(report.Conditions, string(shared.PolicyConditionAccepted))
		require.NotNil(t, accepted)
		assert.Equal(t, metav1.ConditionTrue, accepted.Status)
		assert.Equal(t, state, report.AttachmentState, "attachment state of %s", name)
	}
}

func TestApplyPoliciesAcceptanceOnly(t *testing.T) {
	passes := TranslationPassPlugins{applyTestGK: &TranslationPass{}}
	rm := reports.NewReportMap()
	applyPolicies(passes, applyTestAttached(applyTestPolicy("a")), &policyApplyOptions{
		reporter:       reports.NewReporter(&rm),
		ancestorRef:    applyTestAncestor,
		acceptanceOnly: true,
	}, func(*TranslationPass, *ir.PolicyAtt) error { return nil })
	report := applyTestAncestorReport(rm, "a")
	require.NotNil(t, report)
	assert.NotNil(t, meta.FindStatusCondition(report.Conditions, string(shared.PolicyConditionAccepted)))
	assert.Equal(t, reporter.PolicyAttachmentStatePending, report.AttachmentState, "acceptanceOnly reports no attachment state")
}

func TestApplyPoliciesSelectPolicies(t *testing.T) {
	passes := TranslationPassPlugins{applyTestGK: &TranslationPass{}}
	rm := reports.NewReportMap()
	opts := &policyApplyOptions{reporter: reports.NewReporter(&rm), ancestorRef: applyTestAncestor}

	var applied []string
	opts.selectPolicies = func(schema.GroupKind, *TranslationPass, []ir.PolicyAtt) ([]ir.PolicyAtt, bool, bool) {
		return nil, true, false
	}
	applyPolicies(passes, applyTestAttached(applyTestPolicy("a")), opts, appliedNames(t, &applied))
	assert.Empty(t, applied, "a group kind the selector leaves out is not applied")
	assert.Nil(t, applyTestAncestorReport(rm, "a"), "nor reported")

	opts.selectPolicies = func(_ schema.GroupKind, _ *TranslationPass, pols []ir.PolicyAtt) ([]ir.PolicyAtt, bool, bool) {
		return append(pols, applyTestPolicy("extra")), false, true
	}
	applyPolicies(passes, applyTestAttached(applyTestPolicy("a")), opts, appliedNames(t, &applied))
	assert.Equal(t, []string{"a", "extra"}, applied, "the selected policies replace the attached ones")
	assert.Nil(t, applyTestAncestorReport(rm, "a"), "the selector can turn status reporting off")
}
