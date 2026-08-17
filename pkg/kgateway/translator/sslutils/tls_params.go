package sslutils

import (
	"fmt"
	"strings"
)

// SupportedCipherSuites is the set of cipher suite names accepted by
// BoringSSL/Envoy for BackendConfigPolicy TLS parameters. Both the IANA names
// (e.g. "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256") and the OpenSSL short names
// (e.g. "ECDHE-ECDSA-AES128-GCM-SHA256") are recognized by BoringSSL, so both
// conventions are accepted here.
//
// This list is intentionally permissive: omitting a value BoringSSL supports
// would reject otherwise-valid configuration, so prefer accepting a name that
// may later be rejected by Envoy over rejecting a name Envoy would accept.
var SupportedCipherSuites = map[string]struct{}{
	// TLS 1.3 AEAD suites.
	"TLS_AES_128_GCM_SHA256":       {},
	"TLS_AES_256_GCM_SHA384":       {},
	"TLS_CHACHA20_POLY1305_SHA256": {},

	// TLS 1.2 ECDHE AEAD suites (IANA names).
	"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256":       {},
	"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384":       {},
	"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":         {},
	"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":         {},
	"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256": {},
	"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256":   {},
	"TLS_ECDHE_PSK_WITH_CHACHA20_POLY1305_SHA256":   {},
	"TLS_ECDHE_PSK_WITH_AES_128_GCM_SHA256":         {},
	"TLS_ECDHE_PSK_WITH_AES_256_GCM_SHA384":         {},

	// TLS 1.2 ECDHE CBC suites (IANA names).
	"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA":    {},
	"TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA":    {},
	"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA":      {},
	"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA":      {},
	"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256": {},
	"TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA384": {},
	"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256":   {},
	"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA384":   {},

	// TLS 1.2 AEAD suites (OpenSSL short names).
	"ECDHE-ECDSA-AES128-GCM-SHA256": {},
	"ECDHE-ECDSA-AES256-GCM-SHA384": {},
	"ECDHE-RSA-AES128-GCM-SHA256":   {},
	"ECDHE-RSA-AES256-GCM-SHA384":   {},
	"ECDHE-ECDSA-CHACHA20-POLY1305": {},
	"ECDHE-RSA-CHACHA20-POLY1305":   {},
	"ECDHE-PSK-CHACHA20-POLY1305":   {},
	"ECDHE-PSK-AES128-GCM-SHA256":   {},
	"ECDHE-PSK-AES256-GCM-SHA384":   {},

	// TLS 1.2 CBC suites (OpenSSL short names).
	"ECDHE-ECDSA-AES128-SHA":    {},
	"ECDHE-ECDSA-AES256-SHA":    {},
	"ECDHE-RSA-AES128-SHA":      {},
	"ECDHE-RSA-AES256-SHA":      {},
	"ECDHE-ECDSA-AES128-SHA256": {},
	"ECDHE-ECDSA-AES256-SHA384": {},
	"ECDHE-RSA-AES128-SHA256":   {},
	"ECDHE-RSA-AES256-SHA384":   {},

	// Non-ECDHE suites (OpenSSL short names).
	"AES128-GCM-SHA256": {},
	"AES256-GCM-SHA384": {},
	"AES128-SHA":        {},
	"AES256-SHA":        {},
	"AES128-SHA256":     {},
	"AES256-SHA256":     {},
}

// SupportedEcdhCurves is the set of ECDH curve names accepted by
// BoringSSL/Envoy for BackendConfigPolicy TLS parameters.
var SupportedEcdhCurves = map[string]struct{}{
	"X25519":                {},
	"X25519MLKEM768":        {},
	"X25519Kyber768Draft00": {},
	"P-256":                 {},
	"P-384":                 {},
	"P-521":                 {},
}

// ValidateTlsStrings validates and normalizes a user-provided list of TLS
// parameter values (cipher suites or ECDH curves). It trims each entry,
// rejects values not present in supported, and removes duplicates. An empty
// input is valid and returns a nil slice.
func ValidateTlsStrings(input []string, supported map[string]struct{}, fieldName string) ([]string, error) {
	if len(input) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(input))
	cleaned := make([]string, 0, len(input))
	for _, val := range input {
		trimmed := strings.TrimSpace(val)
		if _, ok := supported[trimmed]; !ok {
			return nil, fmt.Errorf("unsupported %s: %q (ensure you are using a BoringSSL-supported name)", fieldName, trimmed)
		}
		if _, dup := seen[trimmed]; dup {
			continue
		}
		seen[trimmed] = struct{}{}
		cleaned = append(cleaned, trimmed)
	}
	return cleaned, nil
}
