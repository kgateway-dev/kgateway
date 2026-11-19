package trafficpolicy

import (
	"errors"
	"testing"
	"time"

	envoytypev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/test"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
)

func TestNewGatewayExtensionStatus(t *testing.T) {
	nn := types.NamespacedName{Namespace: "default", Name: "ext"}

	valid := gatewayExtensionIRForNameTest(nn.String())
	valid.Generation = 3

	translationErr := valid
	translationErr.ExtAuth = nil
	translationErr.Err = errors.New("failed to resolve ExtAuth gRPC backend: no such service")

	// HttpStatus.Code must be a defined, non-zero status code.
	failsValidation := gatewayExtensionIRForNameTest(nn.String())
	failsValidation.Generation = 3
	failsValidation.ExtAuth.StatusOnError = &envoytypev3.HttpStatus{}

	tests := []struct {
		name        string
		ext         TrafficPolicyGatewayExtensionIR
		wantStatus  metav1.ConditionStatus
		wantReason  string
		wantMessage string
	}{
		{
			name:        "translated extension is accepted",
			ext:         valid,
			wantStatus:  metav1.ConditionTrue,
			wantReason:  "Accepted",
			wantMessage: "GatewayExtension accepted",
		},
		{
			name:        "translation error rejects the extension",
			ext:         translationErr,
			wantStatus:  metav1.ConditionFalse,
			wantReason:  "Invalid",
			wantMessage: `GatewayExtension error: "failed to resolve ExtAuth gRPC backend: no such service"`,
		},
		{
			name:       "proto validation failure rejects the extension",
			ext:        failsValidation,
			wantStatus: metav1.ConditionFalse,
			wantReason: "Invalid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newGatewayExtensionStatus(nil, tt.ext)
			require.NotNil(t, got)
			require.Equal(t, nn, got.NamespacedName)
			require.Equal(t, "Accepted", got.Accepted.Type)
			require.Equal(t, tt.wantStatus, got.Accepted.Status)
			require.Equal(t, tt.wantReason, got.Accepted.Reason)
			require.Equal(t, int64(3), got.Accepted.ObservedGeneration, "the condition must report the generation it was translated from")
			if tt.wantMessage != "" {
				require.Equal(t, tt.wantMessage, got.Accepted.Message)
			} else {
				require.NotEmpty(t, got.Accepted.Message, "a rejection must say why")
			}
		})
	}
}

func TestGatewayExtensionIREqualsDetectsGenerationChange(t *testing.T) {
	a := gatewayExtensionIRForNameTest("default/ext")
	b := a
	b.Generation = a.Generation + 1
	require.False(t, a.Equals(b), "a generation-only change must propagate so observedGeneration advances")
}

func TestGatewayExtensionWriter(t *testing.T) {
	stop := test.NewStop(t)
	nn := types.NamespacedName{Namespace: "default", Name: "ext"}
	res := statussync.Resource{GroupVersionKind: wellknown.GatewayExtensionGVK, NamespacedName: nn}
	accepted := newGatewayExtensionStatus(nil, gatewayExtensionIRForNameTest(nn.String()))

	since := metav1.NewTime(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	foreign := metav1.Condition{Type: "Healthy", Status: metav1.ConditionTrue, Reason: "ArgoCD", LastTransitionTime: since}
	live := &kgateway.GatewayExtension{
		ObjectMeta: metav1.ObjectMeta{Namespace: nn.Namespace, Name: nn.Name, Generation: 2},
		Status: kgateway.GatewayExtensionStatus{Conditions: []metav1.Condition{
			foreign,
			{Type: "Accepted", Status: metav1.ConditionTrue, Reason: "Accepted", Message: "stale", ObservedGeneration: 1, LastTransitionTime: since},
		}},
	}
	apply := func(current *kgateway.GatewayExtension, status kgateway.GatewayExtensionStatus) *kgateway.GatewayExtension {
		next := current.DeepCopy()
		next.Status = *status.DeepCopy()
		return next
	}

	t.Run("writes Accepted, preserving its transition time and foreign conditions", func(t *testing.T) {
		row := *accepted
		row.Accepted.ObservedGeneration = 2
		statuses := krt.NewStaticCollection(nil, []gatewayExtensionStatus{row}, krt.WithStop(stop))
		w := gatewayExtensionWriter(nil, nil, statuses)

		desired, ok := w.Desired(res, live)
		require.True(t, ok)
		require.Len(t, desired.Conditions, 2)
		require.Equal(t, foreign, *meta.FindStatusCondition(desired.Conditions, "Healthy"), "conditions kgateway does not own are kept")
		got := meta.FindStatusCondition(desired.Conditions, "Accepted")
		require.Equal(t, "GatewayExtension accepted", got.Message)
		require.Equal(t, int64(2), got.ObservedGeneration)
		require.Equal(t, since, got.LastTransitionTime, "an unchanged condition status keeps its transition time")
		require.Equal(t, "stale", live.Status.Conditions[1].Message, "the live object must not be mutated")

		require.True(t, statussync.WriterWouldWrite(w, res, live), "the stale status must be written, or the check below proves nothing")
		require.NoError(t, statussync.CheckWriterIdempotent(w, res, live, apply))
	})

	t.Run("writes nothing for an extension that has not been translated", func(t *testing.T) {
		statuses := krt.NewStaticCollection[gatewayExtensionStatus](nil, nil, krt.WithStop(stop))
		require.False(t, statussync.WriterWouldWrite(gatewayExtensionWriter(nil, nil, statuses), res, live))
	})
}
