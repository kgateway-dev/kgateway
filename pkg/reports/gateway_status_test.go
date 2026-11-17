package reports_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/conditions"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// TestBuildGWStatusPreservesBootstrapPatchApplied checks that the translator keeps the
// kgateway.dev/BootstrapPatchApplied condition, which the Gateway reconciler writes.
func TestBuildGWStatusPreservesBootstrapPatchApplied(t *testing.T) {
	// A Gateway whose status carries the condition from the reconciler.
	bootstrapCond := metav1.Condition{
		Type:    conditions.KgatewayConditionBootstrapPatchApplied,
		Status:  metav1.ConditionFalse,
		Reason:  conditions.KgatewayReasonBootstrapInvalidOverlay,
		Message: "bootstrap overlay in GatewayParameters default/gwp: contains a template",
	}
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 80}},
		},
		Status: gwv1.GatewayStatus{Conditions: []metav1.Condition{bootstrapCond}},
	}

	// Build the status as the translation loop does.
	rm := reports.NewReportMap()
	reports.NewReporter(&rm).Gateway(gw)
	status := rm.BuildGWStatus(*gw, nil)

	// The condition is kept as the reconciler wrote it.
	require.NotNil(t, status)
	got := meta.FindStatusCondition(status.Conditions, conditions.KgatewayConditionBootstrapPatchApplied)
	require.NotNil(t, got, "the translator must not drop %s", conditions.KgatewayConditionBootstrapPatchApplied)
	require.Equal(t, bootstrapCond.Status, got.Status)
	require.Equal(t, bootstrapCond.Reason, got.Reason)
	require.Equal(t, bootstrapCond.Message, got.Message)
}
