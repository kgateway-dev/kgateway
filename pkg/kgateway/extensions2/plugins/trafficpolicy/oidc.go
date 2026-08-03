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
	"golang.org/x/sync/singleflight"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
)

const (
	wellKnownOpenIDConfPath = "/.well-known/openid-configuration"
	userAgent               = "kgateway/oidc-discovery"
	oidcAcceptedContentType = "application/json"

	// defaultOIDCCacheRefreshInterval is how long a successful discovery is served from the
	// cache before it is re-discovered. The OpenID provider configuration is not expected to
	// change frequently, so caching it for a longer duration prevents excessive network calls.
	defaultOIDCCacheRefreshInterval = 5 * time.Minute

	// defaultOIDCFailureRetryInterval is how long a failed discovery is served from the cache
	// before it is retried. It is much shorter than the success interval so that a provider
	// which was unreachable when kgateway started, or which was down during a provider
	// outage, is picked up quickly rather than leaving the policy broken until restart.
	defaultOIDCFailureRetryInterval = 30 * time.Second

	// oidcDiscoveryHTTPTimeout bounds a single discovery request.
	oidcDiscoveryHTTPTimeout = 30 * time.Second
)

// oidcProviderConfig maps the OpenID provider config response.
// Refer to https://openid.net/specs/openid-connect-discovery-1_0.html#ProviderConfigurationResponse for more details.
type oidcProviderConfig struct {
	TokenEndpoint         string  `json:"token_endpoint"`
	AuthorizationEndpoint string  `json:"authorization_endpoint"`
	EndSessionEndpoint    *string `json:"end_session_endpoint,omitempty"`
	JWKSURI               string  `json:"jwks_uri"`
}

// equals reports whether two provider configs would produce the same Envoy configuration.
// EndSessionEndpoint is compared by dereferenced value because an unset and an empty
// end_session_endpoint are treated identically by buildOAuth2ProviderConfig.
func (c *oidcProviderConfig) equals(other *oidcProviderConfig) bool {
	if c == nil || other == nil {
		return c == nil && other == nil
	}
	return c.TokenEndpoint == other.TokenEndpoint &&
		c.AuthorizationEndpoint == other.AuthorizationEndpoint &&
		ptr.Deref(c.EndSessionEndpoint, "") == ptr.Deref(other.EndSessionEndpoint, "") &&
		c.JWKSURI == other.JWKSURI
}

// oidcDiscoveryResult is a cached discovery outcome. Failures are cached alongside successes
// so that a GatewayExtension re-translated for an unrelated reason does not block the krt
// event loop re-contacting a provider that is already known to be unreachable.
type oidcDiscoveryResult struct {
	cfg *oidcProviderConfig
	err error
	// expiry is when the refresh loop becomes eligible to re-discover this entry.
	expiry time.Time
}

// sameOutcome reports whether two results translate to the same GatewayExtension IR.
// Errors are compared by message, matching how TrafficPolicyGatewayExtensionIR compares its
// Err field, so a retry that keeps failing the same way does not churn krt.
func (r oidcDiscoveryResult) sameOutcome(other oidcDiscoveryResult) bool {
	if (r.err == nil) != (other.err == nil) {
		return false
	}
	if r.err != nil {
		return r.err.Error() == other.err.Error()
	}
	return r.cfg.equals(other.cfg)
}

type oidcProviderConfigDiscoverer struct {
	// trigger re-runs the GatewayExtension transform when a cached discovery result changes.
	// The OpenID provider is not a Kubernetes resource, so krt has no dependency of its own
	// to track here: without this trigger a discovery failure stays latched in the
	// GatewayExtension IR (and the TrafficPolicies referencing it stay rejected) until the
	// control plane is restarted.
	trigger *krt.RecomputeTrigger

	// liveIssuerURIs returns the issuer URIs currently referenced by GatewayExtensions. The
	// refresh loop intersects this with the cache so it stops polling issuers whose
	// GatewayExtension was deleted or re-pointed at a different provider.
	liveIssuerURIs func() []string

	cacheRefreshInterval time.Duration
	failureRetryInterval time.Duration

	// mu guards cache. The cache is authoritative for get(): expiry is acted on only by the
	// refresh loop, so a translation never blocks on discovery for an issuer already known.
	mu    sync.RWMutex
	cache map[string]oidcDiscoveryResult

	// discoverGroup deduplicates concurrent discover() calls for the same issuer URI,
	// preventing redundant HTTP requests when several extensions share an issuer.
	discoverGroup singleflight.Group
}

// newOIDCProviderConfigDiscoverer returns an oidcProviderConfigDiscoverer that caches OpenID
// provider configurations. liveIssuerURIs must report the issuer URIs still referenced by
// GatewayExtensions so the refresh loop can prune entries it no longer needs to poll.
// Callers must start run() for cached entries to be refreshed and retried.
func newOIDCProviderConfigDiscoverer(liveIssuerURIs func() []string) *oidcProviderConfigDiscoverer {
	return &oidcProviderConfigDiscoverer{
		// Start synced: get() discovers synchronously on first use, so dependent collections
		// must not block waiting for the refresh loop to publish an initial state.
		trigger:              krt.NewRecomputeTrigger(true),
		liveIssuerURIs:       liveIssuerURIs,
		cacheRefreshInterval: defaultOIDCCacheRefreshInterval,
		failureRetryInterval: defaultOIDCFailureRetryInterval,
		cache:                map[string]oidcDiscoveryResult{},
	}
}

