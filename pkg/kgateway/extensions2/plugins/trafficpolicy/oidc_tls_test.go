package trafficpolicy

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// newTLSDiscoveryServer starts an HTTPS server serving a well-known OpenID configuration
// under a self-signed certificate, and returns it with its CA bundle in PEM form and a
// counter of the requests that reached it.
func newTLSDiscoveryServer(t *testing.T) (*httptest.Server, string, *int64) {
	t.Helper()

	var requests int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(oidcProviderConfig{
			TokenEndpoint:         "https://private.example.com/token",
			AuthorizationEndpoint: "https://private.example.com/auth",
		})
	}))
	t.Cleanup(server.Close)

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	require.NotEmpty(t, caPEM, "test server should expose its certificate")
	return server, string(caPEM), &requests
}

// TestOIDCDiscoveryHonorsBackendCA is the regression test for
// https://github.com/kgateway-dev/kgateway/issues/14062: an issuer served under a private CA
// could not be discovered, because the control plane only ever trusted the system store, so
// every TrafficPolicy referencing the extension was rejected even though the proxy could
// reach the provider through the CA on its BackendTLSPolicy.
func TestOIDCDiscoveryHonorsBackendCA(t *testing.T) {
	server, caPEM, _ := newTLSDiscoveryServer(t)
	issuer := server.URL

	tests := []struct {
		name          string
		validation    *ir.UpstreamTLSValidation
		expectError   bool
		errorContains string
	}{
		{
			name:          "system trust store cannot verify a private CA",
			validation:    nil,
			expectError:   true,
			errorContains: "certificate signed by unknown authority",
		},
		{
			name:       "the backend's CA bundle verifies the issuer",
			validation: &ir.UpstreamTLSValidation{CAPEM: caPEM},
		},
		{
			name:       "verification can be skipped entirely",
			validation: &ir.UpstreamTLSValidation{InsecureSkipVerify: true},
		},
		{
			name:          "an unparseable CA bundle is reported, not silently ignored",
			validation:    &ir.UpstreamTLSValidation{CAPEM: "-----BEGIN CERTIFICATE-----\nnope\n-----END CERTIFICATE-----"},
			expectError:   true,
			errorContains: "no valid certificate found in the CA bundle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := require.New(t)
			o := newTestDiscoverer(issuer)

			cfg, err := o.get(context.Background(), issuer, tt.validation)
			if tt.expectError {
				r.Error(err)
				r.Nil(cfg)
				r.Contains(err.Error(), tt.errorContains)
				return
			}
			r.NoError(err)
			r.NotNil(cfg)
			r.Equal("https://private.example.com/token", cfg.TokenEndpoint)
		})
	}
}

// TestOIDCDiscoveryCacheKeyedByTrustMaterial asserts that the trust material is part of the
// cache key. Two extensions naming the same issuer may validate it differently, and a config
// discovered under one trust configuration must not be served for another.
func TestOIDCDiscoveryCacheKeyedByTrustMaterial(t *testing.T) {
	r := require.New(t)

	server, caPEM, requests := newTLSDiscoveryServer(t)
	issuer := server.URL
	o := newTestDiscoverer(issuer)

	withCA := &ir.UpstreamTLSValidation{CAPEM: caPEM}
	insecure := &ir.UpstreamTLSValidation{InsecureSkipVerify: true}

	cfg, err := o.get(context.Background(), issuer, withCA)
	r.NoError(err)
	r.NotNil(cfg)
	r.Equal(int64(1), atomic.LoadInt64(requests))

	cfg, err = o.get(context.Background(), issuer, insecure)
	r.NoError(err)
	r.NotNil(cfg)
	r.Equal(int64(2), atomic.LoadInt64(requests),
		"a different trust configuration must discover rather than reuse the cached entry")

	// Both entries now coexist, and each is served from its own cache slot.
	_, err = o.get(context.Background(), issuer, withCA)
	r.NoError(err)
	_, err = o.get(context.Background(), issuer, insecure)
	r.NoError(err)
	r.Equal(int64(2), atomic.LoadInt64(requests), "both entries should be served from cache")
}

// TestTLSDigestNormalizesSystemTrustStore asserts that everything meaning "verify against the
// system trust store" digests alike, so a backend with no policy and one that explicitly
// selects the well-known system CA set share a cache entry instead of discovering twice.
func TestTLSDigestNormalizesSystemTrustStore(t *testing.T) {
	r := require.New(t)

	r.Equal("", tlsDigest(nil))
	r.Equal("", tlsDigest(&ir.UpstreamTLSValidation{}))
	r.Equal("", tlsDigest(&ir.UpstreamTLSValidation{CAPEM: ""}))
}

