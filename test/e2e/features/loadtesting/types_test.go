//go:build e2e

package loadtesting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidationMetricsDeltaClampsCounterResets(t *testing.T) {
	before := ValidationMetrics{
		Calls:            10,
		CacheHits:        9,
		CacheMisses:      8,
		Valid:            7,
		InvalidXDS:       6,
		InvocationErrors: 5,
		DurationCount:    4,
		DurationSeconds:  3.5,
		ByCaller: map[string]ValidationCallerMetrics{
			"route_full": {
				Calls:            10,
				CacheHits:        9,
				CacheMisses:      8,
				Valid:            7,
				InvalidXDS:       6,
				InvocationErrors: 5,
				DurationCount:    4,
				DurationSeconds:  3.5,
			},
		},
	}
	after := ValidationMetrics{
		Calls:            1,
		CacheHits:        1,
		CacheMisses:      1,
		Valid:            1,
		InvalidXDS:       1,
		InvocationErrors: 1,
		DurationCount:    1,
		DurationSeconds:  1,
		ByCaller: map[string]ValidationCallerMetrics{
			"route_full": {
				Calls:            1,
				CacheHits:        1,
				CacheMisses:      1,
				Valid:            1,
				InvalidXDS:       1,
				InvocationErrors: 1,
				DurationCount:    1,
				DurationSeconds:  1,
			},
		},
	}

	delta := after.Delta(before)

	assert.Zero(t, delta.Calls)
	assert.Zero(t, delta.CacheHits)
	assert.Zero(t, delta.CacheMisses)
	assert.Zero(t, delta.Valid)
	assert.Zero(t, delta.InvalidXDS)
	assert.Zero(t, delta.InvocationErrors)
	assert.Zero(t, delta.DurationCount)
	assert.Zero(t, delta.DurationSeconds)

	caller := delta.ByCaller["route_full"]
	assert.Zero(t, caller.Calls)
	assert.Zero(t, caller.CacheHits)
	assert.Zero(t, caller.CacheMisses)
	assert.Zero(t, caller.Valid)
	assert.Zero(t, caller.InvalidXDS)
	assert.Zero(t, caller.InvocationErrors)
	assert.Zero(t, caller.DurationCount)
	assert.Zero(t, caller.DurationSeconds)
}
