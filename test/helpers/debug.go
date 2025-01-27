//go:build ignore

package helpers

import (
	"testing"

	"go.uber.org/goleak"
)

// DeferredGoroutineLeakDetector returns a function that can be used in tests to identify goroutine leaks
// Example usage:
//
//	leakDetector := DeferredGoroutineLeakDetector(t)
//	defer leakDetector()
//	...
//
// When this fails, you will see:
//
//	 debug.go: found unexpected goroutines:
//			[list of Goroutines]
//
// If your tests fail for other reasons, and this leak detector is running, there may be Goroutines that
// were not cleaned up by the test due to the failure.
//
// NOTE TO DEVS: We would like to extend the usage of this across more test suites: https://github.com/kgateway-dev/kgateway/issues/7147
func DeferredGoroutineLeakDetector(t *testing.T) func(...goleak.Option) {
	leakOptions := []goleak.Option{
		goleak.IgnoreCurrent(),
		goleak.IgnoreTopFunction("github.com/onsi/ginkgo/v2/internal/interrupt_handler.(*InterruptHandler).registerForInterrupts.func2"),
	}

	return func(additionalLeakOptions ...goleak.Option) {
		goleak.VerifyNone(t, append(leakOptions, additionalLeakOptions...)...)
	}
}
