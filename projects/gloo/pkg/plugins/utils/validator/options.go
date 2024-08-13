package validator

import (
	"fmt"

	"github.com/solo-io/gloo/pkg/utils/statsutils"
	"go.opencensus.io/stats"
)

type config struct {
	cacheHits   *stats.Int64Measure
	cacheMisses *stats.Int64Measure
	cacheSize   int
}

type Option func(*config)

func WithCounters(cacheHits, cacheMisses *stats.Int64Measure) Option {
	return func(s *config) {
		s.cacheHits = cacheHits
		s.cacheMisses = cacheMisses
	}
}

func WithCacheSize(size int) Option {
	return func(s *config) {
		s.cacheSize = size
	}
}

func processOptions(name string, options ...Option) *config {
	cfg := &config{
		cacheHits:   statsutils.MakeSumCounter(fmt.Sprintf("gloo.solo.io/%s_validation_cache_hits", name), fmt.Sprintf("The number of cache hits while validating %s config", name)),
		cacheMisses: statsutils.MakeSumCounter(fmt.Sprintf("gloo.solo.io/%s_validation_cache_misses", name), fmt.Sprintf("The number of cache misses while validating %s config", name)),
		cacheSize:   DefaultCacheSize,
	}
	for _, option := range options {
		option(cfg)
	}
	return cfg
}
