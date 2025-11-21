package sslutils

import (
	"crypto/tls"
	"errors"
	"fmt"
	"strings"

	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/cert"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

var (
	ErrInvalidTlsSecret = errors.New("invalid TLS secret")

	InvalidTlsSecretError = func(n, ns string, err error) error {
		return fmt.Errorf("%w %s/%s: %v", ErrInvalidTlsSecret, ns, n, err)
	}

	NoCertificateFoundError = errors.New("no certificate information found")

	ErrMissingCACertKey = errors.New("ca.crt key missing")

	ErrInvalidCACertificate = func(n, ns string, err error) error {
		return fmt.Errorf("invalid ca.crt in ConfigMap %s/%s: %v", ns, n, err)
	}

	// tlsProtocolMap maps TLS version strings to Envoy TLS protocol values
	tlsProtocolMap = map[string]envoytlsv3.TlsParameters_TlsProtocol{
		"1.0": envoytlsv3.TlsParameters_TLSv1_0,
		"1.1": envoytlsv3.TlsParameters_TLSv1_1,
		"1.2": envoytlsv3.TlsParameters_TLSv1_2,
		"1.3": envoytlsv3.TlsParameters_TLSv1_3,
	}
)

// ValidateTlsSecret and return a cleaned cert
func ValidateTlsSecret(sslSecret *corev1.Secret) (cleanedCertChain string, err error) {
	return ValidateTlsSecretData(sslSecret.Name, sslSecret.Namespace, sslSecret.Data)
}

func ValidateTlsSecretData(n, ns string, sslSecretData map[string][]byte) (cleanedCertChain string, err error) {
	certChain := string(sslSecretData[corev1.TLSCertKey])
	privateKey := string(sslSecretData[corev1.TLSPrivateKeyKey])
	rootCa := string(sslSecretData[corev1.ServiceAccountRootCAKey])

	cleanedCertChain, err = cleanedSslKeyPair(certChain, privateKey, rootCa)
	if err != nil {
		err = InvalidTlsSecretError(n, ns, err)
	}
	return cleanedCertChain, err
}

func cleanedSslKeyPair(certChain, privateKey, rootCa string) (cleanedChain string, err error) {
	// in the case where we _only_ provide a rootCa, we do not want to validate tls.key+tls.cert
	if (certChain == "") && (privateKey == "") && (rootCa != "") {
		return certChain, nil
	}

	// validate that the cert and key are a valid pair
	_, err = tls.X509KeyPair([]byte(certChain), []byte(privateKey))
	if err != nil {
		return "", err
	}

	// validate that the parsed piece is valid
	// this is still faster than a call out to openssl despite this second parsing pass of the cert
	// pem parsing in go is permissive while envoy is not
	// this might not be needed once we have larger envoy validation
	candidateCert, err := cert.ParseCertsPEM([]byte(certChain))
	if err != nil {
		// return err rather than sanitize. This is to maintain UX with older versions and to keep in line with kgateway pkg.
		return "", err
	}
	cleanedChainBytes, err := cert.EncodeCertificates(candidateCert...)
	cleanedChain = string(cleanedChainBytes)

	return cleanedChain, err
}

// GetCACertFromConfigMap validates and extracts the ca.crt string from a ConfigMap
func GetCACertFromConfigMap(cm *corev1.ConfigMap) (string, error) {
	caCrt, ok := cm.Data["ca.crt"]
	if !ok {
		return "", ErrMissingCACertKey
	}

	// Validate CA certificate by trying to parse it
	candidateCert, err := cert.ParseCertsPEM([]byte(caCrt))
	if err != nil {
		return "", ErrInvalidCACertificate(cm.Name, cm.Namespace, err)
	}

	// Clean and encode the certificate to ensure proper formatting
	cleanedChainBytes, err := cert.EncodeCertificates(candidateCert...)
	if err != nil {
		return "", ErrInvalidCACertificate(cm.Name, cm.Namespace, err)
	}

	cleanedChain := string(cleanedChainBytes)
	return cleanedChain, nil
}

type TLSExtensionOptionFunc = func(in string, out *ir.TLSConfig) error

func ApplyCipherSuites(in string, out *ir.TLSConfig) error {
	cipherSuites := strings.Split(in, ",")
	for i, suite := range cipherSuites {
		cipherSuites[i] = strings.TrimSpace(suite)
	}
	out.CipherSuites = cipherSuites
	return nil
}

func ApplyEcdhCurves(in string, out *ir.TLSConfig) error {
	ecdhCurves := strings.Split(in, ",")
	for i, curve := range ecdhCurves {
		ecdhCurves[i] = strings.TrimSpace(curve)
	}
	out.EcdhCurves = ecdhCurves
	return nil
}

func ApplyAlpnProtocols(in string, out *ir.TLSConfig) error {
	alpnProtocols := strings.Split(in, ",")
	for i, protocol := range alpnProtocols {
		alpnProtocols[i] = strings.TrimSpace(protocol)
	}
	out.AlpnProtocols = alpnProtocols
	return nil
}

func ApplyMinTLSVersion(in string, out *ir.TLSConfig) error {
	protocol, ok := tlsProtocolMap[in]
	if !ok {
		return fmt.Errorf("invalid minimum tls version: %s", in)
	}

	out.MinTLSVersion = &protocol
	return nil
}

func ApplyMaxTLSVersion(in string, out *ir.TLSConfig) error {
	protocol, ok := tlsProtocolMap[in]
	if !ok {
		return fmt.Errorf("invalid maximum tls version: %s", in)
	}

	out.MaxTLSVersion = &protocol
	return nil
}

func ApplyVerifySubjectAltNames(in string, out *ir.TLSConfig) error {
	altNames := strings.Split(in, ",")
	for i, name := range altNames {
		altNames[i] = strings.TrimSpace(name)
	}
	out.VerifySubjectAltNames = altNames
	return nil
}

var TLSExtensionOptionFuncs = map[gwv1.AnnotationKey]TLSExtensionOptionFunc{
	annotations.CipherSuites:          ApplyCipherSuites,
	annotations.MinTLSVersion:         ApplyMinTLSVersion,
	annotations.MaxTLSVersion:         ApplyMaxTLSVersion,
	annotations.VerifySubjectAltNames: ApplyVerifySubjectAltNames,
	annotations.EcdhCurves:            ApplyEcdhCurves,
	annotations.AlpnProtocols:         ApplyAlpnProtocols,
}

// ApplyTLSExtensionOptions applies the TLS options to the TLS bundle IR
// This function will never exit early, even if an error is encountered.
// It will apply all options and return a wrapped error with all errors encountered.
func ApplyTLSExtensionOptions(options map[gwv1.AnnotationKey]gwv1.AnnotationValue, out *ir.TLSConfig) error {
	var errs error
	for key, option := range options {
		if extensionFunc, ok := TLSExtensionOptionFuncs[key]; ok {
			if err := extensionFunc(string(option), out); err != nil {
				errs = errors.Join(errs, err)
			}
		} else {
			errs = errors.Join(errs, fmt.Errorf("unknown tls option: %s", key))
		}
	}

	if err := validateTLSVersions(out); err != nil {
		errs = errors.Join(errs, err)
	}

	return errs
}

func validateTLSVersions(out *ir.TLSConfig) error {
	if out.MinTLSVersion != nil && out.MaxTLSVersion != nil {
		if *out.MaxTLSVersion < *out.MinTLSVersion {
			return fmt.Errorf("maximum tls version %s is less than minimum tls version %s",
				out.MaxTLSVersion.String(),
				out.MinTLSVersion.String(),
			)
		}
	}
	return nil
}
