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
// SAN matchers are deliberately left out: they match an identity in addition to the one the
// connection is made under, which Go's own verification of ServerName already covers for the
// single-host fetch a control-plane client performs.
type UpstreamTLSValidation struct {
	// CAPEM is the PEM-encoded CA bundle to verify the upstream against. Empty means the
	// system trust store, which is also what a policy explicitly selecting the well-known
	// system CA set resolves to.
	CAPEM string

	// ServerName is the identity to present as SNI and to verify the upstream's certificate
	// against, empty to use the host from the URL being fetched.
	//
	// It matters because a backend's certificate is routinely issued for the name clients
	// reach it by rather than for its in-cluster Service name: a Service "keycloak" fronting
	// a certificate for "example.com", say. Envoy verifies the name the policy gives it, so a
	// control-plane client has to do the same or it would reject a certificate the proxy
	// accepts.
	ServerName string

	// InsecureSkipVerify disables certificate verification entirely.
	InsecureSkipVerify bool
}

// Equals reports whether two validation configs would produce the same TLS client.
func (v *UpstreamTLSValidation) Equals(other *UpstreamTLSValidation) bool {
	if v == nil || other == nil {
		return v == nil && other == nil
	}
	return v.CAPEM == other.CAPEM &&
		v.ServerName == other.ServerName &&
		v.InsecureSkipVerify == other.InsecureSkipVerify
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
