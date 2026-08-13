package trafficpolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"

	kgwv1a1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	wellKnownOpenIDConfPath = "/.well-known/openid-configuration"
	userAgent               = "kgateway/oidc-discovery"
	oidcAcceptedContentType = "application/json"

	// defaultOIDCCacheRefreshInterval is how long a successful discovery is served from the
	// cache before it is re-discovered. The OpenID provider configuration is not expected to
	// change frequently, so caching it for a longer duration prevents excessive network calls.
	// It also caps the backoff applied to repeated failures.
	defaultOIDCCacheRefreshInterval = 5 * time.Minute

	// defaultOIDCFailureRetryInterval is the base interval after which a failed discovery is
	// retried. It is much shorter than the success interval so that a provider which was
	// unreachable when kgateway started, or which was down during a provider outage, is picked
	// up quickly rather than leaving the policy broken until restart. Consecutive failures back
	// off exponentially from here, capped at defaultOIDCCacheRefreshInterval.
	defaultOIDCFailureRetryInterval = 30 * time.Second

	// oidcDiscoveryHTTPTimeout bounds a single discovery request.
	oidcDiscoveryHTTPTimeout = 30 * time.Second

	// foregroundDiscoveryTimeout bounds discovery performed from a krt transform, which blocks
	// the event loop and therefore translation of everything downstream of GatewayExtensions.
	// It is deliberately tight: the background refresh loop retries, so giving up quickly costs
	// only a short-lived error on the policy rather than a lost configuration.
	foregroundDiscoveryTimeout = 10 * time.Second

	// backgroundDiscoveryTimeout bounds discovery performed by the refresh loop. It can afford
	// to be more generous than the foreground budget because it runs off the event loop.
	backgroundDiscoveryTimeout = 30 * time.Second

	// backgroundDiscoveryConcurrency bounds how many issuers the refresh loop re-discovers at
	// once, so recovery latency does not scale with the number of unreachable providers.
	backgroundDiscoveryConcurrency = 4
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
//
// err is only ever set for an issuer that has never been discovered successfully: once there is
// a config to serve, rediscover keeps serving it through failures rather than withdrawing it.
// So a non-zero failures with a non-nil cfg means "stale, still being retried".
type oidcDiscoveryResult struct {
	cfg *oidcProviderConfig
	err error
	// expiry is when the refresh loop becomes eligible to re-discover this entry.
	expiry time.Time
	// failures counts consecutive failed discoveries, and drives the retry backoff.
	failures int
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

	// liveIssuerURIs returns the issuer URIs some GatewayExtension currently discovers from,
	// per oidcDiscoveryRequired. The refresh loop intersects this with the cache so it stops
	// polling an issuer as soon as nothing reads its discovered config.
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
func newOIDCProviderConfigDiscoverer(liveIssuerURIs func() []string, opts ...krt.CollectionOption) *oidcProviderConfigDiscoverer {
	return &oidcProviderConfigDiscoverer{
		// Start synced: get() discovers synchronously on first use, so dependent collections
		// must not block waiting for the refresh loop to publish an initial state.
		trigger:              krt.NewRecomputeTrigger(true, opts...),
		liveIssuerURIs:       liveIssuerURIs,
		cacheRefreshInterval: defaultOIDCCacheRefreshInterval,
		failureRetryInterval: defaultOIDCFailureRetryInterval,
		cache:                map[string]oidcDiscoveryResult{},
	}
}

// oidcDiscoveryRequired reports whether an extension relies on OpenID discovery: it names an
// issuer and leaves at least one endpoint for the well-known document to supply.
//
// This is the single definition of "this extension will call discoverer.get()". It is shared by
// buildOAuth2ProviderConfig and by the refresh loop's live set so the two cannot drift: a live
// set wider than this polls issuers nobody reads, and one narrower prunes entries that are still
// needed, which would re-latch a discovery failure with nothing left to retry it.
func oidcDiscoveryRequired(in *kgwv1a1.OAuth2Provider) bool {
	if in == nil || in.IssuerURI == nil {
		return false
	}
	return in.TokenEndpoint == nil ||
		in.AuthorizationEndpoint == nil ||
		in.EndSessionEndpoint == nil ||
		(in.JWT != nil && in.JWT.JWKSURI == nil)
}

// oidcIssuerURIs collects the issuer URIs of the given extensions that rely on discovery. It
// feeds the discovery refresh loop, so that an issuer stops being polled once no extension
// discovers from it any more: because its GatewayExtension was deleted, was re-pointed at a
// different provider, or had every endpoint filled in explicitly.
func oidcIssuerURIs(exts []ir.GatewayExtension) []string {
	issuerURIs := make([]string, 0, len(exts))
	for _, ext := range exts {
		if !oidcDiscoveryRequired(ext.OAuth2) {
			continue
		}
		issuerURIs = append(issuerURIs, *ext.OAuth2.IssuerURI)
	}
	return issuerURIs
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
// are retried on failureRetryInterval, backing off on consecutive failures.
func (o *oidcProviderConfigDiscoverer) run(ctx context.Context) {
	// Tick at half the retry interval so an entry expiring just after a tick is not delayed by
	// a full extra period. time.NewTicker panics on a non-positive interval, so clamp.
	interval := o.failureRetryInterval / 2
	if interval <= 0 {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
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
	// Resolve the live set outside the lock: it calls into krt, which must not be done while
	// holding o.mu.
	live := sets.New(o.liveIssuerURIs()...)

	// Only refresh issuers that are both still discovered-from and already cached. Entries are
	// only ever added by get(), so a config no extension has ever asked for is never
	// discovered here; the live set additionally drops entries cached earlier that are no
	// longer needed.
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

	if len(expired) == 0 {
		return
	}

	// Re-discover concurrently, with a bound, so that recovery latency for one issuer does not
	// scale with the number of other unreachable issuers.
	changed := make([]bool, len(expired))
	var g errgroup.Group
	g.SetLimit(backgroundDiscoveryConcurrency)
	for i, issuerURI := range expired {
		g.Go(func() error {
			changed[i] = o.rediscover(ctx, issuerURI)
			return nil
		})
	}
	_ = g.Wait() // rediscover never returns an error; failures are recorded in the cache

	// Trigger once for the whole pass, outside o.mu: the trigger synchronously drives the
	// dependent transform, which calls back into load().
	if slices.Contains(changed, true) {
		logger.Debug("openid provider config changed, triggering recomputation")
		o.trigger.TriggerRecomputation()
	}
}

// rediscover re-runs discovery for issuerURI and replaces the cached entry, reporting whether
// the new outcome differs from the cached one.
func (o *oidcProviderConfigDiscoverer) rediscover(parent context.Context, issuerURI string) bool {
	// Never let shutdown look like a provider failure. A refresh pass can be cancelled part
	// way through, and overwriting a healthy cached config with "context canceled" would
	// report a change, trigger a recomputation, and set Err on every OAuth2 extension on the
	// way out.
	if parent.Err() != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(parent, backgroundDiscoveryTimeout)
	defer cancel()

	discoveryURL, err := oidcDiscoveryURL(issuerURI)
	if err != nil {
		// Not reachable in practice: the entry could only have been cached by a get() that
		// parsed the same URI successfully.
		logger.Warn("error refreshing OpenID provider config", "issuer_uri", issuerURI, "error", err)
		return false
	}

	cfg, err := o.discover(ctx, discoveryURL)
	// Check the parent, not ctx: a genuinely slow provider hits our own
	// backgroundDiscoveryTimeout and should be cached as the failure it is, whereas a
	// cancelled parent means we are shutting down and learned nothing about the provider.
	if err != nil && parent.Err() != nil {
		return false
	}

	o.mu.Lock()
	prev, ok := o.cache[issuerURI]
	if !ok {
		// The entry was pruned while we were discovering, because its GatewayExtension went
		// away. Don't resurrect it.
		o.mu.Unlock()
		return false
	}
	next := o.newResult(cfg, err, prev.failures)
	// A refresh failure must not withdraw a configuration that was already discovered
	// successfully. The provider document rarely changes, the proxies are running with the one
	// we have, and caching the error instead would set Err on the GatewayExtension, which
	// rejects every TrafficPolicy referencing it: a provider blip alone would take down
	// authentication on routes that were working. Serve the last known good config and let the
	// backed-off retry that newResult stamped on next.expiry pick up any change once the
	// provider is reachable again.
	servingLastKnownGood := err != nil && prev.err == nil
	if servingLastKnownGood {
		next.cfg, next.err = prev.cfg, nil
	}
	o.cache[issuerURI] = next
	o.mu.Unlock()

	if err != nil {
		logger.Warn("error refreshing OpenID provider config", "issuer_uri", issuerURI,
			"serving_last_known_good", servingLastKnownGood, "consecutive_failures", next.failures,
			"error", err)
	}
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

	// A malformed issuer URI is deliberately not cached. It can only be corrected by editing
	// the GatewayExtension, which is a tracked krt input that re-runs this transform on its
	// own, so there is nothing for the refresh loop to usefully retry: caching it would just
	// log a warning every pass, forever.
	discoveryURL, err := oidcDiscoveryURL(issuerURI)
	if err != nil {
		return nil, err
	}

	// Bound the time spent blocking the krt event loop; run() retries in the background.
	ctx, cancel := context.WithTimeout(ctx, foregroundDiscoveryTimeout)
	defer cancel()

	// Use singleflight to deduplicate concurrent discovery calls for the same issuer;
	// several transforms may call get() for the same issuer at once.
	v, _, _ := o.discoverGroup.Do(issuerURI, func() (any, error) {
		// Re-check the cache inside the singleflight function, as another caller
		// may have populated it between our initial load and entering the group.
		if result, ok := o.load(issuerURI); ok {
			return result, nil
		}
		cfg, err := o.discover(ctx, discoveryURL)
		result := o.newResult(cfg, err, 0)
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
// priorFailures is the consecutive-failure count of the entry being replaced (0 for a first
// discovery), so that a provider which stays down is polled with an exponential backoff rather
// than at a fixed interval for the whole outage.
func (o *oidcProviderConfigDiscoverer) newResult(cfg *oidcProviderConfig, err error, priorFailures int) oidcDiscoveryResult {
	if err == nil {
		return oidcDiscoveryResult{cfg: cfg, expiry: time.Now().Add(o.cacheRefreshInterval)}
	}

	failures := priorFailures + 1
	ttl := o.failureRetryInterval
	// Cap the shift before it can overflow, then cap the interval itself.
	if shift := min(failures-1, 16); shift > 0 {
		ttl <<= shift
	}
	if ttl > o.cacheRefreshInterval {
		ttl = o.cacheRefreshInterval
	}
	return oidcDiscoveryResult{err: err, expiry: time.Now().Add(ttl), failures: failures}
}

// oidcDiscoveryURL builds the well-known discovery URL for an issuer.
func oidcDiscoveryURL(issuerURI string) (string, error) {
	u, err := url.Parse(issuerURI + wellKnownOpenIDConfPath)
	if err != nil {
		return "", fmt.Errorf("error parsing discovery URL: %w", err)
	}
	return u.String(), nil
}

func (o *oidcProviderConfigDiscoverer) discover(ctx context.Context, discoveryURL string) (*oidcProviderConfig, error) {
	cfg := &oidcProviderConfig{}
	client := &http.Client{Timeout: oidcDiscoveryHTTPTimeout}
	err := retry.Do(func() error {
		// TODO: allow using custom certs for HTTPS Issuer URI
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
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
