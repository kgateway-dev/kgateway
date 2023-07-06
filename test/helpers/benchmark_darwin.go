package helpers

import (
	"time"
)

// MeasureIgnore0ns as implemented here for Mac/Darwin is meant to be used in performance tests when developing locally
// It is a less-precise method for measuring than the Linux implementation, and targets should be derived based on
// performance when running on the Linux GHA runner we use for Nightly tests
func MeasureIgnore0ns(f func()) (Result, bool, error) {
	before := time.Now()
	f()
	elapsed := time.Since(before)
	return Result{
		Total: elapsed,
	}, false, nil
}
