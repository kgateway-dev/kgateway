package trafficpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	kgwv1a1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// newTestDiscoverer returns a discoverer whose refresh loop considers the given issuer URIs
// live, with short intervals so refresh behavior is observable in tests.
func newTestDiscoverer(issuerURIs ...string) *oidcProviderConfigDiscoverer {
	o := newOIDCProviderConfigDiscoverer(func() []string { return issuerURIs })
	o.cacheRefreshInterval = 50 * time.Millisecond
	o.failureRetryInterval = 20 * time.Millisecond
	return o
}

func TestOIDCConfigDiscovery(t *testing.T) {
	tests := []struct {
		name           string
		setupServer    func() *httptest.Server
		expectedConfig *oidcProviderConfig
		expectError    bool
		errorContains  string
	}{
		{
			name: "successful discovery",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					require.Equal(t, "/.well-known/openid-configuration", r.URL.Path)
					require.Equal(t, "application/json", r.Header.Get("Accept"))
					require.Equal(t, "kgateway/oidc-discovery", r.Header.Get("User-Agent"))

					config := oidcProviderConfig{
						TokenEndpoint:         "https://example.com/token",
						AuthorizationEndpoint: "https://example.com/auth",
						EndSessionEndpoint:    new("https://example.com/logout"),
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(config)
				}))
			},
			expectedConfig: &oidcProviderConfig{
				TokenEndpoint:         "https://example.com/token",
				AuthorizationEndpoint: "https://example.com/auth",
				EndSessionEndpoint:    new("https://example.com/logout"),
			},
			expectError: false,
		},
		{
			name: "successful discovery without end session endpoint",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					config := oidcProviderConfig{
						TokenEndpoint:         "https://example.com/token",
						AuthorizationEndpoint: "https://example.com/auth",
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(config)
				}))
			},
			expectedConfig: &oidcProviderConfig{
				TokenEndpoint:         "https://example.com/token",
				AuthorizationEndpoint: "https://example.com/auth",
			},
			expectError: false,
		},
		{
			name: "server returns 404",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			expectError:   true,
			errorContains: "unexpected status code 404",
		},
		{
			name: "server returns 500",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			expectError:   true,
			errorContains: "unexpected status code 500",
		},
		{
			name: "invalid JSON response",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte("invalid json"))
				}))
			},
			expectError:   true,
			errorContains: "error decoding OpenID provider config",
		},
		{
			name: "empty response",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte("{}"))
				}))
			},
			expectedConfig: &oidcProviderConfig{},
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := require.New(t)

			// Setup test server
			server := tt.setupServer()
			defer server.Close()

			// Parse server URL to get the issuer
			issuerURL, err := url.Parse(server.URL)
			r.NoError(err)
			issuer := issuerURL.String()

			// Create new OIDC config discovery instance for each test
			o := newTestDiscoverer(issuer)

			// Test the discovery
			config, err := o.get(context.Background(), issuer)

			if tt.expectError {
				r.Error(err)
				if tt.errorContains != "" {
					r.Contains(err.Error(), tt.errorContains)
				}
				r.Nil(config)
				return
			}

			// validate response
			r.NoError(err)
			r.NotNil(config)
			r.Equal(tt.expectedConfig.TokenEndpoint, config.TokenEndpoint)
			r.Equal(tt.expectedConfig.AuthorizationEndpoint, config.AuthorizationEndpoint)
			if tt.expectedConfig.EndSessionEndpoint != nil {
				r.NotNil(config.EndSessionEndpoint)
				r.Equal(*tt.expectedConfig.EndSessionEndpoint, *config.EndSessionEndpoint)
			} else {
				r.Nil(config.EndSessionEndpoint)
			}
		})
	}
}

