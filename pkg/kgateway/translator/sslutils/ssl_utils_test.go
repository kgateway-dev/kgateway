package sslutils

import (
	"strings"
	"testing"

	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/stretchr/testify/assert"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestApplyTLSExtensionOptions(t *testing.T) {
	testCases := []struct {
		name   string
		in     map[gwv1.AnnotationKey]gwv1.AnnotationValue
		out    *ir.TLSConfig
		errors []string
	}{
		{
			name: "cipher_suites",
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.CipherSuites: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
			},
			out: &ir.TLSConfig{
				CipherSuites: []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"},
			},
		},
		{
			name: "cipher_suites_with_whitespace",
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.CipherSuites: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
			},
			out: &ir.TLSConfig{
				CipherSuites: []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"},
			},
		},
		{
			name: "ecdh_curves",
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.EcdhCurves: "X25519MLKEM768,X25519,P-256",
			},
			out: &ir.TLSConfig{
				EcdhCurves: []string{"X25519MLKEM768", "X25519", "P-256"},
			},
		},
		{
			name: "ecdh_curves_with_whitespace",
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.EcdhCurves: "X25519MLKEM768, X25519, P-256",
			},
			out: &ir.TLSConfig{
				EcdhCurves: []string{"X25519MLKEM768", "X25519", "P-256"},
			},
		},
		{
			name: "signature_algorithms",
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.SignatureAlgorithms: "ecdsa_secp256r1_sha256,rsa_pss_rsae_sha256,rsa_pkcs1_sha256",
			},
			out: &ir.TLSConfig{
				SignatureAlgorithms: []string{"ecdsa_secp256r1_sha256", "rsa_pss_rsae_sha256", "rsa_pkcs1_sha256"},
			},
		},
		{
			name: "signature_algorithms_with_whitespace",
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.SignatureAlgorithms: "ecdsa_secp256r1_sha256, rsa_pss_rsae_sha256, rsa_pkcs1_sha256",
			},
			out: &ir.TLSConfig{
				SignatureAlgorithms: []string{"ecdsa_secp256r1_sha256", "rsa_pss_rsae_sha256", "rsa_pkcs1_sha256"},
			},
		},
		{
			name: "subject_alt_names",
			out: &ir.TLSConfig{
				VerifySubjectAltNames: []string{"foo", "bar"},
			},
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.VerifySubjectAltNames: "foo,bar",
			},
		},
		{
			name: "subject_alt_names_with_whitespace",
			out: &ir.TLSConfig{
				VerifySubjectAltNames: []string{"foo", "bar"},
			},
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.VerifySubjectAltNames: "foo, bar",
			},
		},
		{
			name: "alpn_protocols",
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.AlpnProtocols: "h2,http/1.1",
			},
			out: &ir.TLSConfig{
				AlpnProtocols: []string{"h2", "http/1.1"},
			},
		},
		{
			name: "alpn_protocols_with_whitespace",
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.AlpnProtocols: "h2, http/1.1",
			},
			out: &ir.TLSConfig{
				AlpnProtocols: []string{"h2", "http/1.1"},
			},
		},
		{
			// Empty entries are dropped, so an empty value leaves the default in place.
			name: "empty_entries",
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.CipherSuites:        "",
				annotations.EcdhCurves:          "X25519,,P-256,",
				annotations.SignatureAlgorithms: " , ",
				annotations.AlpnProtocols:       "h2,",
			},
			out: &ir.TLSConfig{
				EcdhCurves:    []string{"X25519", "P-256"},
				AlpnProtocols: []string{"h2"},
			},
		},
		{
			name: "cipher_suites_equal_preference_group",
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.CipherSuites: "[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305],ECDHE-RSA-AES256-GCM-SHA384",
			},
			out: &ir.TLSConfig{
				CipherSuites: []string{"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]", "ECDHE-RSA-AES256-GCM-SHA384"},
			},
		},
		{
			name: "cipher_suites_tls13_alongside_tls12",
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.CipherSuites: "TLS_AES_128_GCM_SHA256,ECDHE-RSA-AES128-GCM-SHA256",
			},
			out: &ir.TLSConfig{
				CipherSuites: []string{"TLS_AES_128_GCM_SHA256", "ECDHE-RSA-AES128-GCM-SHA256"},
			},
		},
		{
			// Envoy rejects the whole listener or cluster for any of these.
			name: "unknown_tls_parameters",
			out:  &ir.TLSConfig{},
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.CipherSuites:        "ECDHE-RSA-AES128-GCM-SHA256,NO-SUCH-CIPHER",
				annotations.EcdhCurves:          "P-256,NO-SUCH-CURVE",
				annotations.SignatureAlgorithms: "ed25519,no_such_algorithm",
				annotations.AlpnProtocols:       gwv1.AnnotationValue("h2," + strings.Repeat("a", 256)),
			},
			errors: []string{
				"invalid tls parameter: unknown cipher suite: NO-SUCH-CIPHER",
				"invalid tls parameter: unknown ecdh curve: NO-SUCH-CURVE",
				"invalid tls parameter: unknown signature algorithm: no_such_algorithm",
				"invalid tls parameter: an alpn protocol must be 1 to 255 bytes long",
			},
		},
		{
			name: "duplicate_tls_parameters",
			out:  &ir.TLSConfig{},
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.EcdhCurves:          "P-256,X25519,prime256v1",
				annotations.SignatureAlgorithms: "rsa_pss_rsae_sha256,rsa_pss_rsae_sha256",
			},
			errors: []string{
				"invalid tls parameter: duplicate ecdh curve: prime256v1",
				"invalid tls parameter: duplicate signature algorithm: rsa_pss_rsae_sha256",
			},
		},
		{
			name: "tls_max_version",
			out: &ir.TLSConfig{
				MaxTLSVersion: new(envoytlsv3.TlsParameters_TLSv1_2),
			},
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.MaxTLSVersion: "1.2",
			},
		},
		{
			// A minimum alone normalizes the maximum to the highest supported protocol, so it
			// never leaves an inverted range against Envoy's own implicit default maximum.
			name: "tls_min_version",
			out: &ir.TLSConfig{
				MinTLSVersion: new(envoytlsv3.TlsParameters_TLSv1_3),
				MaxTLSVersion: new(envoytlsv3.TlsParameters_TLSv1_3),
			},
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.MinTLSVersion: "1.3",
			},
		},
		{
			name: "invalid_tls_versions",
			out:  &ir.TLSConfig{},
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.MinTLSVersion: "TLSv1.2",
				annotations.MaxTLSVersion: "TLSv1.3",
			},
			errors: []string{
				"invalid maximum tls version: TLSv1.3",
				"invalid minimum tls version: TLSv1.2",
			},
		},
		{
			name: "maximum_tls_version_less_than_minimum",
			out: &ir.TLSConfig{
				VerifySubjectAltNames: []string{"foo", "bar"},
				MinTLSVersion:         new(envoytlsv3.TlsParameters_TLSv1_3),
				MaxTLSVersion:         new(envoytlsv3.TlsParameters_TLSv1_2),
			},
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.MinTLSVersion:         "1.3",
				annotations.MaxTLSVersion:         "1.2",
				annotations.VerifySubjectAltNames: "foo,bar",
			},
			errors: []string{
				"maximum tls version TLSv1_2 is less than minimum tls version TLSv1_3",
			},
		},
		{
			name: "multiple_options",
			out: &ir.TLSConfig{
				VerifySubjectAltNames: []string{"foo", "bar"},
				MaxTLSVersion:         new(envoytlsv3.TlsParameters_TLSv1_3),
				MinTLSVersion:         new(envoytlsv3.TlsParameters_TLSv1_2),
				CipherSuites:          []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"},
				EcdhCurves:            []string{"X25519MLKEM768", "X25519", "P-256"},
			},
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.MaxTLSVersion:         "1.3",
				annotations.MinTLSVersion:         "1.2",
				annotations.VerifySubjectAltNames: "foo,bar",
				annotations.CipherSuites:          "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
				annotations.EcdhCurves:            "X25519MLKEM768,X25519,P-256",
			},
		},
		{
			name: "misspelled_option",
			out:  &ir.TLSConfig{},
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.MinTLSVersion + "s": "TLSv1_3",
			},
			errors: []string{
				"unknown tls option: kgateway.dev/min-tls-versions",
			},
		},
		{
			// TLS options maps are shared with other implementations, so keys outside the
			// kgateway.dev/ prefix are skipped, while kgateway keys alongside them still apply.
			name: "foreign_options_skipped",
			out: &ir.TLSConfig{
				EcdhCurves: []string{"P-384"},
			},
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				"example.com/other-implementation": "value",
				"unprefixed-option":                "value",
				annotations.EcdhCurves:             "P-384",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out := &ir.TLSConfig{}
			err := ApplyTLSExtensionOptions(tc.in, out)
			assert.Equal(t, tc.out, out)
			if len(tc.errors) > 0 {
				assert.Error(t, err)
				for _, errMsg := range tc.errors {
					assert.Contains(t, err.Error(), errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCipherSuites(t *testing.T) {
	testCases := []struct {
		name         string
		cipherSuites []string
		valid        bool
	}{
		{name: "none", cipherSuites: nil, valid: true},
		{name: "openssl_name", cipherSuites: []string{"ECDHE-ECDSA-AES128-GCM-SHA256"}, valid: true},
		{name: "iana_name", cipherSuites: []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"}, valid: true},
		{name: "repeated", cipherSuites: []string{"AES128-SHA", "AES128-SHA"}, valid: true},
		{name: "group", cipherSuites: []string{"[AES128-SHA|AES256-SHA]"}, valid: true},
		{name: "group_with_tls13", cipherSuites: []string{"[TLS_AES_128_GCM_SHA256|AES256-SHA]"}, valid: true},
		{name: "only_tls13", cipherSuites: []string{"TLS_AES_128_GCM_SHA256", "[TLS_AES_256_GCM_SHA384]"}, valid: false},
		{name: "empty_entry", cipherSuites: []string{"AES128-SHA", ""}, valid: false},
		{name: "wrong_case", cipherSuites: []string{"aes128-sha"}, valid: false},
		{name: "unknown_in_group", cipherSuites: []string{"[AES128-SHA|NO-SUCH-CIPHER]"}, valid: false},
		{name: "empty_group", cipherSuites: []string{"AES128-SHA", "[]"}, valid: false},
		{name: "unterminated_group", cipherSuites: []string{"[AES128-SHA"}, valid: false},
		{name: "nested_group", cipherSuites: []string{"[[AES128-SHA]]"}, valid: false},
		{name: "colon_separated", cipherSuites: []string{"AES128-SHA:AES256-SHA"}, valid: false},
		// Known to BoringSSL, but these leave the list empty, which Envoy rejects.
		{name: "remove_operator", cipherSuites: []string{"AES128-SHA", "-ALL"}, valid: false},
		{name: "kill_operator", cipherSuites: []string{"!ALL"}, valid: false},
		{name: "disjoint_aliases", cipherSuites: []string{"aECDSA+kRSA"}, valid: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCipherSuites(tc.cipherSuites)
			if tc.valid {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, ErrInvalidTLSParameter)
			}
		})
	}
}

func TestValidateAlpnProtocols(t *testing.T) {
	repeat := func(protocol string, n int) []string {
		return strings.Split(strings.Repeat(protocol+",", n-1)+protocol, ",")
	}

	testCases := []struct {
		name          string
		alpnProtocols []string
		valid         bool
	}{
		{name: "none", alpnProtocols: nil, valid: true},
		{name: "common", alpnProtocols: []string{"h2", "http/1.1"}, valid: true},
		{name: "longest", alpnProtocols: []string{strings.Repeat("a", 255)}, valid: true},
		{name: "too_long", alpnProtocols: []string{strings.Repeat("a", 256)}, valid: false},
		// An upstream TLS context asserts on an empty protocol, which takes Envoy down.
		{name: "empty_entry", alpnProtocols: []string{"h2", ""}, valid: false},
		{name: "empty_entry_after_envoy_splits_on_commas", alpnProtocols: []string{"h2,"}, valid: false},
		{name: "longest_list", alpnProtocols: repeat(strings.Repeat("a", 254), 257), valid: true},
		{name: "list_too_long", alpnProtocols: repeat(strings.Repeat("a", 255), 256), valid: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAlpnProtocols(tc.alpnProtocols)
			if tc.valid {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, ErrInvalidTLSParameter)
			}
		})
	}
}

func TestValidateCertificateHash(t *testing.T) {
	testCases := []struct {
		name  string
		hash  string
		valid bool
	}{
		{name: "valid_hash_64_chars", hash: "FC1B6F225ED06510D268A47E3363C19948DD19BBB8A92C211605F4604757ABB9", valid: true},
		{name: "valid_hash_32_pairs", hash: "FC:1B:6F:22:5E:D0:65:10:D2:68:A4:7E:33:63:C1:99:48:DD:19:BB:B8:A9:2C:21:16:05:F4:60:47:57:AB:B9", valid: true},
		{name: "invalid_hash_64_chars_invalid_hex", hash: "FC1B6F225ED06510D268A47E3363C19948DD19BBB8A92C211605F4604757ABAG", valid: false},
		{name: "invalid_hash_32_pairs_invalid_hex", hash: "FC:1B:6F:22:5E:D0:65:10:D2:68:A4:7E:33:63:C1:99:48:DD:19:BB:B8:A9:2C:21:16:05:F4:60:47:57:AB:BG", valid: false},
		{name: "invalid_hash_64_chars_too_short", hash: "FC1B6F225ED06510D268A47E3363C19948DD19BBB8A92C211605F4604757AB", valid: false},
		{name: "invalid_hash_32_pairs_too_short", hash: "FC:1B:6F:22:5E:D0:65:10:D2:68:A4:7E:33:63:C1:99:48:DD:19:BB:B8:A9:2C:21:16:05:F4:60:47:57:AB:B", valid: false},
		{name: "invalid_hash_64_chars_too_long", hash: "FC1B6F225ED06510D268A47E3363C19948DD19BBB8A92C211605F4604757ABB90", valid: false},
		{name: "invalid_hash_32_pairs_too_long", hash: "FC:1B:6F:22:5E:D0:65:10:D2:68:A4:7E:33:63:C1:99:48:DD:19:BB:B8:A9:2C:21:16:05:F4:60:47:57:AB:B90", valid: false},
		{name: "invalid_hash_64_chars_with_colon", hash: "FC1B6F225ED06510D268A47E3363C19948DD19BBB8A92C211605F4604757AB:B9", valid: false},
		{name: "invalid_hash_32_pairs_missing_colon", hash: "FC:1B:6F:22:5E:D0:65:10:D2:68:A4:7E:33:63:C1:99:48:DD:19:BB:B8:A9:2C:21:1605:F4:60:47:57:AB:B9", valid: false},
		{name: "invalid_hash_32_pairs_trailing_colon", hash: "FC:1B:6F:22:5E:D0:65:10:D2:68:A4:7E:33:63:C1:99:48:DD:19:BB:B8:A9:2C:21:16:05:F4:60:47:57:AB:B9:", valid: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCertificateHash(tc.hash)
			assert.Equal(t, tc.valid, err == nil, "input: %s, expected: %v, got %v", tc.hash, tc.valid, err != nil)
		})
	}
}

