package backendtlspolicy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	pluginreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

type testPolicyIR struct {
	created time.Time
}

func (t *testPolicyIR) CreationTime() time.Time {
	return t.created
}

func (t *testPolicyIR) Equals(other any) bool {
	o, ok := other.(*testPolicyIR)
	if !ok {
		return false
	}
	return t.created.Equal(o.created)
}

func TestMergePolicies(t *testing.T) {
	t.Run("prefers oldest policy", func(t *testing.T) {
		policies := []ir.PolicyAtt{
			newTestPolicyAtt("newer", time.Unix(20, 0)),
			newTestPolicyAtt("older", time.Unix(10, 0)),
		}

		winner := MergePolicies(policies)
		require.Equal(t, "older", winner.PolicyRef.Name)
	})

	t.Run("breaks ties lexically", func(t *testing.T) {
		created := time.Unix(10, 0)
		policies := []ir.PolicyAtt{
			newTestPolicyAtt("b-policy", created),
			newTestPolicyAtt("a-policy", created),
		}

		winner := MergePolicies(policies)
		require.Equal(t, "a-policy", winner.PolicyRef.Name)
	})
}

func TestBuildPolicyConditions(t *testing.T) {
	winner := newTestPolicyAtt("winner", time.Unix(10, 0))

	t.Run("accepted policy gets Accepted and ResolvedRefs true", func(t *testing.T) {
		conds := BuildPolicyConditions(winner, &winner)
		requireReporterCondition(t, conds, string(gwv1.PolicyConditionAccepted), metav1.ConditionTrue, string(gwv1.PolicyReasonAccepted))
		requireReporterCondition(t, conds, string(gwv1.BackendTLSPolicyConditionResolvedRefs), metav1.ConditionTrue, string(gwv1.BackendTLSPolicyReasonResolvedRefs))
	})

	t.Run("losing policy gets Conflicted", func(t *testing.T) {
		loser := newTestPolicyAtt("loser", time.Unix(20, 0))
		conds := BuildPolicyConditions(loser, &winner)
		requireReporterCondition(t, conds, string(gwv1.PolicyConditionAccepted), metav1.ConditionFalse, string(gwv1.PolicyReasonConflicted))
		requireReporterCondition(t, conds, string(gwv1.BackendTLSPolicyConditionResolvedRefs), metav1.ConditionTrue, string(gwv1.BackendTLSPolicyReasonResolvedRefs))
	})

	t.Run("invalid kind maps to InvalidKind and NoValidCACertificate", func(t *testing.T) {
		invalid := newTestPolicyAtt("invalid", time.Unix(10, 0))
		invalid.Errors = []error{&InvalidKindError{Group: "invalid.io", Kind: "InvalidKind"}}

		conds := BuildPolicyConditions(invalid, &invalid)
		requireReporterCondition(t, conds, string(gwv1.BackendTLSPolicyConditionResolvedRefs), metav1.ConditionFalse, string(gwv1.BackendTLSPolicyReasonInvalidKind))
		requireReporterCondition(t, conds, string(gwv1.PolicyConditionAccepted), metav1.ConditionFalse, string(gwv1.BackendTLSPolicyReasonNoValidCACertificate))
	})

	t.Run("invalid CA ref maps to InvalidCACertificateRef and NoValidCACertificate", func(t *testing.T) {
		invalid := newTestPolicyAtt("invalid", time.Unix(10, 0))
		invalid.Errors = []error{&InvalidCACertificateRefError{Ref: "ConfigMap/ca", Cause: ErrConfigMapNotFound}}

		conds := BuildPolicyConditions(invalid, &invalid)
		requireReporterCondition(t, conds, string(gwv1.BackendTLSPolicyConditionResolvedRefs), metav1.ConditionFalse, string(gwv1.BackendTLSPolicyReasonInvalidCACertificateRef))
		requireReporterCondition(t, conds, string(gwv1.PolicyConditionAccepted), metav1.ConditionFalse, string(gwv1.BackendTLSPolicyReasonNoValidCACertificate))
	})
}