func TestOIDCConfigDiscoveryCache(t *testing.T) {
	r := require.New(t)

	// Track number of requests
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requestCount++
		config := oidcProviderConfig{
			TokenEndpoint:         "https://example.com/token",
			AuthorizationEndpoint: "https://example.com/auth",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	issuer := server.URL
	o := newTestDiscoverer(issuer)

	// First call should make HTTP request
	config1, err := o.get(context.Background(), issuer)
	r.NoError(err)
	r.NotNil(config1)
	r.Equal(1, requestCount)

	// Second call should use cache
	config2, err := o.get(context.Background(), issuer)
	r.NoError(err)
	r.NotNil(config2)
	r.Equal(1, requestCount) // Should still be 1, no new request

	// Verify configs are the same
	r.Equal(config1.TokenEndpoint, config2.TokenEndpoint)
	r.Equal(config1.AuthorizationEndpoint, config2.AuthorizationEndpoint)
}

// TestOIDCConfigDiscoveryFailureIsCached asserts that a discovery failure is served from the
// cache, so a GatewayExtension re-translated for an unrelated reason does not re-block the krt
// event loop contacting a provider already known to be unreachable.
func TestOIDCConfigDiscoveryFailureIsCached(t *testing.T) {
	r := require.New(t)

	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	issuer := server.URL
	o := newTestDiscoverer(issuer)

	cfg, err := o.get(context.Background(), issuer)
	r.Error(err)
	r.Nil(cfg)
	// 404 is unrecoverable, so exactly one request is made.
	r.Equal(int64(1), atomic.LoadInt64(&requestCount))

	cfg, err = o.get(context.Background(), issuer)
	r.Error(err)
	r.Nil(cfg)
	r.Equal(int64(1), atomic.LoadInt64(&requestCount), "failed discovery should be served from cache")
}

func TestOIDCConfigDiscoveryConcurrentDedup(t *testing.T) {
	r := require.New(t)

	// Track the number of HTTP requests reaching the server.
	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		// Simulate a slow upstream so concurrent callers overlap.
		time.Sleep(50 * time.Millisecond)
		config := oidcProviderConfig{
			TokenEndpoint:         "https://example.com/token",
			AuthorizationEndpoint: "https://example.com/auth",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	issuer := server.URL
	o := newTestDiscoverer(issuer)

	// Launch many concurrent get() calls for the same issuer.
	const goroutines = 10
	errs := make(chan error, goroutines)
	configs := make(chan *oidcProviderConfig, goroutines)
	for range goroutines {
		go func() {
			cfg, err := o.get(context.Background(), issuer)
			errs <- err
			configs <- cfg
		}()
	}

	// All goroutines should succeed.
	for range goroutines {
		r.NoError(<-errs)
		cfg := <-configs
		r.NotNil(cfg)
		r.Equal("https://example.com/token", cfg.TokenEndpoint)
	}

	// Singleflight should have deduplicated all concurrent calls into exactly one HTTP request.
	r.Equal(int64(1), atomic.LoadInt64(&requestCount),
		"expected exactly 1 HTTP request, but singleflight did not deduplicate concurrent calls")
}

func TestOIDCConfigDiscoveryInvalidIssuerURL(t *testing.T) {
	r := require.New(t)

	// Test with invalid URL that would cause url.Parse to fail
	invalidIssuer := "://invalid-url"
	o := newTestDiscoverer(invalidIssuer)

	config, err := o.get(context.Background(), invalidIssuer)
	r.Error(err)
	r.Nil(config)
	r.Contains(err.Error(), "error parsing discovery URL")

	// A malformed issuer URI is not cached: it can only be fixed by editing the
	// GatewayExtension, which re-runs the transform on its own, so there is nothing for the
	// refresh loop to retry and it must not be polled (or warned about) every pass forever.
	_, cached := o.load(invalidIssuer)
	r.False(cached, "malformed issuer URI should not be cached")

	o.refreshOnce(context.Background())
	_, cached = o.load(invalidIssuer)
	r.False(cached, "refresh should not resurrect a malformed issuer URI")
}

// TestOIDCConfigDiscoveryFailureBacksOff asserts consecutive failures are retried with an
// exponential backoff capped at cacheRefreshInterval, so a multi-hour provider outage is not
// polled at the base interval for its whole duration.
func TestOIDCConfigDiscoveryFailureBacksOff(t *testing.T) {
	r := require.New(t)

	o := newOIDCProviderConfigDiscoverer(func() []string { return nil })
	o.failureRetryInterval = time.Second
	o.cacheRefreshInterval = 4 * time.Second
	discoveryErr := errors.New("boom")

	// failures=1 -> 1s, 2 -> 2s, 3 -> 4s, then capped at cacheRefreshInterval.
	for _, tc := range []struct {
		priorFailures int
		wantTTL       time.Duration
	}{
		{priorFailures: 0, wantTTL: time.Second},
		{priorFailures: 1, wantTTL: 2 * time.Second},
		{priorFailures: 2, wantTTL: 4 * time.Second},
		{priorFailures: 3, wantTTL: 4 * time.Second},
		{priorFailures: 40, wantTTL: 4 * time.Second}, // no overflow at high failure counts
	} {
		before := time.Now()
		result := o.newResult(nil, discoveryErr, tc.priorFailures)
		r.Equal(tc.priorFailures+1, result.failures)
		ttl := result.expiry.Sub(before)
		r.GreaterOrEqual(ttl, tc.wantTTL, "ttl for %d prior failures", tc.priorFailures)
		r.Less(ttl, tc.wantTTL+time.Second, "ttl for %d prior failures", tc.priorFailures)
	}

	// A success resets to the full refresh interval.
	success := o.newResult(&oidcProviderConfig{}, nil, 9)
	r.Equal(0, success.failures)
	r.Greater(time.Until(success.expiry), 3*time.Second)
}

// TestOIDCConfigDiscoveryShutdownDoesNotClobberCache asserts a refresh pass interrupted by
// shutdown leaves cached configs alone. Overwriting a healthy entry with "context canceled"
// would report a change, trigger a recomputation, and set Err on every OAuth2 extension on the
// way out.
func TestOIDCConfigDiscoveryShutdownDoesNotClobberCache(t *testing.T) {
	r := require.New(t)

	// Block the handler until the test releases it, so cancellation can land mid-flight.
	release := make(chan struct{})
	var serving atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if serving.Load() {
			serving.Store(false)
			<-release
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(oidcProviderConfig{
			TokenEndpoint:         "https://example.com/token",
			AuthorizationEndpoint: "https://example.com/auth",
		})
	}))
	defer server.Close()
	defer close(release)

	issuer := server.URL
	o := newTestDiscoverer(issuer)

	// Populate a healthy entry.
	cfg, err := o.get(context.Background(), issuer)
	r.NoError(err)
	r.NotNil(cfg)

	assertCacheIntact := func(when string) {
		got, ok := o.load(issuer)
		r.True(ok, "entry should still be cached %s", when)
		r.NoError(got.err, "cached config should survive %s", when)
		r.NotNil(got.cfg, "cached config should survive %s", when)
		r.Equal("https://example.com/token", got.cfg.TokenEndpoint)
	}

	// Cancelled before the pass starts.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.False(o.rediscover(ctx, issuer), "a cancelled pass should not report a change")
	assertCacheIntact("cancellation before the pass")

	// Cancelled while the request is in flight.
	serving.Store(true)
	inflightCtx, cancelInflight := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() { done <- o.rediscover(inflightCtx, issuer) }()
	require.Eventually(t, func() bool { return !serving.Load() }, 5*time.Second, 10*time.Millisecond,
		"handler should have been entered")
	cancelInflight()

	select {
	case changed := <-done:
		r.False(changed, "a pass cancelled in flight should not report a change")
	case <-time.After(5 * time.Second):
		t.Fatal("rediscover did not return after cancellation")
	}
	assertCacheIntact("cancellation in flight")
}

