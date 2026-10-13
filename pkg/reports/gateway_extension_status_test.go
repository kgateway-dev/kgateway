package reports_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func TestBuildGatewayExtensionStatusAccepted(t *testing.T) {
	report := reports.NewGatewayExtensionReport(5, []metav1.Condition{{
		Type:    string(kgateway.GatewayExtensionConditionAccepted),
		Status:  metav1.ConditionTrue,
		Reason:  string(kgateway.GatewayExtensionReasonAccepted),
		Message: "GatewayExtension accepted",
	}})

	status := reports.BuildGatewayExtensionStatus(report, kgateway.GatewayExtensionStatus{})
	require.NotNil(t, status)

	accepted := requireConditionExists(t, status.Conditions, string(kgateway.GatewayExtensionConditionAccepted))
	require.Equal(t, metav1.ConditionTrue, accepted.Status)
	require.Equal(t, string(kgateway.GatewayExtensionReasonAccepted), accepted.Reason)
	require.Equal(t, int64(5), accepted.ObservedGeneration)
}

func TestBuildGatewayExtensionStatusInvalid(t *testing.T) {
	report := reports.NewGatewayExtensionReport(3, []metav1.Condition{{
		Type:    string(kgateway.GatewayExtensionConditionAccepted),
		Status:  metav1.ConditionFalse,
		Reason:  string(kgateway.GatewayExtensionReasonInvalid),
		Message: `GatewayExtension error: "bad backendRef"`,
	}})

	status := reports.BuildGatewayExtensionStatus(report, kgateway.GatewayExtensionStatus{})
	require.NotNil(t, status)

	accepted := requireConditionExists(t, status.Conditions, string(kgateway.GatewayExtensionConditionAccepted))
	require.Equal(t, metav1.ConditionFalse, accepted.Status)
	require.Equal(t, string(kgateway.GatewayExtensionReasonInvalid), accepted.Reason)
	require.Contains(t, accepted.Message, "bad backendRef")
	require.Equal(t, int64(3), accepted.ObservedGeneration)
}

func TestBuildGatewayExtensionStatusNilReportReturnsNil(t *testing.T) {
	require.Nil(t, reports.BuildGatewayExtensionStatus(nil, kgateway.GatewayExtensionStatus{}))
}

// An unchanged condition must keep its LastTransitionTime, and a condition we do not own
// must be preserved: both are what make the writer's live-vs-desired skip terminate the
// status echo loop instead of rewriting forever.
func TestBuildGatewayExtensionStatusPreservesTransitionTimeAndForeignConditions(t *testing.T) {
	existingTransition := metav1.NewTime(time.Now().Add(-time.Minute).Truncate(time.Second))
	currentStatus := kgateway.GatewayExtensionStatus{
		Conditions: []metav1.Condition{
			{
				Type:               string(kgateway.GatewayExtensionConditionAccepted),
				Status:             metav1.ConditionTrue,
				Reason:             string(kgateway.GatewayExtensionReasonAccepted),
				Message:            "GatewayExtension accepted",
				LastTransitionTime: existingTransition,
			},
			{
				Type:    "ForeignCondition",
				Status:  metav1.ConditionTrue,
				Reason:  "SomethingElse",
				Message: "owned by another controller",
			},
		},
	}

	report := reports.NewGatewayExtensionReport(9, []metav1.Condition{{
		Type:    string(kgateway.GatewayExtensionConditionAccepted),
		Status:  metav1.ConditionTrue,
		Reason:  string(kgateway.GatewayExtensionReasonAccepted),
		Message: "GatewayExtension accepted",
	}})

	status := reports.BuildGatewayExtensionStatus(report, currentStatus)
	require.NotNil(t, status)

	accepted := requireConditionExists(t, status.Conditions, string(kgateway.GatewayExtensionConditionAccepted))
	require.Equal(t, existingTransition, accepted.LastTransitionTime)
	requireConditionExists(t, status.Conditions, "ForeignCondition")

	// Rebuilding from what was just written must reproduce it exactly: the fixed point the
	// writer's skip relies on.
	rebuilt := reports.BuildGatewayExtensionStatus(report, *status)
	require.Equal(t, status.Conditions, rebuilt.Conditions)
}
