package helpers

import (
	"time"
)

func MeasureIgnore0ns(f func()) (Result, bool, error) {
	before := time.Now()
	f()
	elapsed := time.Since(before)
	return Result{
		Total: elapsed,
	}, false, nil
}
