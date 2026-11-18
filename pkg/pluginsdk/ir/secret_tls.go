package ir

import (
	"crypto/tls"
	"errors"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/cert"
)

var ErrInvalidTlsSecret = errors.New("invalid TLS secret")

func InvalidTlsSecretError(n, ns string, err error) error {
	return fmt.Errorf("%w %s/%s: %w", ErrInvalidTlsSecret, ns, n, err)
}

// SecretValidation keeps the result of ValidateTLS for one version of a Secret.
type SecretValidation struct {
	once sync.Once
	err  error
}

// ValidateTLS checks that the Secret holds a TLS certificate chain and a matching private key.
// When TLSValidation is set, the check runs on the first call, and later calls on this Secret
// or its copies return that result.
func (c Secret) ValidateTLS() error {
	v := c.TLSValidation
	if v == nil {
		return c.validateTLS()
	}
	v.once.Do(func() { v.err = c.validateTLS() })
	return v.err
}

func (c Secret) validateTLS() error {
	certChain := c.Data[corev1.TLSCertKey]
	privateKey := c.Data[corev1.TLSPrivateKeyKey]

	// in the case where we _only_ provide a rootCa, we do not want to validate tls.key+tls.cert
	if len(certChain) == 0 && len(privateKey) == 0 && len(c.Data[corev1.ServiceAccountRootCAKey]) != 0 {
		return nil
	}

	// validate that the cert and key are a valid pair
	if _, err := tls.X509KeyPair(certChain, privateKey); err != nil {
		return InvalidTlsSecretError(c.Name, c.Namespace, err)
	}

	// validate the chain again on its own: pem parsing in go is permissive while envoy is not
	if _, err := cert.ParseCertsPEM(certChain); err != nil {
		return InvalidTlsSecretError(c.Name, c.Namespace, err)
	}
	return nil
}