func TestSplitFakeYamlArray(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected []string
	}{
		{name: "comma_separated", input: "str1,str2,str3", expected: []string{"str1", "str2", "str3"}},
		{name: "dash_separated", input: "str1-str2-str3", expected: []string{"str1", "str2", "str3"}},
		{name: "mixed_separated", input: "str1,str2-str3", expected: []string{"str1", "str2", "str3"}},
		{name: "whitespace_separated", input: "str1, str2, str3", expected: []string{"str1", "str2", "str3"}},
		{name: "mixed_whitespace_separated", input: "str1, str2- str3", expected: []string{"str1", "str2", "str3"}},
		{name: "mixed_whitespace_comma_separated", input: "str1, str2, str3", expected: []string{"str1", "str2", "str3"}},
		{name: "newlines_and_commas_with_whitespace", input: "str1,\n   str2,\n   str3", expected: []string{"str1", "str2", "str3"}},
		{name: "newlines_and_dashes_with_whitespace", input: "- str1\n   - str2\n   - str3", expected: []string{"str1", "str2", "str3"}},
		{name: "all_types_of_whitespace_mixed_separated", input: "str1, \r\n   str2- \n \t str3", expected: []string{"str1", "str2", "str3"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := splitFakeYamlArray(tc.input)
			assert.ElementsMatch(t, tc.expected, actual, "input: %s, expected: %v, got %v", tc.input, tc.expected, actual)
		})
	}
}