// TestTLSDigestSeparatesDistinctConfigurations asserts that every field that changes how the
// issuer is reached also changes the cache key, so no configuration is ever served a config
// discovered under another.
func TestTLSDigestSeparatesDistinctConfigurations(t *testing.T) {
	r := require.New(t)

	digests := map[string]string{}
	for name, v := range map[string]*ir.UpstreamTLSValidation{
		"ca a":                  {CAPEM: "ca-a"},
		"ca b":                  {CAPEM: "ca-b"},
		"ca a, server name":     {CAPEM: "ca-a", ServerName: "example.com"},
		"ca a, other name":      {CAPEM: "ca-a", ServerName: "other.example.com"},
		"server name only":      {ServerName: "example.com"},
		"insecure":              {InsecureSkipVerify: true},
		"insecure, server name": {InsecureSkipVerify: true, ServerName: "example.com"},
	} {
		digest := tlsDigest(v)
		r.NotEqual("", digest, "%s must not digest as plain system-trust verification", name)
		if other, clash := digests[digest]; clash {
			r.Failf("digest collision", "%q and %q digest alike", name, other)
		}
		digests[digest] = name
	}

	r.Equal(tlsDigest(&ir.UpstreamTLSValidation{CAPEM: "ca-a"}),
		tlsDigest(&ir.UpstreamTLSValidation{CAPEM: "ca-a"}), "digest must be stable")
}

// TestOIDCDiscoveryVerifiesThePolicysHostname covers the shape the e2e suite runs and that
// real deployments use: the issuer's certificate is issued for the hostname clients reach it
// by, not for the in-cluster Service name the URL names. Envoy verifies the name the policy
// gives it, so discovery has to as well or it rejects a certificate the proxy accepts.
func TestOIDCDiscoveryVerifiesThePolicysHostname(t *testing.T) {
	server, caPEM, _ := newTLSDiscoveryServer(t)

	// The test server's certificate covers example.com and 127.0.0.1, but not localhost, so
	// dialing it by that name stands in for a Service name absent from the certificate.
	issuer := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	r := require.New(t)
	r.NotEqual(server.URL, issuer, "test server should be addressed by IP")

	_, err := newTestDiscoverer(issuer).get(context.Background(), issuer,
		&ir.UpstreamTLSValidation{CAPEM: caPEM})
	r.Error(err, "the URL host is not an identity the certificate carries")
	r.Contains(err.Error(), "certificate is valid for")

	cfg, err := newTestDiscoverer(issuer).get(context.Background(), issuer,
		&ir.UpstreamTLSValidation{CAPEM: caPEM, ServerName: "example.com"})
	r.NoError(err, "the policy's hostname is the identity to verify")
	r.NotNil(cfg)
	r.Equal("https://private.example.com/token", cfg.TokenEndpoint)

	_, err = newTestDiscoverer(issuer).get(context.Background(), issuer,
		&ir.UpstreamTLSValidation{CAPEM: caPEM, ServerName: "wrong.example.net"})
	r.Error(err, "a hostname the certificate does not carry must still fail")
}

// TestRefreshPrunesSupersededTrustKey asserts that rotating the CA on the issuer's backend
// stops the entry discovered under the old CA from being polled. The live set only knows
// issuer URIs, so it cannot spot this on its own: the superseded key belongs to a live issuer
// and would otherwise be re-discovered forever with nothing left to read it.
func TestRefreshPrunesSupersededTrustKey(t *testing.T) {
	r := require.New(t)

	server, caPEM, _ := newTLSDiscoveryServer(t)
	issuer := server.URL
	o := newTestDiscoverer(issuer)

	oldKey := discoveryKey{issuerURI: issuer, tlsDigest: tlsDigest(&ir.UpstreamTLSValidation{InsecureSkipVerify: true})}
	newKey := discoveryKey{issuerURI: issuer, tlsDigest: tlsDigest(&ir.UpstreamTLSValidation{CAPEM: caPEM})}

	_, err := o.get(context.Background(), issuer, &ir.UpstreamTLSValidation{InsecureSkipVerify: true})
	r.NoError(err)

	// Age the first entry so the rotation is unambiguously the more recent read, without
	// depending on the clock resolution between two get() calls.
	o.mu.Lock()
	aged := o.cache[oldKey]
	aged.lastRead = aged.lastRead.Add(-time.Minute)
	o.cache[oldKey] = aged
	o.mu.Unlock()

	// The CA rotates: the transform now asks for the same issuer under new trust material.
	_, err = o.get(context.Background(), issuer, &ir.UpstreamTLSValidation{CAPEM: caPEM})
	r.NoError(err)

	_, cached := o.peek(oldKey)
	r.True(cached, "both keys are cached until a refresh pass runs")

	o.refreshOnce(context.Background())

	_, cached = o.peek(oldKey)
	r.False(cached, "the key superseded by the rotation should be pruned")
	_, cached = o.peek(newKey)
	r.True(cached, "the key still in use must survive the prune")

	// The client for the pruned trust configuration is dropped too, so a rotating CA does not
	// leak a connection pool per rotation.
	o.clients.mu.Lock()
	_, kept := o.clients.clients[oldKey.tlsDigest]
	o.clients.mu.Unlock()
	r.False(kept, "the superseded entry's http client should be released")
}

