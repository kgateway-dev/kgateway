package sslutils

import (
	"crypto/tls"
	"fmt"

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

// ValidateTlsSecret and return a cleaned cert
func ValidateTlsSecret(secret *corev1.Secret) (cleanedCertChain string, err error) {
	// why does this just do the same thing as validatedCertData?
	// idk
	return validatedCertData(secret)

}

func validatedCertData(sslSecret *corev1.Secret) (cleanedCertChain string, err error) {
	certChain := string(sslSecret.Data[corev1.TLSCertKey])
	privateKey := string(sslSecret.Data[corev1.TLSPrivateKeyKey])
	rootCa := string(sslSecret.Data[corev1.ServiceAccountRootCAKey])

	cleanedCertChain, err = cleanedSslKeyPair(certChain, privateKey, rootCa)
	if err != nil {
		err = InvalidTlsSecretError(sslSecret, err)
	}
	return
}

func isValidSslKeyPair(certChain, privateKey, rootCa []byte) error {
	_, err := cleanedSslKeyPair(string(certChain), string(privateKey), string(rootCa))
	return err
}

func cleanedSslKeyPair(certChain, privateKey, rootCa string) (cleanedChain string, err error) {

	// in the case where we _only_ provide a rootCa, we do not want to validate tls.key+tls.cert
	if (certChain == "") && (privateKey == "") && (rootCa != "") {
		return
	}

	// validate that the cert and key are a valid pair
	_, err = tls.X509KeyPair([]byte(certChain), []byte(privateKey))
	if err != nil {

		return
	}

	// validate that the parsed piece is valid
	// this is still faster than a call out to openssl despite this second parsing pass of the cert
	// pem parsing in go is permissive while envoy is not
	// this might not be needed once we have larger envoy validation
	candidateCert, err := cert.ParseCertsPEM([]byte(certChain))
	if err != nil {
		// return err rather than sanitize. This is to maintain UX with older versions and to keep in line with gateway2 pkg.
		return
	}
	cleanedChainBytes, err := cert.EncodeCertificates(candidateCert...)
	cleanedChain = string(cleanedChainBytes)

	return
}
