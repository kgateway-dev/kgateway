package ir

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/cert"
)

func TestSecretValidateTLS(t *testing.T) {
	certPEM, keyPEM, err := cert.GenerateSelfSignedCertKey("example.com", nil, nil)
	require.NoError(t, err)
	tlsSecret := func(certChain []byte, validation *SecretValidation) Secret {
		return Secret{
			ObjectSource:  ObjectSource{Kind: "Secret", Namespace: "default", Name: "tls"},
			Data:          map[string][]byte{corev1.TLSCertKey: certChain, corev1.TLSPrivateKeyKey: keyPEM},
			TLSValidation: validation,
		}
	}

	t.Run("valid and invalid Secrets", func(t *testing.T) {
		assert.NoError(t, tlsSecret(certPEM, nil).ValidateTLS())
		err := tlsSecret([]byte("not a cert"), nil).ValidateTLS()
		assert.ErrorIs(t, err, ErrInvalidTlsSecret)
		assert.ErrorContains(t, err, "invalid TLS secret default/tls: ")
		assert.NoError(t, Secret{Data: map[string][]byte{corev1.ServiceAccountRootCAKey: certPEM}}.ValidateTLS(), "a CA-only Secret is valid")
	})

	t.Run("copies share the first result", func(t *testing.T) {
		secret := tlsSecret(certPEM, &SecretValidation{})
		copied := secret
		require.NoError(t, secret.ValidateTLS())
		// Data is never changed in place; doing it here shows that later calls do not check again.
		secret.Data[corev1.TLSCertKey] = []byte("not a cert")
		assert.NoError(t, copied.ValidateTLS())
		assert.Zero(t, testing.AllocsPerRun(10, func() { _ = copied.ValidateTLS() }), "later calls should not parse the certificate")
		assert.True(t, secret.Equals(Secret{ObjectSource: secret.ObjectSource}), "Equals should ignore the cached result")

		invalid := tlsSecret([]byte("not a cert"), &SecretValidation{})
		require.ErrorIs(t, invalid.ValidateTLS(), ErrInvalidTlsSecret)
		assert.ErrorIs(t, invalid.ValidateTLS(), ErrInvalidTlsSecret, "the cached result should keep the error")
	})

	t.Run("without TLSValidation, every call checks", func(t *testing.T) {
		secret := tlsSecret(certPEM, nil)
		require.NoError(t, secret.ValidateTLS())
		secret.Data[corev1.TLSCertKey] = []byte("not a cert")
		assert.ErrorIs(t, secret.ValidateTLS(), ErrInvalidTlsSecret)
	})
}
