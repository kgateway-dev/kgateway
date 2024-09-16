package sslutils

import (
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/rotisserie/eris"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/cert"
)

var (
	InvalidTlsSecretError = func(secret *corev1.Secret, err error) error {
		errorString := fmt.Sprintf("%v.%v is not a valid TLS secret", secret.Namespace, secret.Name)
		return eris.Wrapf(err, errorString)
	}

	NoCertificateFoundError = eris.New("no certificate information found")
)

func ValidateTlsSecret(secret *corev1.Secret) error {
	err := validatedCertData(secret)
	if err != nil {
		return err
	}
	return nil
}

func validatedCertData(sslSecret *corev1.Secret) error {
	certChain := sslSecret.Data[corev1.TLSCertKey]
	privateKey := sslSecret.Data[corev1.TLSPrivateKeyKey]
	rootCa := sslSecret.Data[corev1.ServiceAccountRootCAKey]

	err := isValidSslKeyPair(certChain, privateKey, rootCa)
	if err != nil {
		return InvalidTlsSecretError(sslSecret, err)
	}

	return nil
}

func isValidSslKeyPair(certChain, privateKey, rootCa []byte) error {

	if len(certChain) == 0 || len(privateKey) == 0 {
		return NoCertificateFoundError
	}

	_, err := tls.X509KeyPair(certChain, privateKey)
	if err != nil {
		return err
	}
	// validate that the parsed piece is valid
	// this is still faster than a call out to openssl despite this second parsing pass of the cert
	// pem parsing in go is permissive while envoy is not
	// this might not be needed once we have larger envoy validation
	candidateCert, err := cert.ParseCertsPEM(certChain)
	if err != nil {
		// return err rather than sanitize. This is to maintain UX with older versions and to prevent a large refactor
		// for gatewayv2 secret validation code.
		return err
	}
	reencoded, err := cert.EncodeCertificates(candidateCert...)
	if err != nil {
		return err
	}
	trimmedEncoded := strings.TrimSpace(string(reencoded))
	if trimmedEncoded != strings.TrimSpace(string(certChain)) {
		return fmt.Errorf("certificate chain does not match parsed certificate")
	}

	return err
}
