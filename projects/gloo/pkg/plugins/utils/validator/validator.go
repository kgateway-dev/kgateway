package validator

import (
	"context"
	"hash"

	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/go-utils/contextutils"
	"go.opencensus.io/stats"
	"google.golang.org/protobuf/runtime/protoiface"
	"k8s.io/utils/lru"
)

// Validator validates an envoy config by running it by envoy in validate mode. This requires the envoy binary to be present at $ENVOY_BINARY_PATH (defaults to /usr/local/bin/envoy)
type Validator interface {
	// ValidateConfig validates the given envoy config and returns any out and error from envoy. Returns nil if the envoy binary is not found.
	ValidateConfig(ctx context.Context, config HashableProtoMessage) error
}

type validator struct {
	filterName string
	// validationLruCache is a map of: (config hash) -> error state
	// this is usually a typed error but may be an untyped nil interface
	validationLruCache *lru.Cache
	// Counter to increment on cache hits
	cacheHits *stats.Int64Measure
	// Counter to increment on cache misses
	cacheMisses *stats.Int64Measure
}

// New returns a new Validator
func New(name string, filterName string, opts ...Option) validator {
	cfg := processOptions(name, opts...)
	return validator{
		filterName:         filterName,
		validationLruCache: lru.New(1024),
		cacheHits:          cfg.cacheHits,
		cacheMisses:        cfg.cacheMisses,
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

	// This transformation has already been validated, return the result
	if err, ok := v.validationLruCache.Get(hash); ok {
		utils.MeasureOne(
			ctx,
			v.cacheHits,
		)
		// Error may be nil here since it's just the cached result
		// so return it as a nil err after cast worst case.
		errCasted, _ := err.(error)
		return errCasted
	} else {
		utils.MeasureOne(
			ctx,
			v.cacheMisses,
		)
	}

	err = bootstrap.ValidateBootstrap(ctx, v.filterName, config)
	v.validationLruCache.Add(hash, err)
	return err
}
