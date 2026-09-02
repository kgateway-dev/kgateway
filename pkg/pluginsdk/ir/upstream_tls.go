package ir

// UpstreamTLSValidation is the trust material a backend-attached policy configures for
// verifying an upstream's certificate, reduced to what a Go TLS client needs.
//
// It exists because the control plane occasionally has to talk to a backend itself rather
// than only program Envoy to: OIDC discovery fetches the OpenID provider configuration from
// the issuer during translation. Such a client has to trust the same CA the user configured
// for the data path, or a provider behind a private CA fails discovery even though the proxy
// can reach it perfectly well.
//
// Only trust anchors and the verification mode are carried. The identity to verify is
// deliberately not: a control-plane client dials the upstream's own URL rather than routing
// through the backend's Envoy cluster, and that URL's host is the name it must authenticate.
// A policy's SNI/hostname describes the backend, which is not necessarily the same server —
// an OAuth2 backendRef routinely points at a provider's token-endpoint host while the issuer
// URI names a different one (accounts.google.com versus oauth2.googleapis.com, say).
type UpstreamTLSValidation struct {
	// CAPEM is the PEM-encoded CA bundle to verify the upstream against. Empty means the
	// system trust store, which is also what a policy explicitly selecting the well-known
	// system CA set resolves to.
	CAPEM string

	// InsecureSkipVerify disables certificate verification entirely.
	InsecureSkipVerify bool
}

// Equals reports whether two validation configs would produce the same TLS client.
func (v *UpstreamTLSValidation) Equals(other *UpstreamTLSValidation) bool {
	if v == nil || other == nil {
		return v == nil && other == nil
	}
	return v.CAPEM == other.CAPEM && v.InsecureSkipVerify == other.InsecureSkipVerify
}

// UpstreamTLSValidationProvider is implemented by backend-attached policy IRs that carry TLS
// trust material, so that a control-plane client can validate an upstream the same way the
// proxy does without depending on the plugin that produced the policy.
//
// Returning nil means the policy configures no trust material a Go client can use, and the
// caller should fall back to its own default of the system trust store. A policy whose CA is
// given as a file path on the proxy's filesystem would be one such case: that path does not
// exist in the control plane.
type UpstreamTLSValidationProvider interface {
	UpstreamTLSValidation() *UpstreamTLSValidation
}