// fakeTLSPolicyIR is a policy IR carrying trust material, standing in for a BackendTLSPolicy
// or BackendConfigPolicy IR so this package need not depend on either plugin.
type fakeTLSPolicyIR struct {
	// Equals is deliberately unconditional here: these fakes are only ever compared for the
	// provider behavior, never fed to a krt collection.
	// +noKrtEquals
	validation *ir.UpstreamTLSValidation
}

func (f fakeTLSPolicyIR) CreationTime() time.Time                          { return time.Time{} }
func (f fakeTLSPolicyIR) Equals(any) bool                                  { return false }
func (f fakeTLSPolicyIR) UpstreamTLSValidation() *ir.UpstreamTLSValidation { return f.validation }

// fakeOpaquePolicyIR is a policy IR that carries no trust material.
type fakeOpaquePolicyIR struct{}

func (fakeOpaquePolicyIR) CreationTime() time.Time { return time.Time{} }
func (fakeOpaquePolicyIR) Equals(any) bool         { return false }

func backendWithPolicies(policies map[schema.GroupKind][]ir.PolicyAtt) *ir.BackendObjectIR {
	return &ir.BackendObjectIR{AttachedPolicies: ir.AttachedPolicies{Policies: policies}}
}

func TestUpstreamTLSValidationForBackend(t *testing.T) {
	btpGK := wellknown.BackendTLSPolicyGVK.GroupKind()
	bcpGK := wellknown.BackendConfigPolicyGVK.GroupKind()

	btpCA := &ir.UpstreamTLSValidation{CAPEM: "btp-ca"}
	bcpCA := &ir.UpstreamTLSValidation{CAPEM: "bcp-ca"}

	tests := []struct {
		name    string
		backend *ir.BackendObjectIR
		want    *ir.UpstreamTLSValidation
	}{
		{
			name:    "no backend",
			backend: nil,
			want:    nil,
		},
		{
			name:    "no attached policies falls back to the system trust store",
			backend: backendWithPolicies(nil),
			want:    nil,
		},
		{
			name: "BackendTLSPolicy CA is used",
			backend: backendWithPolicies(map[schema.GroupKind][]ir.PolicyAtt{
				btpGK: {{PolicyIr: fakeTLSPolicyIR{validation: btpCA}}},
			}),
			want: btpCA,
		},
		{
			name: "BackendConfigPolicy CA is used",
			backend: backendWithPolicies(map[schema.GroupKind][]ir.PolicyAtt{
				bcpGK: {{PolicyIr: fakeTLSPolicyIR{validation: bcpCA}}},
			}),
			want: bcpCA,
		},
		{
			name: "BackendTLSPolicy wins over BackendConfigPolicy",
			backend: backendWithPolicies(map[schema.GroupKind][]ir.PolicyAtt{
				btpGK: {{PolicyIr: fakeTLSPolicyIR{validation: btpCA}}},
				bcpGK: {{PolicyIr: fakeTLSPolicyIR{validation: bcpCA}}},
			}),
			want: btpCA,
		},
		{
			name: "a policy carrying no trust material is skipped",
			backend: backendWithPolicies(map[schema.GroupKind][]ir.PolicyAtt{
				btpGK: {
					{PolicyIr: fakeTLSPolicyIR{validation: nil}},
					{PolicyIr: fakeTLSPolicyIR{validation: btpCA}},
				},
			}),
			want: btpCA,
		},
		{
			name: "a policy IR that carries no TLS at all is ignored",
			backend: backendWithPolicies(map[schema.GroupKind][]ir.PolicyAtt{
				btpGK: {{PolicyIr: fakeOpaquePolicyIR{}}},
				bcpGK: {{PolicyIr: fakeTLSPolicyIR{validation: bcpCA}}},
			}),
			want: bcpCA,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, upstreamTLSValidationForBackend(tt.backend))
		})
	}
}
