package sslutils

import (
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
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

const (
	// maxAlpnProtocolLength is the longest protocol name the ALPN wire format can carry.
	maxAlpnProtocolLength = 255
	// maxAlpnProtocolsLength is the longest comma-joined protocol list Envoy accepts.
	maxAlpnProtocolsLength = 65534
)

var (
	ErrInvalidTlsSecret = ir.ErrInvalidTlsSecret

	InvalidTlsSecretError = ir.InvalidTlsSecretError

	NoCertificateFoundError = errors.New("no certificate information found")

	ErrMissingCACertKey = errors.New("ca.crt key missing")

	ErrInvalidCACertificate = func(n, ns string, err error) error {
		return fmt.Errorf("invalid ca.crt in ConfigMap %s/%s: %w", ns, n, err)
	}

	ErrInvalidCACertificateSecret = func(n, ns string, err error) error {
		return fmt.Errorf("invalid ca.crt in Secret %s/%s: %w", ns, n, err)
	}

	ErrVerifySubjectAltNamesRequiresCA = errors.New("verify-subject-alt-names annotation requires a trusted CA to be configured")

	ErrUnknownTLSExtensionOption = errors.New("unknown tls option")

	// ErrInvalidTLSParameter ends up in status conditions, whose size is limited, so the errors
	// that wrap it quote no more of a rejected name than the longest valid name would need.
	ErrInvalidTLSParameter = errors.New("invalid tls parameter")

	// tlsProtocolMap maps TLS version strings to Envoy TLS protocol values
	tlsProtocolMap = map[string]envoytlsv3.TlsParameters_TlsProtocol{
		"1.0": envoytlsv3.TlsParameters_TLSv1_0,
		"1.1": envoytlsv3.TlsParameters_TLSv1_1,
		"1.2": envoytlsv3.TlsParameters_TLSv1_2,
		"1.3": envoytlsv3.TlsParameters_TLSv1_3,
	}

	// The names below are the ones accepted by the BoringSSL that Envoy is built with. Envoy
	// refuses a TLS context that uses any other name, and with it the whole listener or cluster
	// that the context is part of, so keep them in step with the Envoy version in use.

	// validCipherSuites holds the OpenSSL and the IANA name of every TLS 1.0-1.2 cipher suite.
	validCipherSuites = sets.New(
		"ECDHE-ECDSA-AES128-GCM-SHA256", "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
		"ECDHE-RSA-AES128-GCM-SHA256", "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		"ECDHE-ECDSA-AES256-GCM-SHA384", "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
		"ECDHE-RSA-AES256-GCM-SHA384", "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
		"ECDHE-ECDSA-CHACHA20-POLY1305", "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256",
		"ECDHE-RSA-CHACHA20-POLY1305", "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256",
		"ECDHE-PSK-CHACHA20-POLY1305", "TLS_ECDHE_PSK_WITH_CHACHA20_POLY1305_SHA256",
		"ECDHE-ECDSA-AES128-SHA256", "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256",
		"ECDHE-RSA-AES128-SHA256", "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256",
		"ECDHE-ECDSA-AES128-SHA", "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA",
		"ECDHE-RSA-AES128-SHA", "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA",
		"ECDHE-PSK-AES128-CBC-SHA", "TLS_ECDHE_PSK_WITH_AES_128_CBC_SHA",
		"ECDHE-ECDSA-AES256-SHA", "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA",
		"ECDHE-RSA-AES256-SHA", "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA",
		"ECDHE-PSK-AES256-CBC-SHA", "TLS_ECDHE_PSK_WITH_AES_256_CBC_SHA",
		"AES128-GCM-SHA256", "TLS_RSA_WITH_AES_128_GCM_SHA256",
		"AES256-GCM-SHA384", "TLS_RSA_WITH_AES_256_GCM_SHA384",
		"AES128-SHA", "TLS_RSA_WITH_AES_128_CBC_SHA",
		"AES256-SHA", "TLS_RSA_WITH_AES_256_CBC_SHA",
		"PSK-AES128-CBC-SHA", "TLS_PSK_WITH_AES_128_CBC_SHA",
		"PSK-AES256-CBC-SHA", "TLS_PSK_WITH_AES_256_CBC_SHA",
		"DES-CBC3-SHA", "TLS_RSA_WITH_3DES_EDE_CBC_SHA",
	)

	// tls13CipherSuites are recognized in a cipher list but add nothing to it, and a list that
	// ends up empty is refused.
	tls13CipherSuites = sets.New(
		"TLS_AES_128_GCM_SHA256",
		"TLS_AES_256_GCM_SHA384",
		"TLS_CHACHA20_POLY1305_SHA256",
	)

	// validEcdhCurves maps every spelling of a curve to a single one, since repeating a curve is
	// refused as well.
	validEcdhCurves = map[string]string{
		"P-256": "P-256", "prime256v1": "P-256",
		"P-384": "P-384", "secp384r1": "P-384",
		"P-521": "P-521", "secp521r1": "P-521",
		"X25519": "X25519", "x25519": "X25519",
		"X25519MLKEM768": "X25519MLKEM768",
		"MLKEM1024":      "MLKEM1024",
	}

	// validSignatureAlgorithms maps every spelling of a signature algorithm to a single one, since
	// repeating an algorithm is refused as well.
	validSignatureAlgorithms = map[string]string{
		"rsa_pkcs1_sha1": "rsa_pkcs1_sha1", "RSA+SHA1": "rsa_pkcs1_sha1",
		"rsa_pkcs1_sha256": "rsa_pkcs1_sha256", "RSA+SHA256": "rsa_pkcs1_sha256",
		"rsa_pkcs1_sha384": "rsa_pkcs1_sha384", "RSA+SHA384": "rsa_pkcs1_sha384",
		"rsa_pkcs1_sha512": "rsa_pkcs1_sha512", "RSA+SHA512": "rsa_pkcs1_sha512",
		"ecdsa_sha1": "ecdsa_sha1", "ECDSA+SHA1": "ecdsa_sha1",
		"ecdsa_secp256r1_sha256": "ecdsa_secp256r1_sha256", "ECDSA+SHA256": "ecdsa_secp256r1_sha256",
		"ecdsa_secp384r1_sha384": "ecdsa_secp384r1_sha384", "ECDSA+SHA384": "ecdsa_secp384r1_sha384",
		"ecdsa_secp521r1_sha512": "ecdsa_secp521r1_sha512", "ECDSA+SHA512": "ecdsa_secp521r1_sha512",
		"rsa_pss_rsae_sha256": "rsa_pss_rsae_sha256", "RSA-PSS+SHA256": "rsa_pss_rsae_sha256", "PSS+SHA256": "rsa_pss_rsae_sha256",
		"rsa_pss_rsae_sha384": "rsa_pss_rsae_sha384", "RSA-PSS+SHA384": "rsa_pss_rsae_sha384", "PSS+SHA384": "rsa_pss_rsae_sha384",
		"rsa_pss_rsae_sha512": "rsa_pss_rsae_sha512", "RSA-PSS+SHA512": "rsa_pss_rsae_sha512", "PSS+SHA512": "rsa_pss_rsae_sha512",
		"rsa_pkcs1_sha256_legacy": "rsa_pkcs1_sha256_legacy",
		"ed25519":                 "ed25519",
		"mldsa44":                 "mldsa44",
		"mldsa65":                 "mldsa65",
		"mldsa87":                 "mldsa87",
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

// splitCommaSeparated splits a comma separated option value into its whitespace-trimmed,
// non-empty entries.
func splitCommaSeparated(in string) []string {
	var out []string
	for entry := range strings.SplitSeq(in, ",") {
		if entry = strings.TrimSpace(entry); entry != "" {
			out = append(out, entry)
		}
	}
	return out
}

func ApplyCipherSuites(in string, out *ir.TLSConfig) error {
	cipherSuites := splitCommaSeparated(in)
	if err := ValidateCipherSuites(cipherSuites); err != nil {
		return err
	}
	out.CipherSuites = cipherSuites
	return nil
}

func ApplyEcdhCurves(in string, out *ir.TLSConfig) error {
	ecdhCurves := splitCommaSeparated(in)
	if err := ValidateEcdhCurves(ecdhCurves); err != nil {
		return err
	}
	out.EcdhCurves = ecdhCurves
	return nil
}

func ApplySignatureAlgorithms(in string, out *ir.TLSConfig) error {
	signatureAlgorithms := splitCommaSeparated(in)
	if err := ValidateSignatureAlgorithms(signatureAlgorithms); err != nil {
		return err
	}
	out.SignatureAlgorithms = signatureAlgorithms
	return nil
}

func ApplyAlpnProtocols(in string, out *ir.TLSConfig) error {
	alpnProtocols := splitCommaSeparated(in)
	if err := ValidateAlpnProtocols(alpnProtocols); err != nil {
		return err
	}
	out.AlpnProtocols = alpnProtocols
	return nil
}

// ValidateCipherSuites checks that Envoy will accept cipherSuites as TlsParameters.cipher_suites.
// An entry is a cipher suite name or an equal-preference group of names, "[name1|name2]". The
// operators and aliases of the BoringSSL cipher list syntax are not supported, because whether
// Envoy accepts them depends on what the list as a whole evaluates to.
func ValidateCipherSuites(cipherSuites []string) error {
	configurable := false
	for _, entry := range cipherSuites {
		names := []string{entry}
		if strings.HasPrefix(entry, "[") && strings.HasSuffix(entry, "]") {
			names = strings.Split(entry[1:len(entry)-1], "|")
		}
		for _, name := range names {
			switch {
			case validCipherSuites.Has(name):
				configurable = true
			case tls13CipherSuites.Has(name):
			default:
				return fmt.Errorf("%w: unknown cipher suite: %.64s", ErrInvalidTLSParameter, name)
			}
		}
	}
	if len(cipherSuites) > 0 && !configurable {
		return fmt.Errorf("%w: cipher suites need at least one TLS 1.2 cipher suite, TLS 1.3 cipher suites are not configurable", ErrInvalidTLSParameter)
	}
	return nil
}

// ValidateEcdhCurves checks that Envoy will accept ecdhCurves as TlsParameters.ecdh_curves.
func ValidateEcdhCurves(ecdhCurves []string) error {
	return validateUniqueNames("ecdh curve", ecdhCurves, validEcdhCurves)
}

// ValidateSignatureAlgorithms checks that Envoy will accept signatureAlgorithms as
// TlsParameters.signature_algorithms.
func ValidateSignatureAlgorithms(signatureAlgorithms []string) error {
	return validateUniqueNames("signature algorithm", signatureAlgorithms, validSignatureAlgorithms)
}

// validateUniqueNames checks that every name is a key of valid, and that no two names map to the
// same value, i.e. that none repeats an earlier one under the same or another spelling.
func validateUniqueNames(kind string, names []string, valid map[string]string) error {
	seen := sets.New[string]()
	for _, name := range names {
		canonical, ok := valid[name]
		if !ok {
			return fmt.Errorf("%w: unknown %s: %.64s", ErrInvalidTLSParameter, kind, name)
		}
		if seen.Has(canonical) {
			return fmt.Errorf("%w: duplicate %s: %s", ErrInvalidTLSParameter, kind, name)
		}
		seen.Insert(canonical)
	}
	return nil
}

// ValidateAlpnProtocols checks that Envoy will accept alpnProtocols as
// CommonTlsContext.alpn_protocols. Envoy joins the list with commas and splits it again, so a
// comma in an entry is not preserved and can produce the empty protocol that an upstream TLS
// context asserts on.
func ValidateAlpnProtocols(alpnProtocols []string) error {
	for _, protocol := range alpnProtocols {
		if len(protocol) == 0 || len(protocol) > maxAlpnProtocolLength || strings.Contains(protocol, ",") {
			return fmt.Errorf("%w: an alpn protocol must be 1 to %d bytes long and must not contain a comma", ErrInvalidTLSParameter, maxAlpnProtocolLength)
		}
	}
	if len(strings.Join(alpnProtocols, ",")) > maxAlpnProtocolsLength {
		return fmt.Errorf("%w: alpn protocols must not be longer than %d bytes in total", ErrInvalidTLSParameter, maxAlpnProtocolsLength)
	}
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
	annotations.SignatureAlgorithms:   ApplySignatureAlgorithms,
	annotations.AlpnProtocols:         ApplyAlpnProtocols,
	annotations.VerifyCertificateHash: ApplyVerifyCertificateHash,
}

// kgatewayTLSOptionPrefix is the domain prefix of every kgateway TLS extension option.
const kgatewayTLSOptionPrefix = "kgateway.dev/"

// isKgatewayTLSOption reports whether key is in the kgateway TLS extension option namespace.
// TLS options maps are shared with other implementations, so keys outside it are ignored.
func isKgatewayTLSOption(key gwv1.AnnotationKey) bool {
	return strings.HasPrefix(string(key), kgatewayTLSOptionPrefix)
}

// ApplyTLSExtensionOptions applies the TLS options to the TLS bundle IR
// This function will never exit early, even if an error is encountered.
// It will apply all options and return a wrapped error with all errors encountered.
// Keys outside the kgateway.dev/ prefix are skipped; unknown kgateway.dev/ keys are an error,
// so a misspelled option is reported rather than silently ignored.
func ApplyTLSExtensionOptions(options map[gwv1.AnnotationKey]gwv1.AnnotationValue, out *ir.TLSConfig) error {
	var errs error
	for key, option := range options {
		if !isKgatewayTLSOption(key) {
			continue
		}
		if extensionFunc, ok := TLSExtensionOptionFuncs[key]; ok {
			if err := extensionFunc(string(option), out); err != nil {
				errs = errors.Join(errs, err)
			}
		} else {
			errs = errors.Join(errs, fmt.Errorf("%w: %s", ErrUnknownTLSExtensionOption, key))
		}
	}

	normalizeTLSVersionRange(out)

	if err := validateTLSVersions(out); err != nil {
		errs = errors.Join(errs, err)
	}

	return errs
}

// normalizeTLSVersionRange defaults an unset maximum to the highest supported protocol when
// only a minimum was configured. Without this, an explicit minimum alone leaves the maximum at
// whatever Envoy's own implicit default is, which can be lower than the configured minimum and
// produce an inverted, unusable range.
func normalizeTLSVersionRange(out *ir.TLSConfig) {
	if out.MinTLSVersion != nil && out.MaxTLSVersion == nil {
		maxTLSVersion := envoytlsv3.TlsParameters_TLSv1_3
		out.MaxTLSVersion = &maxTLSVersion
	}
}

// ApplyTLSParameters copies TLS settings from cfg to params. The downstream listener path and
// the upstream BackendTLSPolicy path both call this function.
func ApplyTLSParameters(params *envoytlsv3.TlsParameters, cfg *ir.TLSConfig) {
	if len(cfg.CipherSuites) > 0 {
		params.CipherSuites = cfg.CipherSuites
	}
	if len(cfg.EcdhCurves) > 0 {
		params.EcdhCurves = cfg.EcdhCurves
	}
	if len(cfg.SignatureAlgorithms) > 0 {
		params.SignatureAlgorithms = cfg.SignatureAlgorithms
	}
	if cfg.MinTLSVersion != nil {
		params.TlsMinimumProtocolVersion = *cfg.MinTLSVersion
	}
	if cfg.MaxTLSVersion != nil {
		params.TlsMaximumProtocolVersion = *cfg.MaxTLSVersion
	}
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

// ResolveAlpnProtocols converts a TLSConfig's configured ALPN protocols into the list Envoy
// should be given: defaultProtocols when none were requested, an explicit empty list when the
// AllowEmptyAlpnProtocols sentinel was requested, or the requested list otherwise.
func ResolveAlpnProtocols(configured []string, defaultProtocols []string) []string {
	switch {
	case len(configured) == 0:
		return defaultProtocols
	case len(configured) == 1 && configured[0] == string(annotations.AllowEmptyAlpnProtocols):
		return []string{}
	default:
		return configured
	}
}
