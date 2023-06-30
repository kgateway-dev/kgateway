package helpers

import (
	"strings"

	"github.com/solo-io/go-utils/testutils/benchmarking"
)

func MeasureIgnore0ns(f func()) (benchmarking.Result, bool, error) {
	res, err := benchmarking.Measure(f)
	if err != nil && strings.Contains(err.Error(), "total execution time was 0 ns") {
		return res, true, nil
	}
	return res, false, err
}
