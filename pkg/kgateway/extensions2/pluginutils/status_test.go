package pluginutils

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func TestBuildGatewayExtensionConditionAccepted(t *testing.T) {
	cond := BuildGatewayExtensionCondition("GatewayExtension", nil)

	require.Equal(t, string(kgateway.GatewayExtensionConditionAccepted), cond.Type)
	require.Equal(t, metav1.ConditionTrue, cond.Status)
	require.Equal(t, string(kgateway.GatewayExtensionReasonAccepted), cond.Reason)
	require.Equal(t, "GatewayExtension accepted", cond.Message)
}

func TestBuildGatewayExtensionConditionInvalid(t *testing.T) {
	cond := BuildGatewayExtensionCondition("GatewayExtension", []error{errors.New("bad backendRef")})

	require.Equal(t, string(kgateway.GatewayExtensionConditionAccepted), cond.Type)
	require.Equal(t, metav1.ConditionFalse, cond.Status)
	require.Equal(t, string(kgateway.GatewayExtensionReasonInvalid), cond.Reason)
	require.Contains(t, cond.Message, "bad backendRef")
}

func TestBuildGatewayExtensionConditionAggregatesMultipleErrors(t *testing.T) {
	cond := BuildGatewayExtensionCondition("GatewayExtension",
		[]error{errors.New("first"), errors.New("second")})

	require.Equal(t, metav1.ConditionFalse, cond.Status)
	require.Equal(t, string(kgateway.GatewayExtensionReasonInvalid), cond.Reason)
	require.Contains(t, cond.Message, "2 errors")
	require.Contains(t, cond.Message, "first")
	require.Contains(t, cond.Message, "second")
}
