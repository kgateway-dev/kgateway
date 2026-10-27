package listener

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8scert "k8s.io/client-go/util/cert"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/sslutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// secretsByNameQueries resolves Secret references by name, failing for unknown names.
type secretsByNameQueries struct {
	query.GatewayQueries
	secrets map[gwv1.ObjectName]ir.Secret
}

func (q secretsByNameQueries) GetSecretForRef(_ krt.HandlerContext, _ context.Context, _ schema.GroupKind, _ string, ref gwv1.SecretObjectReference) (*ir.Secret, error) {
	secret, ok := q.secrets[ref.Name]
	if !ok {
		return nil, fmt.Errorf("secret %s not found", ref.Name)
	}
	return &secret, nil
}

func TestTranslateTLSConfigVerifySubjectAltNamesRequiresCA(t *testing.T) {
	certPEM, keyPEM, err := k8scert.GenerateSelfSignedCertKey("example.com", nil, nil)
	require.NoError(t, err)
	queries := secretsByNameQueries{secrets: map[gwv1.ObjectName]ir.Secret{
		"server-cert": {
			ObjectSource: ir.ObjectSource{Kind: "Secret", Namespace: "default", Name: "server-cert"},
			Data:         map[string][]byte{corev1.TLSCertKey: certPEM, corev1.TLSPrivateKeyKey: keyPEM},
		},
		"client-ca": {
			ObjectSource: ir.ObjectSource{Kind: "Secret", Namespace: "default", Name: "client-ca"},
			Data:         map[string][]byte{"ca.crt": certPEM},
		},
	}}
	caRef := func(name gwv1.ObjectName) []gwv1.ObjectReference {
		return []gwv1.ObjectReference{{Group: "", Kind: "Secret", Name: name}}
	}

	tests := []struct {
		name       string
		validation *ir.ClientCertificateValidationIR
		wantErr    error
		// wantCAErr is a CA lookup failure that must be reported alongside wantErr.
		wantCAErr string
	}{
		{
			name:       "SANs with a Gateway-level CA are accepted",
			validation: &ir.ClientCertificateValidationIR{CACertificateRefs: caRef("client-ca"), RequireClientCertificate: true},
		},
		{
			name:    "SANs without any CA are rejected",
			wantErr: sslutils.ErrVerifySubjectAltNamesRequiresCA,
		},
		{
			name:       "SANs with an unresolved Gateway-level CA in AllowInsecureFallback mode are rejected",
			validation: &ir.ClientCertificateValidationIR{CACertificateRefs: caRef("missing-ca"), AllowInsecureFallback: true},
			wantErr:    sslutils.ErrVerifySubjectAltNamesRequiresCA,
			wantCAErr:  "secret missing-ca not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"}}
			tls := &gwv1.ListenerTLSConfig{
				Mode:            new(gwv1.TLSModeTerminate),
				CertificateRefs: []gwv1.SecretObjectReference{{Name: "server-cert"}},
				Options:         map[gwv1.AnnotationKey]gwv1.AnnotationValue{annotations.VerifySubjectAltNames: "alt.example.com"},
			}
			l := ir.Listener{
				Listener: gwv1.Listener{Name: "https", Protocol: gwv1.HTTPSProtocolType, Port: 443, TLS: tls},
				Parent:   gw,
			}

			tlsConfig, err := translateTLSConfig(krt.TestingDummyContext{}, context.Background(), l, tls, queries, tt.validation, "default")
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				if tt.wantCAErr != "" {
					assert.ErrorContains(t, err, tt.wantCAErr, "the CA lookup failure should not be dropped")
				}
				assert.Nil(t, tlsConfig, "the listener should fail closed")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, tlsConfig)
			assert.Equal(t, []string{"alt.example.com"}, tlsConfig.VerifySubjectAltNames)
			require.NotNil(t, tlsConfig.ClientCertificateValidation, "the CA should be in the IR")
			assert.Len(t, tlsConfig.ClientCertificateValidation.CACertificates, 1)
		})
	}
}
