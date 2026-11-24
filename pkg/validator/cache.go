package validator

import (
	"context"
	"errors"
	"fmt"
	"time"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/sync/singleflight"
)

// DefaultCacheSize is the default LRU capacity for cachingValidator. It is the
// single source of truth for the default: ValidatorCacheSize in api/settings
// defaults to 0, which routes through NewCaching's size <= 0 fallback to this
// value.
const DefaultCacheSize = 65536

// cachingValidator wraps an inner Validator with an LRU cache keyed on the
// content hash of the marshalled bootstrap. Successful and ErrInvalidXDS
// outcomes are memoized; transient errors (e.g. exec failures, context
// cancellation) are not, so flaky underlying invocations do not get pinned in
// the cache. Concurrent misses on the same key are collapsed to one inner
// invocation via singleflight. During initial sync, independent collections can
// validate identical content concurrently, and without this each concurrent
// miss would fork its own envoy.
type cachingValidator struct {
	inner Validator
	cache *lru.Cache[string, cachedResult]
	sf    singleflight.Group

	// bootstrapCache holds proxy bootstrap verdicts, keyed by the bootstrap's
	// ContentKey. It is separate from cache because a proxy bootstrap is validated
	// as the proxy runs it, not as an xDS validation bootstrap.
	bootstrapCache *lru.Cache[ContentKey, cachedBootstrapResult]
	bootstrapSF    singleflight.Group
}

// bootstrapCacheSize is the LRU capacity for proxy bootstrap verdicts. Each Gateway
// with bootstrap overlays has one current bootstrap, plus one per recent change.
const bootstrapCacheSize = 1024

// cachedBootstrapResult holds a memoized proxy bootstrap verdict. msg is the
// rejection's text verbatim, empty when Envoy accepted the bootstrap.
type cachedBootstrapResult struct {
	msg string
}

func (r cachedBootstrapResult) err() error {
	if r.msg != "" {
		return &cachedInvalidBootstrapError{msg: r.msg}
	}
	return nil
}

// cachedInvalidBootstrapError rehydrates a memoized ErrInvalidBootstrap verdict.
type cachedInvalidBootstrapError struct{ msg string }

func (e *cachedInvalidBootstrapError) Error() string { return e.msg }
func (e *cachedInvalidBootstrapError) Unwrap() error { return ErrInvalidBootstrap }

// cachedResult holds a memoized validation outcome. msg is the inner error's
// text verbatim (empty on success); it is rehydrated as an error that
// satisfies errors.Is(err, ErrInvalidXDS) without re-wrapping, so the message
// renders exactly as the inner validator produced it regardless of how the
// producer formatted its error chain.
type cachedResult struct {
	ok  bool
	msg string
}

func (r cachedResult) err() error {
	if r.ok {
		return nil
	}
	return &cachedInvalidError{msg: r.msg}
}

// cachedInvalidError rehydrates a memoized ErrInvalidXDS verdict: the message
// is the original error text, and Unwrap preserves errors.Is identity.
type cachedInvalidError struct{ msg string }

func (e *cachedInvalidError) Error() string { return e.msg }
func (e *cachedInvalidError) Unwrap() error { return ErrInvalidXDS }

// NewCaching wraps v with an LRU result cache of the given size. If size <= 0,
// DefaultCacheSize is used.
func NewCaching(v Validator, size int) Validator {
	if size <= 0 {
		size = DefaultCacheSize
	}
	cache, err := lru.New[string, cachedResult](size)
	if err != nil {
		// lru.New only errors when size <= 0, which we already guarded against.
		// Fall back to a passthrough validator rather than panicking.
		return v
	}
	bootstrapCache, err := lru.New[ContentKey, cachedBootstrapResult](bootstrapCacheSize)
	if err != nil {
		return v
	}
	return &cachingValidator{inner: v, cache: cache, bootstrapCache: bootstrapCache}
}

var _ BootstrapValidator = &cachingValidator{}

// ValidateBootstrap validates bs with the inner validator, which must implement
// BootstrapValidator, and memoizes acceptance and ErrInvalidBootstrap verdicts. Like
// Validate, it does not memoize errors from running Envoy.
func (c *cachingValidator) ValidateBootstrap(ctx context.Context, bs *envoybootstrapv3.Bootstrap) error {
	// Find the inner validator and the key.
	inner, ok := c.inner.(BootstrapValidator)
	if !ok {
		return fmt.Errorf("%w: the configured validator cannot validate proxy bootstraps", ErrBootstrapValidatorUnavailable)
	}
	key, err := ContentKeyOf(bs)
	if err != nil {
		return inner.ValidateBootstrap(ctx, bs)
	}

	// Return a memoized verdict.
	if hit, ok := c.bootstrapCache.Get(key); ok {
		return hit.err()
	}

	// Validate once for concurrent misses on the same content.
	res, sfErr, _ := c.bootstrapSF.Do(string(key[:]), func() (any, error) {
		innerErr := inner.ValidateBootstrap(ctx, bs)
		var r cachedBootstrapResult
		switch {
		case innerErr == nil:
		case errors.Is(innerErr, ErrInvalidBootstrap):
			r = cachedBootstrapResult{msg: innerErr.Error()}
		default:
			return nil, innerErr
		}
		c.bootstrapCache.Add(key, r)
		return r, nil
	})
	if sfErr != nil {
		return sfErr
	}
	return res.(cachedBootstrapResult).err()
}

func (c *cachingValidator) Validate(ctx context.Context, bootstrap *envoybootstrapv3.Bootstrap) error {
	start := time.Now()
	caller := validationCaller(ctx)
	recordValidationCall(caller)

	// The key is an in-process cache key only: it hashes protojson output,
	// which is deterministic for a given binary but deliberately unstable
	// across builds (protojson varies whitespace via a seed derived from a
	// hash of the executable itself; see protobuf's internal/detrand). It
	// must never be persisted or compared across versions; if that ever
	// happens anyway, the failure mode is a cache miss, never a wrong
	// verdict.
	key, err := cacheKeyFor(bootstrap)
	if err != nil {
		// If we cannot compute a key, fall through to the inner validator.
		innerErr := c.inner.Validate(ctx, bootstrap)
		recordValidationResult(caller, validationResultFromError(innerErr), start)
		return innerErr
	}

	if hit, ok := c.cache.Get(key); ok {
		recordValidationCacheHit(caller)
		recordValidationResult(caller, validationResultFromError(hit.err()), start)
		return hit.err()
	}
	recordValidationCacheMiss(caller)

	// Collapse concurrent misses on the same content to one inner call. The
	// shared call runs under the leader's context; if that call fails
	// transiently (cancellation, exec error), every waiter sees the transient
	// error and nothing is cached.
	res, sfErr, _ := c.sf.Do(key, func() (any, error) {
		innerErr := c.inner.Validate(ctx, bootstrap)
		if innerErr == nil {
			r := cachedResult{ok: true}
			c.cache.Add(key, r)
			return r, nil
		}
		if errors.Is(innerErr, ErrInvalidXDS) {
			r := cachedResult{msg: innerErr.Error()}
			c.cache.Add(key, r)
			return r, nil
		}
		return nil, innerErr
	})
	if sfErr != nil {
		recordValidationResult(caller, validationResultFromError(sfErr), start)
		return sfErr
	}
	result := res.(cachedResult)
	recordValidationResult(caller, validationResultFromError(result.err()), start)
	return result.err()
}
