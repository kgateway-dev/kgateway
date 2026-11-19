package reports

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

func gatewayTranslationContribution(gw types.NamespacedName, report *GatewayReport) StatusContribution {
	return StatusContribution{
		Target:       StatusKey{GroupKind: wellknown.GatewayGVK.GroupKind(), NamespacedName: gw},
		Source:       StatusSource{Kind: GatewayStatusSource, Name: gw.String()},
		StatusReport: StatusReport{Gateway: report},
	}
}

func TestXdsRejectionOverlaysGatewayStatus(t *testing.T) {
	nn := types.NamespacedName{Namespace: "default", Name: "gw"}
	gw := gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace, Generation: 3},
		Spec:       gwv1.GatewaySpec{Listeners: []gwv1.Listener{{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType}}},
	}
	translated := &GatewayReport{observedGeneration: 3}

	reduced := ReduceStatusContributions([]StatusContribution{
		// The overlay sorts after the translation report, but the result must not depend on it.
		NewXdsRejectionContribution(nn, "envoy.config.listener.v3.Listener: bad listener"),
		gatewayTranslationContribution(nn, translated),
	})
	require.Empty(t, translated.conditions, "reducing must not mutate the translation's report")

	status := BuildGWStatus(reduced.Gateway, gw, nil)
	require.NotNil(t, status)
	programmed := meta.FindStatusCondition(status.Conditions, string(gwv1.GatewayConditionProgrammed))
	require.NotNil(t, programmed)
	require.Equal(t, metav1.ConditionFalse, programmed.Status)
	require.Equal(t, reporter.GatewayXdsRejectedReason, programmed.Reason)
	require.Equal(t, "envoy.config.listener.v3.Listener: bad listener", programmed.Message)
	require.Equal(t, int64(3), programmed.ObservedGeneration)
	accepted := meta.FindStatusCondition(status.Conditions, string(gwv1.GatewayConditionAccepted))
	require.NotNil(t, accepted)
	require.Equal(t, metav1.ConditionTrue, accepted.Status, "a rejection does not change Accepted")

	// Once the rejection resolves, the overlay contribution goes away and Programmed recovers.
	reduced = ReduceStatusContributions([]StatusContribution{gatewayTranslationContribution(nn, translated)})
	gw.Status = *status
	status = BuildGWStatus(reduced.Gateway, gw, nil)
	programmed = meta.FindStatusCondition(status.Conditions, string(gwv1.GatewayConditionProgrammed))
	require.Equal(t, metav1.ConditionTrue, programmed.Status)
	require.Equal(t, string(gwv1.GatewayReasonProgrammed), programmed.Reason)
}

func TestXdsRejectionKeepsTranslationFalseCondition(t *testing.T) {
	nn := types.NamespacedName{Namespace: "default", Name: "gw"}
	translated := &GatewayReport{}
	translated.SetCondition(reporter.GatewayCondition{
		Type:    gwv1.GatewayConditionProgrammed,
		Status:  metav1.ConditionFalse,
		Reason:  gwv1.GatewayReasonInvalid,
		Message: "translation failed",
	})

	reduced := ReduceStatusContributions([]StatusContribution{
		gatewayTranslationContribution(nn, translated),
		NewXdsRejectionContribution(nn, "rejected"),
	})

	programmed := meta.FindStatusCondition(reduced.Gateway.conditions, string(gwv1.GatewayConditionProgrammed))
	require.Equal(t, string(gwv1.GatewayReasonInvalid), programmed.Reason)
	require.Equal(t, "translation failed", programmed.Message)
}

func TestXdsRejectionWithoutTranslationReportIsDropped(t *testing.T) {
	nn := types.NamespacedName{Namespace: "default", Name: "gw"}
	reduced := ReduceStatusContributions([]StatusContribution{NewXdsRejectionContribution(nn, "rejected")})
	require.Nil(t, reduced.Gateway, "an overlay alone must not make kgateway write status for a Gateway it did not translate")
}
