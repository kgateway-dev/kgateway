package validator

import (
	"context"
	"errors"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/sync/singleflight"
)

// DefaultCacheSize is the default LRU capacity for cachingValidator.
const DefaultCacheSize = 4096

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
}

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
	return &cachingValidator{inner: v, cache: cache}
}

func (c *cachingValidator) Validate(ctx context.Context, bootstrap *envoybootstrapv3.Bootstrap) error {
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
		return c.inner.Validate(ctx, bootstrap)
	}
	if hit, ok := c.cache.Get(key); ok {
		return hit.err()
	}
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
		return sfErr
	}
	return res.(cachedResult).err()
}
