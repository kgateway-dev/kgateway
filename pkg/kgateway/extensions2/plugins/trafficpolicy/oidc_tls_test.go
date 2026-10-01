package trafficpolicy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
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

// unrelatedCAPEM returns a freshly generated self-signed CA certificate in PEM form that signs
// nothing any test server presents. Appending it to a server's CA yields a bundle that still
// verifies that server but digests differently: a second, distinct trust configuration.
func unrelatedCAPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "unrelated test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

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
			name:       "a bundle carrying additional roots still verifies the issuer",
			validation: &ir.UpstreamTLSValidation{CAPEM: caPEM + unrelatedCAPEM(t)},
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

// TestTLSClientConfigExtendsSystemRoots pins that a policy's CA bundle is added to the system
// roots rather than replacing them. Discovery dials the issuer URL, not the backend the policy
// describes, and the two are routinely different servers: a private-CA backendRef fronting a
// public-CA issuer discovered fine before backend policies were honored, and must keep doing so.
func TestTLSClientConfigExtendsSystemRoots(t *testing.T) {
	r := require.New(t)

	system, err := x509.SystemCertPool()
	if err != nil {
		t.Skipf("no system cert pool on this platform: %v", err)
	}
	caPEM := unrelatedCAPEM(t)
	want := system.Clone()
	r.True(want.AppendCertsFromPEM([]byte(caPEM)))

	cfg, err := tlsClientConfig(&ir.UpstreamTLSValidation{CAPEM: caPEM})
	r.NoError(err)
	r.NotNil(cfg)
	r.True(cfg.RootCAs.Equal(want), "the client's roots should be the system roots plus the policy's CA")
	r.False(cfg.RootCAs.Equal(system), "the policy's CA must actually be trusted")
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
	withExtraRoot := &ir.UpstreamTLSValidation{CAPEM: caPEM + unrelatedCAPEM(t)}

	cfg, err := o.get(context.Background(), testExtName, issuer, withCA)
	r.NoError(err)
	r.NotNil(cfg)
	r.Equal(int64(1), atomic.LoadInt64(requests))

	cfg, err = o.get(context.Background(), testExtName, issuer, withExtraRoot)
	r.NoError(err)
	r.NotNil(cfg)
	r.Equal(int64(2), atomic.LoadInt64(requests),
		"a different trust configuration must discover rather than reuse the cached entry")

	// Both entries are still cached, and each is served from its own slot.
	_, err = o.get(context.Background(), testExtName, issuer, withCA)
	r.NoError(err)
	_, err = o.get(context.Background(), testExtName, issuer, withExtraRoot)
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
		"ca a":       {CAPEM: "ca-a"},
		"ca b":       {CAPEM: "ca-b"},
		"ca a and b": {CAPEM: "ca-a" + "ca-b"},
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

	oldCA := &ir.UpstreamTLSValidation{CAPEM: caPEM + unrelatedCAPEM(t)}
	newCA := &ir.UpstreamTLSValidation{CAPEM: caPEM}
	oldKey := discoveryKey{issuerURI: issuer, tlsDigest: tlsDigest(oldCA)}
	newKey := discoveryKey{issuerURI: issuer, tlsDigest: tlsDigest(newCA)}

	_, err := o.get(context.Background(), testExtName, issuer, oldCA)
	r.NoError(err)

	// The CA rotates: the same extension is translated again under new trust material.
	_, err = o.get(context.Background(), testExtName, issuer, newCA)
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
	withExtraRoot := &ir.UpstreamTLSValidation{CAPEM: caPEM + unrelatedCAPEM(t)}

	_, err := o.get(context.Background(), testExtName, issuer, withCA)
	r.NoError(err)
	_, err = o.get(context.Background(), otherExtName, issuer, withExtraRoot)
	r.NoError(err)
	r.Equal(int64(2), atomic.LoadInt64(requests))

	o.refreshOnce(context.Background())

	_, cached := o.load(discoveryKey{issuerURI: issuer, tlsDigest: tlsDigest(withCA)})
	r.True(cached, "the entry the first extension reads must survive")
	_, cached = o.load(discoveryKey{issuerURI: issuer, tlsDigest: tlsDigest(withExtraRoot)})
	r.True(cached, "the entry the second extension reads must survive")

	// Both are still served from cache, so neither extension re-fetches on translation.
	_, err = o.get(context.Background(), testExtName, issuer, withCA)
	r.NoError(err)
	_, err = o.get(context.Background(), otherExtName, issuer, withExtraRoot)
	r.NoError(err)
	r.Equal(int64(2), atomic.LoadInt64(requests), "neither entry should have been re-discovered")
}

