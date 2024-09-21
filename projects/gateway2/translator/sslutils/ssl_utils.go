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

	// we always return an error when the certChain and/or privateKey are invalid
	// in theory we could propagate only the valid blocks of the certChain (ie the output of cert.ParseCertsPEM(certChain))º
	// and this would be accepted by Envoy, however we choose to maintain consistency between the secret at rest and in
	// Envoy, which also maintains consistency with existing UX
	err := isValidSslKeyPair(certChain, privateKey, rootCa)
	if err != nil {
		return InvalidTlsSecretError(sslSecret, err)
	}

	return nil
}

// isValidSslKeyPair validates that the cert and key are a valid pair
// It previously only checked in go but now also checks that nothing is lost in cert encoding
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
