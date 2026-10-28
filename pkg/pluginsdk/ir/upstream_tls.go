package ir

// UpstreamTLSValidation is the trust material a backend-attached policy configures for
// verifying an upstream's certificate, reduced to what a Go TLS client needs. Its only caller
// is OIDC discovery in the trafficpolicy plugin.
//
// Only trust anchors are carried, not the identity to verify: a control-plane client
// authenticates the host of the URL it dials, which need not be the backend's hostname.
//
// No krt collection emits it, so it has no Equals; consumers key on its contents.
type UpstreamTLSValidation struct {
	// CAPEM is the PEM-encoded CA bundle to verify the upstream against. Empty means the
	// system trust store.
	CAPEM string
}

// UpstreamTLSValidationProvider is implemented by backend-attached policy IRs that carry TLS
// trust material. Returning nil means the system trust store.
type UpstreamTLSValidationProvider interface {
	UpstreamTLSValidation() *UpstreamTLSValidation
}
