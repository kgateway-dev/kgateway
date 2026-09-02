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
// Only trust material is carried. SNI and SAN matchers are deliberately left out: a
// control-plane client dials the upstream's URL directly rather than through the backend's
// Envoy cluster, so the URL host is always the right name to verify against.
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
// given as a file path on the proxy's filesystem is one such case: that path does not exist
// in the control plane.
type UpstreamTLSValidationProvider interface {
	UpstreamTLSValidation() *UpstreamTLSValidation
}
