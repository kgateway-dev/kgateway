package setup_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/test/envtestutil"
)

// TestGatewayAcceptedWithInvalidParametersAndNoValidPorts runs the Gateway deployer and the
// status syncer together on a Gateway whose parametersRef is invalid and whose only listener
// is on a reserved port. Both used to write the Accepted condition and rewrote each other's
// value at the deployer's retry rate.
func TestGatewayAcceptedWithInvalidParametersAndNoValidPorts(t *testing.T) {
	st, err := envtestutil.BuildSettings()
	require.NoError(t, err)
	setupEnvTestAndRun(t, st, func(t *testing.T, ctx context.Context, _ *krt.DebugHandler, client istiokube.CLIClient, _ int) {
		const ns = "gwaccepted"
		_, err := client.Kube().CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}, metav1.CreateOptions{})
		require.NoError(t, err)
		require.NoError(t, client.ApplyYAMLContents(ns, `apiVersion: gateway.kgateway.dev/v1alpha1
kind: GatewayParameters
metadata:
  name: gwp
spec:
  kube: {}`, `kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: gw
spec:
  gatewayClassName: kgateway
  infrastructure:
    parametersRef:
      group: gateway.kgateway.dev
      kind: InvalidKindName
      name: gwp
  listeners:
  - protocol: HTTP
    port: 9091
    name: http`))

		gateways := client.GatewayAPI().GatewayV1().Gateways(ns)
		get := func(c *assert.CollectT) *gwv1.Gateway {
			gw, err := gateways.Get(ctx, "gw", metav1.GetOptions{})
			require.NoError(c, err)
			return gw
		}
		accepted := func(gw *gwv1.Gateway) *metav1.Condition {
			return meta.FindStatusCondition(gw.Status.Conditions, string(gwv1.GatewayConditionAccepted))
		}

		// Wait for both writers to have spoken: the deployer's InvalidParameters, and the
		// status syncer's listener status for the reserved port.
		var settled *gwv1.Gateway
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			gw := get(c)
			cond := accepted(gw)
			require.NotNil(c, cond)
			require.Equal(c, metav1.ConditionFalse, cond.Status)
			require.Equal(c, string(gwv1.GatewayReasonInvalidParameters), cond.Reason)
			require.Len(c, gw.Status.Listeners, 1)
			programmed := meta.FindStatusCondition(gw.Status.Listeners[0].Conditions, string(gwv1.ListenerConditionProgrammed))
			require.NotNil(c, programmed)
			require.Equal(c, metav1.ConditionFalse, programmed.Status)
			settled = gw
			t.Logf("settled at resourceVersion %s: %+v", gw.ResourceVersion, gw.Status)
		}, 30*time.Second, 100*time.Millisecond, "the Gateway should settle on InvalidParameters")

		// Nothing may rewrite the status while the parameters stay invalid. The deployer
		// retries with backoff from 500ms up to 10s, so a fight shows up in this window.
		seen := map[string]string{settled.ResourceVersion: reasonOf(accepted(settled))}
		deadline := time.Now().Add(12 * time.Second)
		for time.Now().Before(deadline) {
			gw, err := gateways.Get(ctx, "gw", metav1.GetOptions{})
			require.NoError(t, err)
			if _, ok := seen[gw.ResourceVersion]; !ok {
				t.Logf("Gateway rewritten at resourceVersion %s: %+v", gw.ResourceVersion, gw.Status)
			}
			seen[gw.ResourceVersion] = reasonOf(accepted(gw))
			time.Sleep(50 * time.Millisecond)
		}
		require.Len(t, seen, 1, "the Gateway was rewritten while its parameters were invalid: %v", seen)

		// Fixing the parametersRef clears InvalidParameters even though the listener is still
		// unusable, and the status syncer's ListenersNotValid shows instead.
		_, err = gateways.Patch(ctx, "gw", types.MergePatchType,
			[]byte(`{"spec":{"infrastructure":{"parametersRef":{"kind":"GatewayParameters"}}}}`), metav1.PatchOptions{})
		require.NoError(t, err)
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			gw := get(c)
			cond := accepted(gw)
			require.NotNil(c, cond)
			require.Equal(c, metav1.ConditionTrue, cond.Status)
			require.Equal(c, string(gwv1.GatewayReasonListenersNotValid), cond.Reason)
			require.Equal(c, gw.Generation, cond.ObservedGeneration)
		}, 30*time.Second, 100*time.Millisecond, "fixing the parametersRef should clear InvalidParameters")
	})
}

func reasonOf(cond *metav1.Condition) string {
	if cond == nil {
		return ""
	}
	return string(cond.Status) + "/" + cond.Reason
}
