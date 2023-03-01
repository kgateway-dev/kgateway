package helpers

import (
	"testing"

	"go.uber.org/goleak"
)

// DeferredGoroutineLeakDetector returns a function that can be used in tests to identify goroutine leaks
// Example usage:
//
//		leakDetector := DeferredGoroutineLeakDetector(t)
//	 defer leakDetector()
//	 ...
//
// NOTE TO DEVS: We would like to extend the usage of this across more test suites: https://github.com/solo-io/gloo/issues/7147
func DeferredGoroutineLeakDetector(t *testing.T) func() {
	leakOptions := []goleak.Option{
		goleak.IgnoreCurrent(),
		goleak.IgnoreTopFunction("github.com/onsi/ginkgo/v2/internal/interrupt_handler.(*InterruptHandler).registerForInterrupts.func2"),
	}

	return func() {
		goleak.VerifyNone(t, leakOptions...)
	}
}