// TestOIDCDiscovererRunClampsNonPositiveInterval guards against time.NewTicker panicking when a
// discoverer is built with a zero interval, as the pruning test does.
func TestOIDCDiscovererRunClampsNonPositiveInterval(t *testing.T) {
	o := newOIDCProviderConfigDiscoverer(func() []string { return nil })
	o.failureRetryInterval = 0
	o.cacheRefreshInterval = 0

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		o.run(ctx) // must not panic
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return after context cancellation")
	}
}

// TestOIDCConfigDiscoveryRetriesFailureAndRecovers covers the regression in
// https://github.com/kgateway-dev/kgateway/issues/14497: an issuer that is unreachable when
// the config is first discovered must be re-discovered by the refresh loop, and the recovery
// must trigger a krt recomputation so the GatewayExtension stops reporting the error.
func TestOIDCConfigDiscoveryRetriesFailureAndRecovers(t *testing.T) {
	r := require.New(t)

	// Serve the 521 from the issue report until the test flips the switch, mirroring a
	// provider that is still starting up while the control plane translates.
	var healthy atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !healthy.Load() {
			w.WriteHeader(521)
			return
		}
		config := oidcProviderConfig{
			TokenEndpoint:         "https://example.com/token",
			AuthorizationEndpoint: "https://example.com/auth",
			JWKSURI:               "https://example.com/keys",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	issuer := server.URL
	o := newTestDiscoverer(issuer)

	// Initial translation fails, exactly as reported in the issue.
	cfg, err := o.get(context.Background(), issuer)
	r.Error(err)
	r.Nil(cfg)
	r.Contains(err.Error(), "unexpected status code 521")

	// While the provider is still down, refreshing must not report a change: the error is
	// identical, so krt should not be churned.
	r.False(o.rediscover(context.Background(), issuer), "unchanged failure should not trigger recomputation")

	// The provider comes back.
	healthy.Store(true)

	// The refresh loop re-discovers and reports the changed outcome.
	r.True(o.rediscover(context.Background(), issuer), "recovery should trigger recomputation")

	// A subsequent translation now sees the discovered config instead of the latched error.
	cfg, err = o.get(context.Background(), issuer)
	r.NoError(err)
	r.NotNil(cfg)
	r.Equal("https://example.com/token", cfg.TokenEndpoint)
	r.Equal("https://example.com/keys", cfg.JWKSURI)
}

// TestProviderBlipKeepsCachedConfig is the mirror image of
// TestOIDCConfigDiscoveryRetriesFailureAndRecovers: the provider starts healthy and then blips.
// A refresh failure must not withdraw a configuration that was discovered successfully -- it
// should keep serving the last known good config and just retry.
func TestProviderBlipKeepsCachedConfig(t *testing.T) {
	r := require.New(t)

	var healthy atomic.Bool
	healthy.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(oidcProviderConfig{
			TokenEndpoint:         "https://example.com/token",
			AuthorizationEndpoint: "https://example.com/auth",
		})
	}))
	defer server.Close()

	issuer := server.URL
	o := newTestDiscoverer(issuer)

	// Steady state: discovery succeeded, translation is serving a real config.
	cfg, err := o.get(context.Background(), issuer)
	r.NoError(err)
	r.Equal("https://example.com/token", cfg.TokenEndpoint)

	// The provider blips and a refresh pass lands in the window.
	healthy.Store(false)
	r.False(o.rediscover(context.Background(), issuer), "a transient refresh failure should not trigger a recomputation")

	// What the next translation sees.
	cfg, err = o.get(context.Background(), issuer)
	r.NoError(err, "the last known good config should still be served during a blip")
	r.NotNil(cfg, "the last known good config should still be served during a blip")
	r.Equal("https://example.com/token", cfg.TokenEndpoint)
}

