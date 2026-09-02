package trafficpolicy

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
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

var errTestInvalidPolicy = errors.New("invalid CA certificate ref")

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

			cfg, err := o.get(context.Background(), testExtName, issuer, tt.validation)
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

// TestOIDCDiscoveryAuthenticatesTheIssuerURLHost pins that a backend policy supplies trust
// anchors only, never the identity to verify. Discovery dials the issuer URL itself rather
// than routing through the backend's Envoy cluster, so that URL's host is the name that must
// be authenticated; borrowing the policy's SNI/hostname instead would authenticate a
// different server, and would break the common case of an issuer whose host differs from the
// backendRef's (accounts.google.com fronted by an oauth2.googleapis.com backend).
func TestOIDCDiscoveryAuthenticatesTheIssuerURLHost(t *testing.T) {
	r := require.New(t)

	server, caPEM, _ := newTLSDiscoveryServer(t)

	// The test server's certificate covers example.com and 127.0.0.1, but not localhost.
	issuerByIP := server.URL
	issuerByName := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	r.NotEqual(issuerByIP, issuerByName, "test server should be addressed by IP")

	cfg, err := newTestDiscoverer(issuerByIP).get(context.Background(), testExtName, issuerByIP,
		&ir.UpstreamTLSValidation{CAPEM: caPEM})
	r.NoError(err, "the issuer URL's host is covered by the certificate")
	r.NotNil(cfg)

	_, err = newTestDiscoverer(issuerByName).get(context.Background(), testExtName, issuerByName,
		&ir.UpstreamTLSValidation{CAPEM: caPEM})
	r.Error(err, "a host the certificate does not carry must fail even with the right CA")
	r.Contains(err.Error(), "certificate is valid for")
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

	cfg, err := o.get(context.Background(), testExtName, issuer, withCA)
	r.NoError(err)
	r.NotNil(cfg)
	r.Equal(int64(1), atomic.LoadInt64(requests))

	cfg, err = o.get(context.Background(), testExtName, issuer, insecure)
	r.NoError(err)
	r.NotNil(cfg)
	r.Equal(int64(2), atomic.LoadInt64(requests),
		"a different trust configuration must discover rather than reuse the cached entry")

	// Both entries are still cached, and each is served from its own slot.
	_, err = o.get(context.Background(), testExtName, issuer, withCA)
	r.NoError(err)
	_, err = o.get(context.Background(), testExtName, issuer, insecure)
	r.NoError(err)
	r.Equal(int64(2), atomic.LoadInt64(requests), "both entries should be served from cache")
}

// TestTLSDigestNormalizesSystemTrustStore asserts that everything meaning "the system trust
// store" digests alike, so a backend with no policy and one that explicitly selects the
// well-known system CA set share a cache entry instead of discovering the issuer twice.
func TestTLSDigestNormalizesSystemTrustStore(t *testing.T) {
	r := require.New(t)

	r.Equal("", tlsDigest(nil))
	r.Equal("", tlsDigest(&ir.UpstreamTLSValidation{}))
	r.Equal("", tlsDigest(&ir.UpstreamTLSValidation{CAPEM: ""}))
}

// TestTLSDigestSeparatesDistinctConfigurations asserts that every configuration that changes
// how the issuer is verified also changes the cache key.
func TestTLSDigestSeparatesDistinctConfigurations(t *testing.T) {
	r := require.New(t)

	digests := map[string]string{}
	for name, v := range map[string]*ir.UpstreamTLSValidation{
		"ca a":     {CAPEM: "ca-a"},
		"ca b":     {CAPEM: "ca-b"},
		"insecure": {InsecureSkipVerify: true},
	} {
		digest := tlsDigest(v)
		r.NotEqual("", digest, "%s must not digest as system-trust verification", name)
		if other, clash := digests[digest]; clash {
			r.Failf("digest collision", "%q and %q digest alike", name, other)
		}
		digests[digest] = name
	}

	r.Equal(tlsDigest(&ir.UpstreamTLSValidation{CAPEM: "ca-a"}),
		tlsDigest(&ir.UpstreamTLSValidation{CAPEM: "ca-a"}), "digest must be stable")
}

