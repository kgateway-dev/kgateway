package trafficpolicy

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// discoveryKey identifies a cache entry: an issuer, plus the trust material used to reach it.
//
// The trust material is part of the key because two GatewayExtensions naming the same issuer
// can validate it differently, and because rotating the CA on a backend must not keep serving
// a config discovered against the old one.
type discoveryKey struct {
	issuerURI string
	// tlsDigest is the digest of the trust configuration, empty for the system trust store.
	tlsDigest string
}

func (k discoveryKey) String() string {
	if k.tlsDigest == "" {
		return k.issuerURI
	}
	return k.issuerURI + "|" + k.tlsDigest
}

// tlsDigest identifies a trust configuration for cache keying. The CA bundle is not used as
// the key itself: it is large, and a digest keeps keys cheap to compare and safe to log.
//
// Anything that resolves to the system trust store digests to the empty string, so a backend
// with no policy at all and one that explicitly selects the well-known system CA set share a
// cache entry rather than discovering the same issuer twice.
func tlsDigest(v *ir.UpstreamTLSValidation) string {
	switch {
	case v == nil:
		return ""
	case v.InsecureSkipVerify:
		return "insecure"
	case v.CAPEM == "":
		return ""
	default:
		sum := sha256.Sum256([]byte(v.CAPEM))
		return hex.EncodeToString(sum[:])
	}
}

// tlsClientConfig builds the TLS config for a discovery client. A nil validation, or one
// carrying nothing a Go client can act on, yields a nil config so that the client falls back
// to plain system-trust-store verification: the behavior before backend policies were
// honored here.
//
// A CA bundle replaces the system roots rather than extending them, matching what the same
// bundle does to the Envoy cluster's validation context.
func tlsClientConfig(v *ir.UpstreamTLSValidation) (*tls.Config, error) {
	if v == nil {
		return nil, nil
	}
	if v.InsecureSkipVerify {
		// The user disabled verification for this backend; discovery is not the place to
		// second-guess that, or discovery and the data path would disagree.
		return &tls.Config{InsecureSkipVerify: true}, nil //nolint:gosec // G402: explicitly requested by the attached policy
	}
	if v.CAPEM == "" {
		return nil, nil
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(v.CAPEM)) {
		return nil, errors.New("no valid certificate found in the CA bundle configured for the issuer backend")
	}
	// ServerName is deliberately left unset: the client authenticates the issuer URL's own
	// host, which is the server it dials. See ir.UpstreamTLSValidation.
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, nil
}

// discoveryClients memoizes an http.Client per distinct trust configuration. Without it every
// refresh pass would re-parse the CA bundle and build a fresh connection pool.
type discoveryClients struct {
	mu      sync.Mutex
	clients map[string]*http.Client
}

func (c *discoveryClients) get(digest string, v *ir.UpstreamTLSValidation) (*http.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if client, ok := c.clients[digest]; ok {
		return client, nil
	}
	tlsCfg, err := tlsClientConfig(v)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: oidcDiscoveryHTTPTimeout}
	if tlsCfg != nil {
		// Clone the default transport rather than building one from scratch, so proxy and
		// timeout defaults are kept and only the trust store differs.
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = tlsCfg
		client.Transport = transport
	}
	if c.clients == nil {
		c.clients = map[string]*http.Client{}
	}
	c.clients[digest] = client
	return client, nil
}

// retain drops the clients whose trust configuration is no longer cached, so that a rotating
// CA does not leak a connection pool per rotation.
func (c *discoveryClients) retain(digests sets.Set[string]) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for digest, client := range c.clients {
		if digests.Has(digest) {
			continue
		}
		client.CloseIdleConnections()
		delete(c.clients, digest)
	}
}

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
	// tls is the trust material this entry was discovered with. The refresh loop runs off
	// the krt event loop and so cannot resolve a backend's policies itself, so the transform
	// that cached the entry records them here for it to reuse.
	tls *ir.UpstreamTLSValidation
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

	// liveExtensions returns the GatewayExtensions that currently exist. The refresh loop
	// intersects the ones relying on discovery, per oidcDiscoveryRequired, with bindings so
	// it stops polling an entry as soon as no extension reads it.
	liveExtensions func() []ir.GatewayExtension

	cacheRefreshInterval time.Duration
	failureRetryInterval time.Duration

	// mu guards cache. The cache is authoritative for get(): expiry is acted on only by the
	// refresh loop, so a translation never blocks on discovery for a key already known.
	mu    sync.RWMutex
	cache map[discoveryKey]oidcDiscoveryResult

	// bindings maps a GatewayExtension's identity to the cache entry its last translation
	// read. It is what makes cache liveness consumer-aware: the live set alone only knows
	// which extensions exist, not which entry each one depends on, because it is resolved off
	// the krt event loop where a backend's attached policies cannot be looked up.
	//
	// Without it, two extensions sharing an issuer under different CAs cannot both be cached,
	// and neither can a CA rotation be told from a second live consumer.
	bindings map[string]discoveryKey

	// clients holds one http.Client per distinct trust configuration in the cache.
	clients discoveryClients

	// discoverGroup deduplicates concurrent discover() calls for the same key, preventing
	// redundant HTTP requests when several extensions share an issuer and trust material.
	discoverGroup singleflight.Group
}

