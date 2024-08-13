package validator

import (
	"context"
	"hash"

	"github.com/solo-io/gloo/pkg/utils/statsutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/go-utils/contextutils"
	"go.opencensus.io/stats"
	"google.golang.org/protobuf/runtime/protoiface"
	"k8s.io/utils/lru"
)

// DefaultCacheSize defines the default size of the LRU cache used by the validator
const DefaultCacheSize int = 1024

// Validator validates an envoy config by running it by envoy in validate mode. This requires the envoy binary to be present at $ENVOY_BINARY_PATH (defaults to /usr/local/bin/envoy).
// Results are cached via an LRU cache for performance
type Validator interface {
	// ValidateConfig validates the given envoy config and returns any out and error from envoy. Returns nil if the envoy binary is not found.
	ValidateConfig(ctx context.Context, config HashableProtoMessage) error

	// CacheLength returns the returns the number of items in the cache
	CacheLength() int
}

type validator struct {
	filterName string
	// lruCache is a map of: (config hash) -> error state
	// this is usually a typed error but may be an untyped nil interface
	lruCache *lru.Cache
	// Counter to increment on cache hits
	cacheHits *stats.Int64Measure
	// Counter to increment on cache misses
	cacheMisses *stats.Int64Measure
}

// New returns a new Validator
func New(name string, filterName string, opts ...Option) validator {
	cfg := processOptions(name, opts...)
	return validator{
		filterName:  filterName,
		lruCache:    lru.New(cfg.cacheSize),
		cacheHits:   cfg.cacheHits,
		cacheMisses: cfg.cacheMisses,
	}
}

// HashableProtoMessage defines a ProtoMessage that can be hashed. Useful when passing different ProtoMessages objects that need to be hashed.
type HashableProtoMessage interface {
	protoiface.MessageV1
	Hash(hasher hash.Hash64) (uint64, error)
}

func (v validator) ValidateConfig(ctx context.Context, config HashableProtoMessage) error {
	hash, err := config.Hash(nil)
	if err != nil {
		contextutils.LoggerFrom(ctx).DPanicf("error hashing the config, should never happen: %v", err)
		return err
	}

	// This proto has already been validated, return the result
	if err, ok := v.lruCache.Get(hash); ok {
		statsutils.MeasureOne(
			ctx,
			v.cacheHits,
		)
		// Error may be nil here since it's just the cached result
		// so return it as a nil err after cast worst case.
		errCasted, _ := err.(error)
		return errCasted
	}
	statsutils.MeasureOne(
		ctx,
		v.cacheMisses,
	)

	err = bootstrap.ValidateBootstrap(ctx, v.filterName, config)
	v.lruCache.Add(hash, err)
	return err
}

func (v validator) CacheLength() int {
	return v.lruCache.Len()
}