// TestRefreshReleasesRotatedTrustKey asserts that rotating the CA on the issuer's backend
// stops the entry discovered under the old CA from being polled: the extension rebinds to the
// new entry, leaving the old one with no consumer.
func TestRefreshReleasesRotatedTrustKey(t *testing.T) {
	r := require.New(t)

	server, caPEM, _ := newTLSDiscoveryServer(t)
	issuer := server.URL
	o := newTestDiscoverer(issuer)

	oldKey := discoveryKey{issuerURI: issuer, tlsDigest: tlsDigest(&ir.UpstreamTLSValidation{InsecureSkipVerify: true})}
	newKey := discoveryKey{issuerURI: issuer, tlsDigest: tlsDigest(&ir.UpstreamTLSValidation{CAPEM: caPEM})}

	_, err := o.get(context.Background(), testExtName, issuer, &ir.UpstreamTLSValidation{InsecureSkipVerify: true})
	r.NoError(err)

	// The CA rotates: the same extension is translated again under new trust material.
	_, err = o.get(context.Background(), testExtName, issuer, &ir.UpstreamTLSValidation{CAPEM: caPEM})
	r.NoError(err)

	_, cached := o.load(oldKey)
	r.True(cached, "both entries are cached until a refresh pass runs")

	o.refreshOnce(context.Background())

	_, cached = o.load(oldKey)
	r.False(cached, "the entry the rotation released should be pruned")
	_, cached = o.load(newKey)
	r.True(cached, "the entry the extension is bound to must survive the prune")

	// The client for the released trust configuration is dropped too, so a rotating CA does
	// not leak a connection pool per rotation.
	o.clients.mu.Lock()
	_, kept := o.clients.clients[oldKey.tlsDigest]
	o.clients.mu.Unlock()
	r.False(kept, "the released entry's http client should be released")
}

// TestRefreshRetainsSharedIssuerUnderDifferentCAs is the case a most-recently-used heuristic
// cannot serve: two live extensions discover from the same issuer but validate it differently,
// so both entries have a consumer and both must survive every refresh pass. Were either
// pruned, its next translation would block the krt event loop on a foreground fetch, which is
// what the cache exists to avoid.
func TestRefreshRetainsSharedIssuerUnderDifferentCAs(t *testing.T) {
	r := require.New(t)

	server, caPEM, requests := newTLSDiscoveryServer(t)
	issuer := server.URL

	otherSource := ir.ObjectSource{Namespace: "ns", Name: "other-ext"}
	otherExtName := otherSource.ResourceName()
	exts := []ir.GatewayExtension{
		testExtension(issuer),
		{ObjectSource: otherSource, OAuth2: testExtension(issuer).OAuth2},
	}
	o := newOIDCProviderConfigDiscoverer(func() []ir.GatewayExtension { return exts })
	// Never expire, so this test observes pruning alone rather than re-discovery.
	o.cacheRefreshInterval = time.Hour
	o.failureRetryInterval = time.Hour

	withCA := &ir.UpstreamTLSValidation{CAPEM: caPEM}
	insecure := &ir.UpstreamTLSValidation{InsecureSkipVerify: true}

	_, err := o.get(context.Background(), testExtName, issuer, withCA)
	r.NoError(err)
	_, err = o.get(context.Background(), otherExtName, issuer, insecure)
	r.NoError(err)
	r.Equal(int64(2), atomic.LoadInt64(requests))

	o.refreshOnce(context.Background())

	_, cached := o.load(discoveryKey{issuerURI: issuer, tlsDigest: tlsDigest(withCA)})
	r.True(cached, "the entry the first extension reads must survive")
	_, cached = o.load(discoveryKey{issuerURI: issuer, tlsDigest: tlsDigest(insecure)})
	r.True(cached, "the entry the second extension reads must survive")

	// Both are still served from cache, so neither extension re-fetches on translation.
	_, err = o.get(context.Background(), testExtName, issuer, withCA)
	r.NoError(err)
	_, err = o.get(context.Background(), otherExtName, issuer, insecure)
	r.NoError(err)
	r.Equal(int64(2), atomic.LoadInt64(requests), "neither entry should have been re-discovered")
}

