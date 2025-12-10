package sslutils

import (
	"testing"

	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
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
			name: "tls_max_version",
			out: &ir.TLSConfig{
				MaxTLSVersion: ptr.To(envoytlsv3.TlsParameters_TLSv1_2),
			},
			in: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				annotations.MaxTLSVersion: "1.2",
			},
		},
		{
			name: "tls_min_version",
			out: &ir.TLSConfig{
				MinTLSVersion: ptr.To(envoytlsv3.TlsParameters_TLSv1_3),
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
				MinTLSVersion:         ptr.To(envoytlsv3.TlsParameters_TLSv1_3),
				MaxTLSVersion:         ptr.To(envoytlsv3.TlsParameters_TLSv1_2),
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
				MaxTLSVersion:         ptr.To(envoytlsv3.TlsParameters_TLSv1_3),
				MinTLSVersion:         ptr.To(envoytlsv3.TlsParameters_TLSv1_2),
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