func TestBuildPolicyStatusFn(t *testing.T) {
	key := pluginreporter.PolicyKey{
		Group:     gwv1.GroupVersion.Group,
		Kind:      "BackendTLSPolicy",
		Namespace: "default",
		Name:      "tls-policy",
	}
	ancestorRef := gwv1.ParentReference{
		Group:     new(gwv1.Group(gwv1.GroupVersion.Group)),
		Kind:      ptrTo(gwv1.Kind("Gateway")),
		Namespace: ptrTo(gwv1.Namespace("default")),
		Name:      gwv1.ObjectName("gw"),
	}
	rm := reports.NewReportMap()
	ancestorReporter := reports.NewReporter(&rm).Policy(key, 0).AncestorRef(ancestorRef)
	for _, condition := range BuildPolicyConditions(newTestPolicyAtt("tls-policy", time.Unix(10, 0)), nil) {
		ancestorReporter.SetCondition(condition)
	}

	currentStatus := gwv1.PolicyStatus{
		Ancestors: []gwv1.PolicyAncestorStatus{
			{
				AncestorRef:    ancestorRef,
				ControllerName: gwv1.GatewayController("kgateway.dev/kgateway"),
				Conditions: []metav1.Condition{
					{
						Type:               string(gwv1.PolicyConditionAccepted),
						Status:             metav1.ConditionTrue,
						Reason:             string(gwv1.PolicyReasonAccepted),
						ObservedGeneration: 7,
						LastTransitionTime: metav1.NewTime(time.Unix(1, 0)),
					},
					{
						Type:               "Attached",
						Status:             metav1.ConditionTrue,
						Reason:             "Attached",
						ObservedGeneration: 0,
						LastTransitionTime: metav1.NewTime(time.Unix(0, 0)),
					},
				},
			},
			{
				AncestorRef:    ancestorRef,
				ControllerName: gwv1.GatewayController("other.example/controller"),
				Conditions:     []metav1.Condition{{Type: "Other", Status: metav1.ConditionTrue, Reason: "Other"}},
			},
		},
	}

	status := buildPolicyStatusFn()(t.Context(), rm, key, "kgateway.dev/kgateway", currentStatus)
	require.NotNil(t, status)
	require.Len(t, status.Ancestors, 2)

	var ours *gwv1.PolicyAncestorStatus
	for i := range status.Ancestors {
		if status.Ancestors[i].ControllerName == gwv1.GatewayController("kgateway.dev/kgateway") {
			ours = &status.Ancestors[i]
			break
		}
	}
	require.NotNil(t, ours)
	require.Nil(t, findCondition(ours.Conditions, "Attached"))
	requireCondition(t, ours.Conditions, string(gwv1.PolicyConditionAccepted), metav1.ConditionTrue, string(gwv1.PolicyReasonAccepted))
	requireCondition(t, ours.Conditions, string(gwv1.BackendTLSPolicyConditionResolvedRefs), metav1.ConditionTrue, string(gwv1.BackendTLSPolicyReasonResolvedRefs))
}

func TestBuildPolicyStatusFnCapsAncestorsAtAPILimit(t *testing.T) {
	key := pluginreporter.PolicyKey{
		Group:     gwv1.GroupVersion.Group,
		Kind:      "BackendTLSPolicy",
		Namespace: "default",
		Name:      "tls-policy",
	}

	rm := reports.NewReportMap()
	reporter := reports.NewReporter(&rm)
	policyReporter := reporter.Policy(key, 1)
	for i := range reports.MaxPolicyStatusAncestors + 1 {
		ancestorRef := gwv1.ParentReference{
			Group:     ptrTo(gwv1.Group(gwv1.GroupVersion.Group)),
			Kind:      ptrTo(gwv1.Kind("Gateway")),
			Namespace: ptrTo(gwv1.Namespace("default")),
			Name:      gwv1.ObjectName("gw-" + string(rune('a'+i))),
		}

		ancestorReporter := policyReporter.AncestorRef(ancestorRef)
		for _, condition := range BuildPolicyConditions(newTestPolicyAtt("tls-policy", time.Unix(10, 0)), nil) {
			ancestorReporter.SetCondition(condition)
		}
	}

	status := buildPolicyStatusFn()(t.Context(), rm, key, "kgateway.dev/kgateway", gwv1.PolicyStatus{})
	require.NotNil(t, status)
	require.Len(t, status.Ancestors, reports.MaxPolicyStatusAncestors)
	for _, ancestor := range status.Ancestors {
		require.NotEqual(t, gwv1.ObjectName("StatusSummary"), ancestor.AncestorRef.Name)
	}
}

func newTestPolicyAtt(name string, created time.Time) ir.PolicyAtt {
	return ir.PolicyAtt{
		Generation: 1,
		PolicyIr:   &testPolicyIR{created: created},
		PolicyRef: &ir.AttachedPolicyRef{
			Group:     gwv1.GroupVersion.Group,
			Kind:      "BackendTLSPolicy",
			Namespace: "default",
			Name:      name,
		},
	}
}

func requireCondition(t *testing.T, conditions []metav1.Condition, conditionType string, status metav1.ConditionStatus, reason string) {
	t.Helper()
	condition := findCondition(conditions, conditionType)
	require.NotNil(t, condition, "expected condition %s", conditionType)
	require.Equal(t, status, condition.Status)
	require.Equal(t, reason, condition.Reason)
}

func requireReporterCondition(t *testing.T, conditions []pluginreporter.PolicyCondition, conditionType string, status metav1.ConditionStatus, reason string) {
	t.Helper()
	var condition *pluginreporter.PolicyCondition
	for i := range conditions {
		if conditions[i].Type == conditionType {
			condition = &conditions[i]
			break
		}
	}
	require.NotNil(t, condition, "expected condition %s", conditionType)
	require.Equal(t, status, condition.Status)
	require.Equal(t, reason, condition.Reason)
}

func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

//go:fix inline
func ptrTo[T any](v T) *T {
	return new(v)
}
