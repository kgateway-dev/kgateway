//go:build ignore

package helpers

import (
	"fmt"
	"math"
)

// PercentileIndex returns the index of percentile pct for a slice of length len
// The Nearest Rank Method is used to determine percentiles (https://en.wikipedia.org/wiki/Percentile#The_nearest-rank_method)
// Valid inputs for pct are 0 < n <= 100, any other input will cause panic
func PercentileIndex(length, pct int) int {
	if pct <= 0 || pct > 100 {
		panic(fmt.Sprintf("percentile must be > 0 and <= 100, given %d", pct))
	}

	return int(math.Ceil(float64(length)*(float64(pct)/float64(100)))) - 1
}
