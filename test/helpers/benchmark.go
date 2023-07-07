package helpers

import (
	"github.com/onsi/gomega/types"
	"time"
)

// Result represents the result of measuring a function's execution time.
type Result struct {
	// Time spent in user mode
	Utime time.Duration
	// Time spent in kernel mode
	Stime time.Duration
	// Time spent in user mode + kernel mode
	Total time.Duration
}

// BenchmarkConfig allows configuration for benchmarking tests to be reused for similar cases
// This struct can be factored out to an accessible location should additional benchmarking suites be added
type BenchmarkConfig struct {
	Iterations        int                   // the number of iterations to attempt for a particular entry
	MaxDur            time.Duration         // the maximum time to spend on a particular entry even if not all iterations are complete
	BenchmarkMatchers []types.GomegaMatcher // matchers representing the assertions we wish to make for a particular entry
}
