package transforms

import (
	"github.com/solo-io/gloo/test/helpers"
	"sort"
	"time"
)

// WithPercentile returns a function that extracts the value at the given percentile from a slice of durations
// The Nearest Rank Method is used to determine percentiles (https://en.wikipedia.org/wiki/Percentile#The_nearest-rank_method)
// Valid inputs are 0 < n <= 100
func WithPercentile(percentile int) func(durations []time.Duration) time.Duration {
	return func(durations []time.Duration) time.Duration {
		sort.Slice(durations, func(i, j int) bool {
			return durations[i] < durations[j]
		})

		idx := helpers.PercentileIndex(len(durations), percentile)
		return durations[idx]
	}
}

// WithMedian returns a function that extracts the value at the median from a slice of durations
func WithMedian() func([]time.Duration) time.Duration {
	return func(durations []time.Duration) time.Duration {
		sort.Slice(durations, func(i, j int) bool {
			return durations[i] < durations[j]
		})
		var median time.Duration
		if l := len(durations); l%2 == 1 {
			median = durations[l/2]
		} else {
			median = (durations[l/2] + durations[l/2-1]) / 2
		}
		return median
	}
}
