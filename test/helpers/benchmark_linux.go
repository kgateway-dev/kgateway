package helpers

import (
	"strings"

	"github.com/solo-io/go-utils/testutils/benchmarking"
)

// MeasureIgnore0ns wraps benchmarking.Measure, checking error values for the 0ns error and returning true as the middle
// argument if that is the error
// The 0ns error occurs when measuring very short durations (~100µs) when measurements may round down to 0ns
// This function should be used in circumstances where we want to ignore that particular error but not others
func MeasureIgnore0ns(f func()) (benchmarking.Result, bool, error) {
	res, err := Measure(f)
	if err != nil && strings.Contains(err.Error(), "total execution time was 0 ns") {
		return res, true, nil
	}
	return res, false, err
}

// Measure wraps benchmarking.Measure 1:1
// It is redefined here so that we can build with a darwin-compatible version depending on the machine being used
func Measure(f func()) (benchmarking.Result, error) {
	return benchmarking.Measure(f)
}
