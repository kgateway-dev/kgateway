package helpers

import "time"

// Result represents the result of measuring a function's execution time.
type Result struct {
	// Time spent in user mode
	Utime time.Duration
	// Time spent in kernel mode
	Stime time.Duration
	// Time spent in user mode + kernel mode
	Total time.Duration
}
