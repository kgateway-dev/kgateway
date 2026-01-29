package sslutils

import (
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/cert"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// Temporary home for constants for conformance testing that are not yet in a released version of Gateway API (https://github.com/kubernetes-sigs/gateway-api/blob/aa1ab6fd282dee4f74eeca803ec48b333297c637/apis/v1/gateway_types.go#L1606-L1614)
const (
	ListenerReasonInvalidCACertificateRef  gwv1.ListenerConditionReason = "InvalidCACertificateRef"
	ListenerReasonInvalidCACertificateKind gwv1.ListenerConditionReason = "InvalidCACertificateKind"
	ListenerReasonNoValidCACertificate     gwv1.ListenerConditionReason = "NoValidCACertificate"
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

	ErrInvalidCACertificateSecret = func(n, ns string, err error) error {
		return fmt.Errorf("invalid ca.crt in Secret %s/%s: %v", ns, n, err)
	}

	ErrVerifySubjectAltNamesRequiresCA = errors.New("verify-subject-alt-names annotation requires a trusted CA to be configured")

	// tlsProtocolMap maps TLS version strings to Envoy TLS protocol values
	tlsProtocolMap = map[string]envoytlsv3.TlsParameters_TlsProtocol{
		"1.0": envoytlsv3.TlsParameters_TLSv1_0,
		"1.1": envoytlsv3.TlsParameters_TLSv1_1,
		"1.2": envoytlsv3.TlsParameters_TLSv1_2,
		"1.3": envoytlsv3.TlsParameters_TLSv1_3,
	}

	ErrInvalidCACertificateRef  = errors.New(string(ListenerReasonInvalidCACertificateRef))
	ErrInvalidCACertificateKind = errors.New(string(ListenerReasonInvalidCACertificateKind))

	ErrInvalidCACertificateRefDetails = func(n, ns string) error {
		return fmt.Errorf("invalid ca.crt in ConfigMap %s/%s: %w", ns, n, ErrInvalidCACertificateRef)
	}

	ErrInvalidCACertificateKindDetails = func(n, ns, kind string) error {
		return fmt.Errorf("invalid ca.crt kind %s in %s/%s: %w", kind, ns, n, ErrInvalidCACertificateKind)
	}
	ErrMissingCaCertificateRefGrant = errors.New("missing CA certificate reference grant")
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
	return getCACertFromBytes([]byte(caCrt), cm.Name, cm.Namespace)
}

// GetCACertFromSecret validates and extracts the ca.crt string from an ir.Secret
func GetCACertFromSecret(secret *ir.Secret) (string, error) {
	caCrtBytes, ok := secret.Data["ca.crt"]
	if !ok {
		return "", ErrMissingCACertKey
	}

	return getCACertFromBytes(caCrtBytes, secret.Name, secret.Namespace)
}

// getCACertFromBytes validates and extracts the ca.crt string from certificate bytes
func getCACertFromBytes(caCrtBytes []byte, name, namespace string) (string, error) {
	if len(caCrtBytes) == 0 {
		return "", ErrMissingCACertKey
	}

	// Validate CA certificate by trying to parse it
	candidateCert, err := cert.ParseCertsPEM(caCrtBytes)
	if err != nil {
		return "", ErrInvalidCACertificate(name, namespace, err)
	}

	// Clean and encode the certificate to ensure proper formatting
	cleanedChainBytes, err := cert.EncodeCertificates(candidateCert...)
	if err != nil {
		return "", ErrInvalidCACertificate(name, namespace, err)
	}

	return string(cleanedChainBytes), nil
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

func ApplyVerifyCertificateHash(in string, out *ir.TLSConfig) error {
	hashes := splitFakeYamlArray(in)

	var errs error
	for _, hash := range hashes {
		if err := validateCertificateHash(hash); err != nil {
			errs = errors.Join(errs, err)
		}
	}

	if errs != nil {
		return errs
	}

	out.VerifyCertificateHash = hashes
	return nil
}

// Regex to match a SHA256 hash that is split into 32 pairs of hex characters by colons
var sha256HashRegexHexPairs = regexp.MustCompile(
	"^[[:xdigit:]]{2}(:[[:xdigit:]]{2}){31}$",
)

// Validate that a certificate hash is a valid SHA256 hash
// - it has 64 hex characters
// - it may be split into pairs by colons
func validateCertificateHash(hash string) error {
	switch {
	case len(hash) == 64: // 64 hex characters is a valid SHA256 hash
		_, err := hex.DecodeString(hash)
		if err != nil {
			return fmt.Errorf("invalid certificate hash: %s", hash)
		}
		return nil
	case sha256HashRegexHexPairs.MatchString(hash): // 32 pairs of hex characters is a valid SHA256 hash
		return nil
	default:
		return fmt.Errorf("invalid certificate hash: %s", hash)
	}
}

// This function is used to support "fake yaml" array syntax in the annotations.
// This function is used to split strings that are comma or "-" separated and whitespace padded.
// This supports input that look like:
// "string1, string2, string3"
// or
// " - string1
//   - string2
//   - string3"
//
// as well as (not recommended) hybrids like:
// "------ string1, string2
// , string3       -string4"
// This function does not use any actual YAML parsing and the strings may not contain - or , characters, regardless of how they are quoted.
// This is used to parse the VerifyCertificateHash annotation value, which is expected to contain hex characters and colons. This function is safe
// for valid values of this data, and is unexported to discourage its use unless the data being parsed is well understood.
func splitFakeYamlArray(in string) []string {
	hashes := []string{}
	for commaSeparatedHash := range strings.SplitSeq(in, ",") {
		for hash := range strings.SplitSeq(commaSeparatedHash, "-") {
			trimmedHash := strings.TrimFunc(hash, func(r rune) bool {
				return unicode.IsSpace(r)
			})
			if trimmedHash != "" {
				hashes = append(hashes, trimmedHash)
			}
		}
	}
	return hashes
}

var TLSExtensionOptionFuncs = map[gwv1.AnnotationKey]TLSExtensionOptionFunc{
	annotations.CipherSuites:          ApplyCipherSuites,
	annotations.MinTLSVersion:         ApplyMinTLSVersion,
	annotations.MaxTLSVersion:         ApplyMaxTLSVersion,
	annotations.VerifySubjectAltNames: ApplyVerifySubjectAltNames,
	annotations.EcdhCurves:            ApplyEcdhCurves,
	annotations.AlpnProtocols:         ApplyAlpnProtocols,
	annotations.VerifyCertificateHash: ApplyVerifyCertificateHash,
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