// TestProviderBlipBacksOffAndPicksUpChanges guards the two things retaining a config through a
// failure must not cost us: the retry must still back off while the provider is down, and the
// entry must not be latched -- once the provider returns with a *different* document, the change
// has to propagate.
func TestProviderBlipBacksOffAndPicksUpChanges(t *testing.T) {
	r := require.New(t)

	var token atomic.Value
	token.Store("https://example.com/token")
	var healthy atomic.Bool
	healthy.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !healthy.Load() {
			w.WriteHeader(http.StatusNotFound) // unrecoverable, so the blip resolves fast
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(oidcProviderConfig{
			TokenEndpoint:         token.Load().(string),
			AuthorizationEndpoint: "https://example.com/auth",
		})
	}))
	defer server.Close()

	issuer := server.URL
	o := newTestDiscoverer(issuer)
	o.failureRetryInterval = time.Second
	o.cacheRefreshInterval = 4 * time.Second

	_, err := o.get(context.Background(), issuer)
	r.NoError(err)

	// Two failed refresh passes while stale-serving: the retry backs off 1s -> 2s rather than
	// hammering the provider for the length of the outage.
	healthy.Store(false)
	for _, want := range []time.Duration{time.Second, 2 * time.Second} {
		before := time.Now()
		r.False(o.rediscover(context.Background(), issuer))

		result, ok := o.load(issuer)
		r.True(ok)
		r.NotNil(result.cfg, "the last known good config must be retained across every failure")
		r.NoError(result.err)
		ttl := result.expiry.Sub(before)
		r.GreaterOrEqual(ttl, want)
		r.Less(ttl, want+time.Second)
	}

	// The provider comes back, with a different token endpoint than we cached.
	token.Store("https://example.com/token/v2")
	healthy.Store(true)

	r.True(o.rediscover(context.Background(), issuer), "a changed config must trigger a recomputation")
	cfg, err := o.get(context.Background(), issuer)
	r.NoError(err)
	r.Equal("https://example.com/token/v2", cfg.TokenEndpoint)

	result, ok := o.load(issuer)
	r.True(ok)
	r.Equal(0, result.failures, "a successful refresh resets the backoff")
}