// fakeTLSPolicyIR is a policy IR carrying trust material, standing in for a BackendTLSPolicy
// IR so this package need not depend on that plugin.
type fakeTLSPolicyIR struct {
	// Equals is deliberately unconditional here: these fakes are only ever compared for the
	// provider behavior, never fed to a krt collection.
	// +noKrtEquals
	validation *ir.UpstreamTLSValidation
	// +noKrtEquals
	ct time.Time
}

func (f fakeTLSPolicyIR) CreationTime() time.Time                          { return f.ct }
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

	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)

	winnerCA := &ir.UpstreamTLSValidation{CAPEM: "winner-ca"}
	loserCA := &ir.UpstreamTLSValidation{CAPEM: "loser-ca"}

	tests := []struct {
		name          string
		backend       *ir.BackendObjectIR
		want          *ir.UpstreamTLSValidation
		errorContains string
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
			name: "the BackendTLSPolicy CA is used",
			backend: backendWithPolicies(map[schema.GroupKind][]ir.PolicyAtt{
				btpGK: {{PolicyIr: fakeTLSPolicyIR{validation: winnerCA, ct: older}}},
			}),
			want: winnerCA,
		},
		{
			// The backend translator collapses multiple attachments through MergePolicies,
			// which picks the oldest. Scanning attachments in slice order would disagree.
			name: "several policies resolve to the merge winner, not the first attachment",
			backend: backendWithPolicies(map[schema.GroupKind][]ir.PolicyAtt{
				btpGK: {
					{PolicyIr: fakeTLSPolicyIR{validation: loserCA, ct: newer}},
					{PolicyIr: fakeTLSPolicyIR{validation: winnerCA, ct: older}},
				},
			}),
			want: winnerCA,
		},
		{
			// An erroring policy contributes nothing to the cluster, and a BackendTLSPolicy
			// being attached at all suppresses BackendConfigPolicy's TLS. So the proxy gets no
			// TLS config here: falling back to the system trust store would have discovery
			// quietly succeed against a backend the proxy cannot talk to.
			name: "an invalid effective policy is reported rather than skipped",
			backend: backendWithPolicies(map[schema.GroupKind][]ir.PolicyAtt{
				btpGK: {{
					PolicyIr: fakeTLSPolicyIR{ct: older},
					Errors:   []error{errTestInvalidPolicy},
				}},
			}),
			errorContains: "BackendTLSPolicy in effect on the issuer backend is invalid",
		},
		{
			name: "an invalid loser does not mask a valid winner",
			backend: backendWithPolicies(map[schema.GroupKind][]ir.PolicyAtt{
				btpGK: {
					{PolicyIr: fakeTLSPolicyIR{validation: winnerCA, ct: older}},
					{PolicyIr: fakeTLSPolicyIR{ct: newer}, Errors: []error{errTestInvalidPolicy}},
				},
			}),
			want: winnerCA,
		},
		{
			// BackendConfigPolicy TLS is not consulted: honoring it needs its field-wise
			// merge, which is not reachable here.
			name: "a BackendConfigPolicy alone leaves the system trust store",
			backend: backendWithPolicies(map[schema.GroupKind][]ir.PolicyAtt{
				bcpGK: {{PolicyIr: fakeTLSPolicyIR{validation: loserCA, ct: older}}},
			}),
			want: nil,
		},
		{
			name: "a policy IR that carries no TLS at all is ignored",
			backend: backendWithPolicies(map[schema.GroupKind][]ir.PolicyAtt{
				btpGK: {{PolicyIr: fakeOpaquePolicyIR{}}},
			}),
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := upstreamTLSValidationForBackend(tt.backend)
			if tt.errorContains != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorContains)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
