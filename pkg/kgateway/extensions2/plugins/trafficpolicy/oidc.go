package trafficpolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
)

const (
	wellKnownOpenIDConfPath = "/.well-known/openid-configuration"
	userAgent               = "kgateway/oidc-discovery"
	oidcAcceptedContentType = "application/json"
)

type oidcProviderConfigDiscoverer struct {
	// caches oidcProviderConfig per issuer URI
	cache                sync.Map
	cacheRefreshInterval time.Duration
}

// oidcProviderConfig maps the OpenID provider config response.
// Refer to https://openid.net/specs/openid-connect-discovery-1_0.html#ProviderConfigurationResponse for more details.
type oidcProviderConfig struct {
	TokenEndpoint         string  `json:"token_endpoint"`
	AuthorizationEndpoint string  `json:"authorization_endpoint"`
	EndSessionEndpoint    *string `json:"end_session_endpoint,omitempty"`
}

// newOIDCProviderConfigDiscoverer returns a oidcProviderConfigDiscoverer instance that is responsible
// for periodically refreshing the OpenID provider configuration cache
func newOIDCProviderConfigDiscoverer() *oidcProviderConfigDiscoverer {
	return &oidcProviderConfigDiscoverer{
		cacheRefreshInterval: 5 * time.Minute,
	}
}

// refresh periodically clears the cache to allow re-discovery of OpenID provider configurations.
// The OpenID provider configuration is not expected to change frequently, so caching it for a longer duration
// is desirable to prevent excessive network calls. However, to accommodate potential changes in the provider configuration,
// the cache is cleared at regular intervals, prompting re-discovery on subsequent requests.
func (o *oidcProviderConfigDiscoverer) refresh(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(o.cacheRefreshInterval):
			// refresh the cache every 5 minutes; next get() will re-discover the config
			o.cache.Clear()
		}
	}
}

func (o *oidcProviderConfigDiscoverer) get(issuerURI string) (*oidcProviderConfig, error) {
	v, ok := o.cache.Load(issuerURI)
	if ok {
		return v.(*oidcProviderConfig), nil
	}

	// discover configuration
	cfg, err := o.discover(issuerURI)
	if err != nil {
		return nil, err
	}

	// Cache the configuration
	o.cache.Store(issuerURI, cfg)
	return cfg, nil
}

func (o *oidcProviderConfigDiscoverer) discover(issuerURI string) (*oidcProviderConfig, error) {
	discoveryURL, err := url.Parse(issuerURI + wellKnownOpenIDConfPath)
	if err != nil {
		return nil, fmt.Errorf("error parsing discovery URL: %w", err)
	}

	cfg := &oidcProviderConfig{}
	err = retry.Do(func() error {
		// TODO: allow using custom certs for HTTPS Issuer URI
		req, err := http.NewRequest(http.MethodGet, discoveryURL.String(), nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", oidcAcceptedContentType)
		req.Header.Set("User-Agent", userAgent)

		client := &http.Client{
			Timeout: 30 * time.Second,
		}

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to fetch OIDC configuration: %w", err)
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		// retry on specific 5xx status codes
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return fmt.Errorf("error discovering OpenID provider config; unexpected status code %d", resp.StatusCode)

		case http.StatusOK:
			if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
				return retry.Unrecoverable(fmt.Errorf("error decoding OpenID provider config: %w", err))
			}

		default:
			return retry.Unrecoverable(fmt.Errorf("error discovering OpenID provider config; unexpected status code %d", resp.StatusCode))
		}
		return nil
	}, retry.Attempts(5), retry.Delay(100*time.Millisecond), retry.MaxDelay(5*time.Second), retry.DelayType(retry.BackOffDelay))
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
