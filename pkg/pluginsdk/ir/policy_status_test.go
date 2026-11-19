package ir

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func TestPolicyKey(t *testing.T) {
	want := reporter.PolicyKey{Group: "g", Kind: "K", Namespace: "ns", Name: "n"}
	ref := &AttachedPolicyRef{Group: "g", Kind: "K", Namespace: "ns", Name: "n", SectionName: "ignored"}
	assert.Equal(t, want, ref.PolicyKey(), "the section name is not part of the policy's identity")
	wrapper := PolicyWrapper{ObjectSource: ObjectSource{Group: "g", Kind: "K", Namespace: "ns", Name: "n"}}
	assert.Equal(t, want, wrapper.PolicyKey())
}

func TestAcceptedCondition(t *testing.T) {
	valid := PolicyAtt{Generation: 3}
	assert.Equal(t, reporter.PolicyCondition{
		Type:               string(shared.PolicyConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(shared.PolicyReasonValid),
		Message:            reporter.PolicyAcceptedMsg,
		ObservedGeneration: 3,
	}, valid.AcceptedCondition())

	invalid := PolicyAtt{Generation: 4, Errors: []error{errors.New("bad")}}
	assert.Equal(t, reporter.PolicyCondition{
		Type:               string(shared.PolicyConditionAccepted),
		Status:             metav1.ConditionFalse,
		Reason:             string(shared.PolicyReasonInvalid),
		Message:            invalid.FormatErrors(),
		ObservedGeneration: 4,
	}, invalid.AcceptedCondition())
}

func TestReportPolicyAccepted(t *testing.T) {
	ancestor := gwv1.ParentReference{
		Group:     new(gwv1.Group(gwv1.GroupName)),
		Kind:      new(gwv1.Kind("Gateway")),
		Namespace: new(gwv1.Namespace("ns")),
		Name:      "gw",
	}
	rm := reports.NewPolicyReportMap()
	rp := reports.NewReporter(&rm)

	assert.Nil(t, ReportPolicyAccepted(rp, PolicyAtt{Generation: 1}, ancestor),
		"an attachment that names no policy object has no status to report")
	assert.Empty(t, rm.Policies)

	att := PolicyAtt{Generation: 5, PolicyRef: &AttachedPolicyRef{Group: "g", Kind: "K", Namespace: "ns", Name: "n"}}
	require.NotNil(t, ReportPolicyAccepted(rp, att, ancestor))
	report := rm.Policies[att.PolicyRef.PolicyKey()]
	require.NotNil(t, report)
	key := reports.ParentRefKey{Group: gwv1.GroupName, Kind: "Gateway", NamespacedName: types.NamespacedName{Namespace: "ns", Name: "gw"}}
	require.Contains(t, report.Ancestors, key)
	conds := report.Ancestors[key].Conditions
	require.Len(t, conds, 1)
	assert.Equal(t, string(shared.PolicyConditionAccepted), conds[0].Type)
	assert.Equal(t, metav1.ConditionTrue, conds[0].Status)
	assert.Equal(t, int64(5), conds[0].ObservedGeneration)
}