// TestRefreshKeepsBindingsMadeDuringLiveSnapshot covers the race between the refresh loop and
// a transform for a GatewayExtension that has just been created. The live set is resolved off
// the lock, so the transform can bind the new extension while the snapshot is being taken and
// the snapshot may not include it. Releasing that binding would prune the entry on the next
// pass without ever firing the trigger, leaving a discovery failure latched with nothing left
// to retry it: exactly what the refresh loop exists to prevent.
func TestRefreshKeepsBindingsMadeDuringLiveSnapshot(t *testing.T) {
	r := require.New(t)

	server, caPEM, _ := newTLSDiscoveryServer(t)
	issuer := server.URL
	withCA := &ir.UpstreamTLSValidation{CAPEM: caPEM}
	key := discoveryKey{issuerURI: issuer, tlsDigest: tlsDigest(withCA)}

	lateSource := ir.ObjectSource{Namespace: "ns", Name: "late-ext"}
	lateExtName := lateSource.ResourceName()

	// The live set stands in for a krt List that has not caught up with lateExt yet. While it
	// is being resolved, lateExt's transform runs and binds. The snapshot still omits it.
	var o *oidcProviderConfigDiscoverer
	bindDuringSnapshot := true
	o = newOIDCProviderConfigDiscoverer(func() []ir.GatewayExtension {
		if bindDuringSnapshot {
			_, err := o.get(context.Background(), lateExtName, issuer, withCA)
			r.NoError(err)
		}
		return nil
	})
	o.cacheRefreshInterval = time.Hour
	o.failureRetryInterval = time.Hour

	o.refreshOnce(context.Background())

	o.mu.RLock()
	_, bound := o.bindings[lateExtName]
	o.mu.RUnlock()
	r.True(bound, "a binding made during the live snapshot must survive the pass that took it")
	_, cached := o.load(key)
	r.True(cached, "the entry it names must survive with it")

	// On the next pass the binding predates the snapshot, so the live set is authoritative:
	// an extension that genuinely does not exist is released and its entry pruned.
	bindDuringSnapshot = false
	o.refreshOnce(context.Background())

	o.mu.RLock()
	_, bound = o.bindings[lateExtName]
	o.mu.RUnlock()
	r.False(bound, "a binding the live set has had a chance to see is released when not live")
	_, cached = o.load(key)
	r.False(cached, "and the entry nothing reads any more is pruned")
}

// TestUnbindReleasesEntry covers the translation that relies on discovery but gives up before
// reaching get(): the backend does not resolve, or its BackendTLSPolicy is invalid. The
// extension is still live, so without an explicit release the refresh loop would keep the
// entry from the previous translation bound and re-discover it forever, under a CA or for an
// issuer that nothing reads any more.
func TestUnbindReleasesEntry(t *testing.T) {
	r := require.New(t)

	server, caPEM, requests := newTLSDiscoveryServer(t)
	issuer := server.URL
	o := newTestDiscoverer(issuer)
	o.cacheRefreshInterval = time.Hour
	o.failureRetryInterval = time.Hour

	withCA := &ir.UpstreamTLSValidation{CAPEM: caPEM}
	key := discoveryKey{issuerURI: issuer, tlsDigest: tlsDigest(withCA)}

	_, err := o.get(context.Background(), testExtName, issuer, withCA)
	r.NoError(err)
	r.Equal(int64(1), atomic.LoadInt64(requests))

	// Without a release, a live extension's entry is kept across passes.
	o.refreshOnce(context.Background())
	_, cached := o.load(key)
	r.True(cached, "a bound entry survives the pass")

	// The extension's next translation fails before get(), and says so.
	o.unbind(testExtName)
	o.refreshOnce(context.Background())

	_, cached = o.load(key)
	r.False(cached, "an entry its only reader released must be pruned")
	o.clients.mu.Lock()
	_, kept := o.clients.clients[key.tlsDigest]
	o.clients.mu.Unlock()
	r.False(kept, "and its http client with it")
	r.Equal(int64(1), atomic.LoadInt64(requests), "a released entry is not re-discovered")
}

// TestRefreshRetainsClientForBoundButUncachedKey covers the client memo's invariant against a
// foreground get() in flight: get() binds its key before it discovers, and builds the client
// before it inserts into the cache, so at the moment a refresh pass prunes, a key can be bound
// with a client and no cache entry. Retention has to be computed from the bound keys, not the
// cached ones, or that client is evicted underneath the fetch using it.
func TestRefreshRetainsClientForBoundButUncachedKey(t *testing.T) {
	r := require.New(t)

	server, caPEM, _ := newTLSDiscoveryServer(t)
	issuer := server.URL

	otherSource := ir.ObjectSource{Namespace: "ns", Name: "other-ext"}
	otherExtName := otherSource.ResourceName()
	exts := []ir.GatewayExtension{
		testExtension(issuer),
		{ObjectSource: otherSource, OAuth2: testExtension(issuer).OAuth2},
	}
	o := newOIDCProviderConfigDiscoverer(func() []ir.GatewayExtension { return exts })
	o.cacheRefreshInterval = time.Hour
	o.failureRetryInterval = time.Hour

	oldCA := &ir.UpstreamTLSValidation{CAPEM: caPEM + unrelatedCAPEM(t)}
	newCA := &ir.UpstreamTLSValidation{CAPEM: caPEM}
	inFlightCA := &ir.UpstreamTLSValidation{CAPEM: caPEM + unrelatedCAPEM(t)}
	oldDigest, inFlightDigest := tlsDigest(oldCA), tlsDigest(inFlightCA)
	r.NotEqual(oldDigest, inFlightDigest)

	// The first extension rotates its CA, leaving the old entry with no reader to be pruned.
	_, err := o.get(context.Background(), testExtName, issuer, oldCA)
	r.NoError(err)
	_, err = o.get(context.Background(), testExtName, issuer, newCA)
	r.NoError(err)

	// The second extension is mid-fetch: bound, client built, nothing cached yet.
	o.bind(otherExtName, discoveryKey{issuerURI: issuer, tlsDigest: inFlightDigest})
	inFlightClient, err := o.clients.get(inFlightDigest, inFlightCA)
	r.NoError(err)

	o.refreshOnce(context.Background())

	o.clients.mu.Lock()
	_, oldKept := o.clients.clients[oldDigest]
	kept, inFlightKept := o.clients.clients[inFlightDigest]
	o.clients.mu.Unlock()
	r.False(oldKept, "the pruned entry's client is released")
	r.True(inFlightKept, "the in-flight key's client must survive the prune")
	r.Same(inFlightClient, kept, "and be the very client the fetch is using")
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