// TestOIDCConfigDiscoveryRefreshLoopRecovers exercises the same recovery through the running
// refresh loop rather than by calling rediscover() directly.
func TestOIDCConfigDiscoveryRefreshLoopRecovers(t *testing.T) {
	r := require.New(t)

	var healthy atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !healthy.Load() {
			w.WriteHeader(http.StatusNotFound) // unrecoverable, so no retry backoff
			return
		}
		config := oidcProviderConfig{
			TokenEndpoint:         "https://example.com/token",
			AuthorizationEndpoint: "https://example.com/auth",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	issuer := server.URL
	o := newTestDiscoverer(issuer)

	_, err := o.get(context.Background(), issuer)
	r.Error(err)

	ctx := t.Context()
	go o.run(ctx)

	healthy.Store(true)

	require.Eventually(t, func() bool {
		cfg, err := o.get(context.Background(), issuer)
		return err == nil && cfg != nil && cfg.TokenEndpoint == "https://example.com/token"
	}, 5*time.Second, 10*time.Millisecond, "refresh loop should re-discover the recovered provider")
}

// TestOIDCConfigDiscoveryPrunesDeletedIssuers asserts the refresh loop stops polling an issuer
// once no GatewayExtension references it, so a deleted or re-pointed extension does not leave
// kgateway contacting a dead endpoint forever.
func TestOIDCConfigDiscoveryPrunesDeletedIssuers(t *testing.T) {
	r := require.New(t)

	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		config := oidcProviderConfig{
			TokenEndpoint:         "https://example.com/token",
			AuthorizationEndpoint: "https://example.com/auth",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	issuer := server.URL

	var live atomic.Bool
	live.Store(true)
	o := newOIDCProviderConfigDiscoverer(func() []string {
		if !live.Load() {
			return nil
		}
		return []string{issuer}
	})
	// Expire immediately so every refresh pass re-discovers a live issuer.
	o.cacheRefreshInterval = 0
	o.failureRetryInterval = 0

	_, err := o.get(context.Background(), issuer)
	r.NoError(err)
	r.Equal(int64(1), atomic.LoadInt64(&requestCount))

	// While referenced, a refresh pass re-discovers.
	o.refreshOnce(context.Background())
	r.Equal(int64(2), atomic.LoadInt64(&requestCount))
	_, cached := o.load(issuer)
	r.True(cached)

	// Once the GatewayExtension is gone, the entry is pruned and no longer polled.
	live.Store(false)
	o.refreshOnce(context.Background())
	_, cached = o.load(issuer)
	r.False(cached, "issuer with no referencing GatewayExtension should be pruned")

	countAfterPrune := atomic.LoadInt64(&requestCount)
	o.refreshOnce(context.Background())
	r.Equal(countAfterPrune, atomic.LoadInt64(&requestCount), "pruned issuer should not be polled")
}

// TestOIDCDiscoveryRequired pins the predicate that defines "this extension will call
// discoverer.get()". buildOAuth2ProviderConfig and the refresh loop's live set both use it, so a
// change here silently changes which issuers are polled: widening it polls issuers nobody reads,
// narrowing it prunes entries that are still needed and re-latches discovery failures.
func TestOIDCDiscoveryRequired(t *testing.T) {
	const uri = kgwv1a1.HttpsUri("https://idp.example.com/x")
	issuer := "https://idp.example.com"

	// allExplicit is the only shape that does not need discovery while still naming an issuer.
	allExplicit := func() *kgwv1a1.OAuth2Provider {
		return &kgwv1a1.OAuth2Provider{
			IssuerURI:             &issuer,
			TokenEndpoint:         ptr.To(uri),
			AuthorizationEndpoint: ptr.To(uri),
			EndSessionEndpoint:    ptr.To(uri),
		}
	}

	tests := []struct {
		name string
		in   *kgwv1a1.OAuth2Provider
		want bool
	}{
		{name: "nil provider", in: nil, want: false},
		{name: "no issuer at all", in: &kgwv1a1.OAuth2Provider{}, want: false},
		{
			name: "no issuer, endpoints explicit",
			in:   &kgwv1a1.OAuth2Provider{TokenEndpoint: ptr.To(uri), AuthorizationEndpoint: ptr.To(uri)},
			want: false,
		},
		{name: "issuer only", in: &kgwv1a1.OAuth2Provider{IssuerURI: &issuer}, want: true},
		{name: "issuer and every endpoint explicit", in: allExplicit(), want: false},
		{
			name: "issuer, token endpoint missing",
			in: func() *kgwv1a1.OAuth2Provider {
				p := allExplicit()
				p.TokenEndpoint = nil
				return p
			}(),
			want: true,
		},
		{
			name: "issuer, authorization endpoint missing",
			in: func() *kgwv1a1.OAuth2Provider {
				p := allExplicit()
				p.AuthorizationEndpoint = nil
				return p
			}(),
			want: true,
		},
		{
			name: "issuer, end session endpoint missing",
			in: func() *kgwv1a1.OAuth2Provider {
				p := allExplicit()
				p.EndSessionEndpoint = nil
				return p
			}(),
			want: true,
		},
		{
			name: "issuer, endpoints explicit, JWT without jwksURI",
			in: func() *kgwv1a1.OAuth2Provider {
				p := allExplicit()
				p.JWT = &kgwv1a1.OAuth2JWTConfig{}
				return p
			}(),
			want: true,
		},
		{
			name: "issuer, endpoints explicit, JWT with jwksURI",
			in: func() *kgwv1a1.OAuth2Provider {
				p := allExplicit()
				p.JWT = &kgwv1a1.OAuth2JWTConfig{JWKSURI: ptr.To(uri)}
				return p
			}(),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, oidcDiscoveryRequired(tt.in))

			// oidcIssuerURIs must agree: it lists exactly the issuers that will be discovered.
			got := oidcIssuerURIs([]ir.GatewayExtension{{OAuth2: tt.in}})
			if tt.want {
				require.Equal(t, []string{issuer}, got)
			} else {
				require.Empty(t, got)
			}
		})
	}
}

