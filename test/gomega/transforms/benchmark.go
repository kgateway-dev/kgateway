package transforms

import (
	"fmt"
	"sort"
	"time"
)

// WithPercentile returns a function that extracts the value at the given percentile from a slice of durations
func WithPercentile(percentile int) func(durations []time.Duration) time.Duration {
	if percentile <= 0 || percentile > 100 {
		panic(fmt.Sprintf("percentile must be >0 and <= 100, given %d", percentile))
	}
	return func(durations []time.Duration) time.Duration {
		sort.Slice(durations, func(i, j int) bool {
			return durations[i] < durations[j]
		})
		return durations[int(float64(len(durations))*(float64(percentile-1)/float64(100)))]
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