// markDependant registers the calling krt transform as depending on discovery results.
// It must be called before get(), including when get() goes on to fail, so that the transform
// is re-run once discovery starts succeeding.
func (o *oidcProviderConfigDiscoverer) markDependant(krtctx krt.HandlerContext) {
	o.trigger.MarkDependant(krtctx)
}

// run periodically re-discovers the provider configuration for every cached issuer that is
// still referenced by a GatewayExtension, and triggers a krt recomputation whenever an
// outcome changes. Successful entries are refreshed on cacheRefreshInterval; failed entries
// are retried on the much shorter failureRetryInterval.
func (o *oidcProviderConfigDiscoverer) run(ctx context.Context) {
	ticker := time.NewTicker(o.failureRetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Guard against the race where both ctx.Done() and ticker.C are
			// ready simultaneously and the scheduler picks ticker.C first.
			if ctx.Err() != nil {
				return
			}
			o.refreshOnce(ctx)
		}
	}
}

// refreshOnce prunes issuers no longer referenced by any GatewayExtension, re-discovers the
// expired ones, and triggers a single recomputation if any outcome changed.
func (o *oidcProviderConfigDiscoverer) refreshOnce(ctx context.Context) {
	live := sets.New(o.liveIssuerURIs()...)

	// Only refresh issuers that are both still referenced and already cached. Entries are
	// only ever added by get(), so this never discovers a config no GatewayExtension asked
	// for, e.g. an issuerURI whose endpoints are all explicitly configured.
	var pruned, expired []string
	now := time.Now()
	o.mu.RLock()
	for issuerURI, result := range o.cache {
		switch {
		case !live.Has(issuerURI):
			pruned = append(pruned, issuerURI)
		case now.After(result.expiry):
			expired = append(expired, issuerURI)
		}
	}
	o.mu.RUnlock()

	if len(pruned) > 0 {
		o.mu.Lock()
		for _, issuerURI := range pruned {
			delete(o.cache, issuerURI)
		}
		o.mu.Unlock()
	}

	changed := false
	for _, issuerURI := range expired {
		if ctx.Err() != nil {
			return
		}
		if o.rediscover(ctx, issuerURI) {
			changed = true
		}
	}

	if changed {
		logger.Debug("openid provider config changed, triggering recomputation")
		o.trigger.TriggerRecomputation()
	}
}

// rediscover re-runs discovery for issuerURI and replaces the cached entry, reporting whether
// the new outcome differs from the cached one.
func (o *oidcProviderConfigDiscoverer) rediscover(ctx context.Context, issuerURI string) bool {
	next := o.newResult(o.discover(ctx, issuerURI))
	if next.err != nil {
		logger.Warn("error refreshing OpenID provider config", "issuer_uri", issuerURI, "error", next.err)
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	prev, ok := o.cache[issuerURI]
	if !ok {
		// The entry was pruned while we were discovering, because its GatewayExtension went
		// away. Don't resurrect it.
		return false
	}
	o.cache[issuerURI] = next
	return !prev.sameOutcome(next)
}

// get returns the OpenID provider config for issuerURI, discovering it if it is not already
// cached. Both successes and failures are cached; run() owns re-discovering them and
// triggering a recomputation when the outcome changes, so callers must have registered with
// markDependant to observe that change.
func (o *oidcProviderConfigDiscoverer) get(ctx context.Context, issuerURI string) (*oidcProviderConfig, error) {
	if result, ok := o.load(issuerURI); ok {
		return result.cfg, result.err
	}

	// Use singleflight to deduplicate concurrent discovery calls for the same issuer;
	// several transforms may call get() for the same issuer at once.
	v, _, _ := o.discoverGroup.Do(issuerURI, func() (any, error) {
		// Re-check the cache inside the singleflight function, as another caller
		// may have populated it between our initial load and entering the group.
		if result, ok := o.load(issuerURI); ok {
			return result, nil
		}
		result := o.newResult(o.discover(ctx, issuerURI))
		o.mu.Lock()
		o.cache[issuerURI] = result
		o.mu.Unlock()
		return result, nil
	})
	// The discovery error is carried inside the result rather than returned from the
	// singleflight function, so that every waiter observes the same cached outcome.
	result := v.(oidcDiscoveryResult)
	return result.cfg, result.err
}

func (o *oidcProviderConfigDiscoverer) load(issuerURI string) (oidcDiscoveryResult, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	result, ok := o.cache[issuerURI]
	return result, ok
}

// newResult stamps a discovery outcome with the expiry after which run() may retry it.
func (o *oidcProviderConfigDiscoverer) newResult(cfg *oidcProviderConfig, err error) oidcDiscoveryResult {
	ttl := o.cacheRefreshInterval
	if err != nil {
		ttl = o.failureRetryInterval
	}
	return oidcDiscoveryResult{cfg: cfg, err: err, expiry: time.Now().Add(ttl)}
}

func (o *oidcProviderConfigDiscoverer) discover(ctx context.Context, issuerURI string) (*oidcProviderConfig, error) {
	discoveryURL, err := url.Parse(issuerURI + wellKnownOpenIDConfPath)
	if err != nil {
		return nil, fmt.Errorf("error parsing discovery URL: %w", err)
	}

	cfg := &oidcProviderConfig{}
	client := &http.Client{Timeout: oidcDiscoveryHTTPTimeout}
	err = retry.Do(func() error {
		// TODO: allow using custom certs for HTTPS Issuer URI
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL.String(), nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", oidcAcceptedContentType)
		req.Header.Set("User-Agent", userAgent)

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
	}, retry.Attempts(5), retry.Delay(100*time.Millisecond), retry.MaxDelay(5*time.Second), retry.DelayType(retry.BackOffDelay), retry.Context(ctx))
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
