package fake

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"
)

func statusTestGateway() *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Labels: map[string]string{"a": "b"}},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
			Listeners: []gwv1.Listener{{
				Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 80,
			}},
		},
	}
}

// The status writers send an ObjectMeta carrying only identity, because the API server takes
// nothing but status from a status write. Without the reactor the fake takes the whole
// object, so spec and labels are erased by the first status update.
func TestStatusSubresourceReactorPreservesEverythingButStatus(t *testing.T) {
	ctx := context.Background()
	c := NewClient(t, statusTestGateway())
	InstallStatusSubresourceReactor(c.GatewayAPI().(*gatewayfake.Clientset))

	_, err := c.GatewayAPI().GatewayV1().Gateways("default").UpdateStatus(ctx, &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
		Status: gwv1.GatewayStatus{Conditions: []metav1.Condition{{
			Type:   string(gwv1.GatewayConditionAccepted),
			Status: metav1.ConditionTrue,
			Reason: string(gwv1.GatewayReasonAccepted),
		}}},
	}, metav1.UpdateOptions{})
	require.NoError(t, err)

	got, err := c.GatewayAPI().GatewayV1().Gateways("default").Get(ctx, "gw", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, statusTestGateway().Spec, got.Spec, "a status write must not touch spec")
	require.Equal(t, map[string]string{"a": "b"}, got.Labels, "a status write must not touch metadata")
	require.Len(t, got.Status.Conditions, 1, "the status from the request must be stored")
}

// Non-status updates must keep going through the tracker untouched.
func TestStatusSubresourceReactorLeavesNormalUpdatesAlone(t *testing.T) {
	ctx := context.Background()
	c := NewClient(t, statusTestGateway())
	InstallStatusSubresourceReactor(c.GatewayAPI().(*gatewayfake.Clientset))

	updated := statusTestGateway()
	updated.Spec.Listeners[0].Port = 8080
	_, err := c.GatewayAPI().GatewayV1().Gateways("default").Update(ctx, updated, metav1.UpdateOptions{})
	require.NoError(t, err)

	got, err := c.GatewayAPI().GatewayV1().Gateways("default").Get(ctx, "gw", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, gwv1.PortNumber(8080), got.Spec.Listeners[0].Port)
}
