package listener_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8scert "k8s.io/client-go/util/cert"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/listener"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// sharedSecretQueries resolves every Secret reference to one Secret.
type sharedSecretQueries struct {
	query.GatewayQueries
	secret ir.Secret
}

func (q sharedSecretQueries) GetSecretForRef(krt.HandlerContext, context.Context, schema.GroupKind, string, gwv1.SecretObjectReference) (*ir.Secret, error) {
	// Return a copy, as a KRT fetch does.
	secret := q.secret
	return &secret, nil
}

func sharedTLSSecret(t testing.TB, validation *ir.SecretValidation) ir.Secret {
	certPEM, keyPEM, err := k8scert.GenerateSelfSignedCertKey("example.com", nil, nil)
	require.NoError(t, err)
	return ir.Secret{
		ObjectSource:  ir.ObjectSource{Kind: "Secret", Namespace: "default", Name: "shared-tls"},
		Data:          map[string][]byte{corev1.TLSCertKey: certPEM, corev1.TLSPrivateKeyKey: keyPEM},
		TLSValidation: validation,
	}
}

// httpsListenersSharingASecret builds n HTTPS listeners on one port, each with its own hostname
// and all referencing the same certificate Secret.
func httpsListenersSharingASecret(n int, queries query.GatewayQueries, statusReporter reporter.Reporter) (*listener.MergedListeners, []reporter.ListenerReporter) {
	gw := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"}}
	gatewayReporter := statusReporter.Gateway(gw)
	ml := &listener.MergedListeners{Queries: queries}
	listenerReporters := make([]reporter.ListenerReporter, 0, n)
	for i := range n {
		name := fmt.Sprintf("https-%d", i)
		lr := gatewayReporter.ListenerName(name)
		listenerReporters = append(listenerReporters, lr)
		ml.AppendListener(ir.Listener{
			Listener: gwv1.Listener{
				Name:     gwv1.SectionName(name),
				Protocol: gwv1.HTTPSProtocolType,
				Port:     443,
				Hostname: new(gwv1.Hostname(fmt.Sprintf("host-%d.example.com", i))),
				TLS: &gwv1.ListenerTLSConfig{
					Mode:            new(gwv1.TLSModeTerminate),
					CertificateRefs: []gwv1.SecretObjectReference{{Name: "shared-tls"}},
				},
			},
			Parent: gw,
		}, nil, lr)
	}
	return ml, listenerReporters
}

func TestHTTPSListenersSharingAnInvalidSecretEachReportIt(t *testing.T) {
	secret := sharedTLSSecret(t, &ir.SecretValidation{})
	secret.Data[corev1.TLSCertKey] = []byte("not a cert")
	queries := sharedSecretQueries{secret: secret}
	rm := reports.NewReportMap()
	statusReporter := reports.NewReporter(&rm)
	ml, listenerReporters := httpsListenersSharingASecret(3, queries, statusReporter)
	require.Len(t, ml.Listeners, 1, "listeners on one port should merge")

	translated := ml.Listeners[0].TranslateListener(krt.TestingDummyContext{}, context.Background(), queries, statusReporter)
	assert.Empty(t, translated.HttpFilterChain, "no filter chain should be built from an invalid certificate")

	for i, lr := range listenerReporters {
		resolvedRefs := meta.FindStatusCondition(lr.(*reports.ListenerReport).Status.Conditions, string(gwv1.ListenerConditionResolvedRefs))
		require.NotNil(t, resolvedRefs, "listener %d should report ResolvedRefs", i)
		assert.Equal(t, metav1.ConditionFalse, resolvedRefs.Status, "listener %d", i)
		assert.Equal(t, string(gwv1.ListenerReasonInvalidCertificateRef), resolvedRefs.Reason, "listener %d", i)
		assert.Contains(t, resolvedRefs.Message, "invalid TLS secret default/shared-tls", "listener %d", i)
	}
}

// BenchmarkTranslateHTTPSListenersSharingASecret translates one port with many HTTPS listeners
// that share a certificate Secret, the shape of many ListenerSets sharing a wildcard certificate.
// Without a cached validation every listener parses the certificate on every translation; with
// one, the Secret is parsed once and later translations reuse the result.
func BenchmarkTranslateHTTPSListenersSharingASecret(b *testing.B) {
	const listeners = 500
	for _, bc := range []struct {
		name       string
		validation *ir.SecretValidation
	}{
		{name: "validate per listener", validation: nil},
		{name: "validate once per Secret", validation: &ir.SecretValidation{}},
	} {
		b.Run(bc.name, func(b *testing.B) {
			queries := sharedSecretQueries{secret: sharedTLSSecret(b, bc.validation)}
			rm := reports.NewReportMap()
			statusReporter := reports.NewReporter(&rm)
			ml, _ := httpsListenersSharingASecret(listeners, queries, statusReporter)
			ctx := context.Background()
			b.ReportAllocs()
			for b.Loop() {
				translated := ml.Listeners[0].TranslateListener(krt.TestingDummyContext{}, ctx, queries, statusReporter)
				if len(translated.HttpFilterChain) != listeners {
					b.Fatalf("got %d filter chains, want %d", len(translated.HttpFilterChain), listeners)
				}
			}
		})
	}
}
