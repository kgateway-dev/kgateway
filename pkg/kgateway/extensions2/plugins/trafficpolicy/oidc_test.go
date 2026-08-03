package trafficpolicy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	o.failureRetryInterval = 10 * time.Millisecond

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