// TestOIDCIssuerURIsSharedIssuer asserts an issuer stays live while any one extension still
// discovers from it, even if another has every endpoint configured explicitly.
func TestOIDCIssuerURIsSharedIssuer(t *testing.T) {
	issuer := "https://idp.example.com"
	uri := kgwv1a1.HttpsUri("https://idp.example.com/x")

	needsDiscovery := &kgwv1a1.OAuth2Provider{IssuerURI: &issuer}
	fullyExplicit := &kgwv1a1.OAuth2Provider{
		IssuerURI:             &issuer,
		TokenEndpoint:         new(uri),
		AuthorizationEndpoint: new(uri),
		EndSessionEndpoint:    new(uri),
	}

	require.Equal(t, []string{issuer}, oidcIssuerURIs([]ir.GatewayExtension{
		{OAuth2: fullyExplicit}, {OAuth2: needsDiscovery},
	}), "issuer should stay live while any extension still discovers from it")
}

// TestOIDCConfigDiscoveryPrunesIssuerNoLongerDiscovered covers the transition the deletion-based
// prune test does not: the GatewayExtension still exists and still names the issuer, but every
// endpoint is now configured explicitly, so nothing reads the discovered config any more.
func TestOIDCConfigDiscoveryPrunesIssuerNoLongerDiscovered(t *testing.T) {
	r := require.New(t)

	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(oidcProviderConfig{
			TokenEndpoint:         "https://example.com/token",
			AuthorizationEndpoint: "https://example.com/auth",
		})
	}))
	defer server.Close()

	issuer := server.URL
	uri := kgwv1a1.HttpsUri("https://example.com/x")

	// The extension starts out relying on discovery, then is edited to spell out every endpoint
	// while keeping issuerURI.
	ext := &kgwv1a1.OAuth2Provider{IssuerURI: &issuer}
	o := newOIDCProviderConfigDiscoverer(func() []string {
		return oidcIssuerURIs([]ir.GatewayExtension{{OAuth2: ext}})
	})
	// Expire immediately so every pass re-discovers whatever is still live.
	o.cacheRefreshInterval = 0
	o.failureRetryInterval = 0

	_, err := o.get(context.Background(), issuer)
	r.NoError(err)
	r.Equal(int64(1), atomic.LoadInt64(&requestCount))

	// While discovery is still required, the entry is refreshed.
	o.refreshOnce(context.Background())
	r.Equal(int64(2), atomic.LoadInt64(&requestCount))
	_, cached := o.load(issuer)
	r.True(cached)

	// The user fills in every endpoint. buildOAuth2ProviderConfig will no longer call get() for
	// this extension, so the cached entry must be dropped rather than polled forever.
	ext.TokenEndpoint = new(uri)
	ext.AuthorizationEndpoint = new(uri)
	ext.EndSessionEndpoint = new(uri)

	o.refreshOnce(context.Background())
	_, cached = o.load(issuer)
	r.False(cached, "issuer should be pruned once no extension discovers from it")

	countAfterPrune := atomic.LoadInt64(&requestCount)
	o.refreshOnce(context.Background())
	r.Equal(countAfterPrune, atomic.LoadInt64(&requestCount), "pruned issuer should not be polled")
}