// newOIDCProviderConfigDiscoverer returns an oidcProviderConfigDiscoverer that caches OpenID
// provider configurations. liveExtensions must report the GatewayExtensions that still exist,
// so the refresh loop can prune entries no extension reads any more. Callers must start run()
// for cached entries to be refreshed and retried.
func newOIDCProviderConfigDiscoverer(liveExtensions func() []ir.GatewayExtension, opts ...krt.CollectionOption) *oidcProviderConfigDiscoverer {
	return &oidcProviderConfigDiscoverer{
		// Start synced: get() discovers synchronously on first use, so dependent collections
		// must not block waiting for the refresh loop to publish an initial state.
		trigger:              krt.NewRecomputeTrigger(true, opts...),
		liveExtensions:       liveExtensions,
		cacheRefreshInterval: defaultOIDCCacheRefreshInterval,
		failureRetryInterval: defaultOIDCFailureRetryInterval,
		cache:                map[discoveryKey]oidcDiscoveryResult{},
		bindings:             map[string]discoveryKey{},
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

// oidcDiscoveryConsumers collects the identities of the given extensions that rely on
// discovery. It feeds the discovery refresh loop, so that an entry stops being polled once no
// extension reads it any more: because its GatewayExtension was deleted, was re-pointed at a
// different provider, or had every endpoint filled in explicitly.
func oidcDiscoveryConsumers(exts []ir.GatewayExtension) sets.Set[string] {
	consumers := sets.New[string]()
	for _, ext := range exts {
		if !oidcDiscoveryRequired(ext.OAuth2) {
			continue
		}
		consumers.Insert(ext.ResourceName())
	}
	return consumers
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

// refreshOnce prunes the cache entries no extension reads any more, re-discovers the expired
// ones, and triggers a single recomputation if any outcome changed.
func (o *oidcProviderConfigDiscoverer) refreshOnce(ctx context.Context) {
	// Resolve the live set outside the lock: it calls into krt, which must not be done while
	// holding o.mu.
	live := oidcDiscoveryConsumers(o.liveExtensions())

	var pruned, expired []discoveryKey
	now := time.Now()

	o.mu.Lock()
	// Release the bindings of extensions that no longer discover, then keep exactly the
	// entries the surviving bindings name. An entry can be named by several bindings, which is
	// what lets two extensions share an issuer, and an extension moved to a new CA releases
	// the entry it held without disturbing anyone else's.
	for extName := range o.bindings {
		if !live.Has(extName) {
			delete(o.bindings, extName)
		}
	}
	bound := sets.New[discoveryKey]()
	for _, key := range o.bindings {
		bound.Insert(key)
	}
	// Entries are only ever added by get(), so a config no extension has ever asked for is
	// never discovered here.
	for key, result := range o.cache {
		switch {
		case !bound.Has(key):
			pruned = append(pruned, key)
		case now.After(result.expiry):
			expired = append(expired, key)
		}
	}
	digests := sets.New[string]()
	for _, key := range pruned {
		delete(o.cache, key)
	}
	for key := range o.cache {
		digests.Insert(key.tlsDigest)
	}
	o.mu.Unlock()

	if len(pruned) > 0 {
		o.clients.retain(digests)
	}

	if len(expired) == 0 {
		return
	}

	// Re-discover concurrently, with a bound, so that recovery latency for one issuer does not
	// scale with the number of other unreachable issuers.
	changed := make([]bool, len(expired))
	var g errgroup.Group
	g.SetLimit(backgroundDiscoveryConcurrency)
	for i, key := range expired {
		g.Go(func() error {
			changed[i] = o.rediscover(ctx, key)
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

// rediscover re-runs discovery for key and replaces the cached entry, reporting whether the
// new outcome differs from the cached one.
func (o *oidcProviderConfigDiscoverer) rediscover(parent context.Context, key discoveryKey) bool {
	// Never let shutdown look like a provider failure. A refresh pass can be cancelled part
	// way through, and overwriting a healthy cached config with "context canceled" would
	// report a change, trigger a recomputation, and set Err on every OAuth2 extension on the
	// way out.
	if parent.Err() != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(parent, backgroundDiscoveryTimeout)
	defer cancel()

	discoveryURL, err := oidcDiscoveryURL(key.issuerURI)
	if err != nil {
		// Not reachable in practice: the entry could only have been cached by a get() that
		// parsed the same URI successfully.
		logger.Warn("error refreshing OpenID provider config", "issuer_uri", key.issuerURI, "error", err)
		return false
	}

	// Reuse the trust material this entry was discovered with; the refresh loop cannot
	// resolve the backend's policies itself.
	cached, ok := o.load(key)
	if !ok {
		// Pruned while a concurrent pass was running. Don't resurrect it.
		return false
	}

	cfg, err := o.discover(ctx, discoveryURL, key.tlsDigest, cached.tls)
	// Check the parent, not ctx: a genuinely slow provider hits our own
	// backgroundDiscoveryTimeout and should be cached as the failure it is, whereas a
	// cancelled parent means we are shutting down and learned nothing about the provider.
	if err != nil && parent.Err() != nil {
		return false
	}

	o.mu.Lock()
	prev, ok := o.cache[key]
	if !ok {
		// The entry was pruned while we were discovering, because its GatewayExtension went
		// away. Don't resurrect it.
		o.mu.Unlock()
		return false
	}
	next := o.newResult(cfg, err, prev.failures)
	// Carry forward what identifies the entry rather than its outcome.
	next.tls = prev.tls
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
	o.cache[key] = next
	o.mu.Unlock()

	if err != nil {
		logger.Warn("error refreshing OpenID provider config", "issuer_uri", key.issuerURI,
			"serving_last_known_good", servingLastKnownGood, "consecutive_failures", next.failures,
			"error", err)
	}
	return !prev.sameOutcome(next)
}

// get returns the OpenID provider config for issuerURI, discovering it if it is not already
// cached. Both successes and failures are cached; run() owns re-discovering them and
// triggering a recomputation when the outcome changes, so callers must have registered with
// markDependant to observe that change.
//
// extName identifies the GatewayExtension asking, so the refresh loop knows the entry is
// still read; it must be the same identity liveExtensions reports.
//
// tlsValidation is the trust material to validate the issuer with, resolved from the
// BackendTLSPolicy in effect on the backend the extension names, and nil to use the system
// trust store. It is part of the cache key, so re-pointing an extension at a different CA
// re-discovers rather than serving a config obtained under the old trust.
func (o *oidcProviderConfigDiscoverer) get(
	ctx context.Context,
	extName string,
	issuerURI string,
	tlsValidation *ir.UpstreamTLSValidation,
) (*oidcProviderConfig, error) {
	key := discoveryKey{issuerURI: issuerURI, tlsDigest: tlsDigest(tlsValidation)}

	// Claim the entry before discovering, and release whatever this extension held before.
	// Binding first matters: an entry cached by an unbound extension would be pruned by the
	// very next refresh pass.
	o.bind(extName, key)

	if result, ok := o.load(key); ok {
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

	// Use singleflight to deduplicate concurrent discovery calls for the same key; several
	// transforms may call get() for the same issuer at once.
	v, _, _ := o.discoverGroup.Do(key.String(), func() (any, error) {
		// Re-check the cache inside the singleflight function, as another caller
		// may have populated it between our initial load and entering the group.
		if result, ok := o.load(key); ok {
			return result, nil
		}
		cfg, err := o.discover(ctx, discoveryURL, key.tlsDigest, tlsValidation)
		result := o.newResult(cfg, err, 0)
		result.tls = tlsValidation
		o.mu.Lock()
		o.cache[key] = result
		o.mu.Unlock()
		return result, nil
	})
	// The discovery error is carried inside the result rather than returned from the
	// singleflight function, so that every waiter observes the same cached outcome.
	result := v.(oidcDiscoveryResult)
	return result.cfg, result.err
}

// bind records that extName now reads the entry at key, replacing whatever it read before.
func (o *oidcProviderConfigDiscoverer) bind(extName string, key discoveryKey) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.bindings[extName] = key
}

func (o *oidcProviderConfigDiscoverer) load(key discoveryKey) (oidcDiscoveryResult, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	result, ok := o.cache[key]
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

// discover fetches the provider configuration, validating the issuer with tlsValidation.
// digest identifies that trust configuration, and is what the memoized client is keyed by.
func (o *oidcProviderConfigDiscoverer) discover(
	ctx context.Context,
	discoveryURL string,
	digest string,
	tlsValidation *ir.UpstreamTLSValidation,
) (*oidcProviderConfig, error) {
	// A CA bundle that does not parse is a user error on the attached policy, so let it be
	// cached and reported like an unreachable provider rather than retried in a loop.
	client, err := o.clients.get(digest, tlsValidation)
	if err != nil {
		return nil, err
	}

	cfg := &oidcProviderConfig{}
	err = retry.Do(func() error {
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
