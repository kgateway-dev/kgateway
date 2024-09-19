package sslutils

import (
	"crypto/tls"
	"fmt"

	"github.com/rotisserie/eris"
	corev1 "k8s.io/api/core/v1"
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

	_, err := tls.X509KeyPair([]byte(certChain), []byte(privateKey))
	return err
}
