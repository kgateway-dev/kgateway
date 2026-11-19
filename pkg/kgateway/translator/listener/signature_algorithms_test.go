package listener_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/listener"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// An invalid kgateway.dev/signature-algorithms value must fail translation and be
// reported on the listener, not reach Envoy, where it NACKs the whole listener
// while Programmed still reads True (#14453).
func TestInvalidSignatureAlgorithmFailsTranslationAndIsReported(t *testing.T) {
	secret := sharedTLSSecret(t, &ir.SecretValidation{})
	queries := sharedSecretQueries{secret: secret}
	rm := reports.NewReportMap()
	statusReporter := reports.NewReporter(&rm)

	gw := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"}}
	gatewayReporter := statusReporter.Gateway(gw)
	lr := gatewayReporter.ListenerName("https")
	ml := &listener.MergedListeners{Queries: queries}
	ml.AppendListener(ir.Listener{
		Listener: gwv1.Listener{
			Name:     "https",
			Protocol: gwv1.HTTPSProtocolType,
			Port:     443,
			TLS: &gwv1.ListenerTLSConfig{
				Mode:            new(gwv1.TLSModeTerminate),
				CertificateRefs: []gwv1.SecretObjectReference{{Name: "shared-tls"}},
				Options: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
					annotations.SignatureAlgorithms: "rsa_pss_rsae_sha255",
				},
			},
		},
		Parent: gw,
	}, nil, lr)
	require.Len(t, ml.Listeners, 1)

	translated := ml.Listeners[0].TranslateListener(krt.TestingDummyContext{}, context.Background(), queries, statusReporter)
	assert.Empty(t, translated.HttpFilterChain, "no filter chain should carry a config Envoy would NACK")

	programmed := meta.FindStatusCondition(lr.(*reports.ListenerReport).Status.Conditions, string(gwv1.ListenerConditionProgrammed))
	require.NotNil(t, programmed, "listener should report Programmed")
	assert.Equal(t, metav1.ConditionFalse, programmed.Status)
	assert.Equal(t, string(gwv1.ListenerReasonInvalid), programmed.Reason)
	assert.Contains(t, programmed.Message, "invalid signature algorithm: rsa_pss_rsae_sha255")

	// The certificate refs resolved fine, so the failure must not be reported as a
	// reference problem; ResolvedRefs keeps its default True.
	resolvedRefs := meta.FindStatusCondition(lr.(*reports.ListenerReport).Status.Conditions, string(gwv1.ListenerConditionResolvedRefs))
	if resolvedRefs != nil {
		assert.Equal(t, metav1.ConditionTrue, resolvedRefs.Status)
	}
}
