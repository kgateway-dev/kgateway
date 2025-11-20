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
	"k8s.io/utils/ptr"
)

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
						EndSessionEndpoint:    ptr.To("https://example.com/logout"),
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(config)
				}))
			},
			expectedConfig: &oidcProviderConfig{
				TokenEndpoint:         "https://example.com/token",
				AuthorizationEndpoint: "https://example.com/auth",
				EndSessionEndpoint:    ptr.To("https://example.com/logout"),
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
			o := newOIDCProviderConfigDiscoverer()

			// Test the discovery
			config, err := o.get(issuer)

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

	o := newOIDCProviderConfigDiscoverer()
	issuer := server.URL

	// First call should make HTTP request
	config1, err := o.get(issuer)
	r.NoError(err)
	r.NotNil(config1)
	r.Equal(1, requestCount)

	// Second call should use cache
	config2, err := o.get(issuer)
	r.NoError(err)
	r.NotNil(config2)
	r.Equal(1, requestCount) // Should still be 1, no new request

	// Verify configs are the same
	r.Equal(config1.TokenEndpoint, config2.TokenEndpoint)
	r.Equal(config1.AuthorizationEndpoint, config2.AuthorizationEndpoint)
}

func TestOIDCConfigDiscoveryInvalidIssuerURL(t *testing.T) {
	r := require.New(t)

	o := newOIDCProviderConfigDiscoverer()

	// Test with invalid URL that would cause url.Parse to fail
	invalidIssuer := "://invalid-url"

	config, err := o.get(invalidIssuer)
	r.Error(err)
	r.Nil(config)
	r.Contains(err.Error(), "error parsing discovery URL")
}

func TestOIDCProviderConfigDiscovererRun(t *testing.T) {
	r := require.New(t)

	// Track the number of requests made to the server
	var requestCount int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt64(&requestCount, 1)

		config := oidcProviderConfig{
			TokenEndpoint:         "https://example.com/token",
			AuthorizationEndpoint: "https://example.com/auth",
			EndSessionEndpoint:    ptr.To("https://example.com/logout"),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	// Create discoverer with very short refresh interval for testing
	o := &oidcProviderConfigDiscoverer{
		cacheRefreshInterval: 50 * time.Millisecond,
	}

	issuer := server.URL

	// Initial request to populate cache
	config1, err := o.get(issuer)
	r.NoError(err)
	r.NotNil(config1)
	r.Equal("https://example.com/token", config1.TokenEndpoint)
	r.Equal(int64(1), atomic.LoadInt64(&requestCount)) // First request

	// Second get should use cache (no new request)
	config2, err := o.get(issuer)
	r.NoError(err)
	r.NotNil(config2)
	r.Equal("https://example.com/token", config2.TokenEndpoint)
	r.Equal(int64(1), atomic.LoadInt64(&requestCount)) // Still only 1 request (from cache)

	// Start the background cache clearing
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go o.refresh(ctx)

	// Wait for cache to be cleared (refresh interval is 50ms)
	time.Sleep(75 * time.Millisecond)

	// Now get should make a new request because cache was cleared
	config3, err := o.get(issuer)
	r.NoError(err)
	r.NotNil(config3)
	r.Equal("https://example.com/token", config3.TokenEndpoint)
	r.Equal(int64(2), atomic.LoadInt64(&requestCount)) // Second request after cache clear

	// Verify cache is working again (no new request)
	config4, err := o.get(issuer)
	r.NoError(err)
	r.NotNil(config4)
	r.Equal("https://example.com/token", config4.TokenEndpoint)
	r.Equal(int64(2), atomic.LoadInt64(&requestCount)) // Still 2 requests (from cache)

	// Wait for another cache clear cycle
	time.Sleep(75 * time.Millisecond)

	// Another get should make a third request because cache was cleared again
	config5, err := o.get(issuer)
	r.NoError(err)
	r.NotNil(config5)
	r.Equal("https://example.com/token", config5.TokenEndpoint)
	r.Equal(int64(3), atomic.LoadInt64(&requestCount)) // Third request after second cache clear

	// Cancel context and verify no more cache clearing
	cancel()
	requestCountBeforeCancel := atomic.LoadInt64(&requestCount)

	// Wait longer than the refresh interval
	time.Sleep(100 * time.Millisecond)

	// Get should still use cache since run() stopped
	config6, err := o.get(issuer)
	r.NoError(err)
	r.NotNil(config6)
	r.Equal("https://example.com/token", config6.TokenEndpoint)
	r.Equal(requestCountBeforeCancel, atomic.LoadInt64(&requestCount)) // No new requests after cancel
}
