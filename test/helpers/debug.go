package helpers

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gleak"
	"github.com/onsi/gomega/types"
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
// NOTE TO DEVS: We would like to extend the usage of this across more test suites: https://github.com/solo-io/gloo/issues/7147
func DeferredGoroutineLeakDetector(t *testing.T) func(...goleak.Option) {
	leakOptions := []goleak.Option{
		goleak.IgnoreCurrent(),
		goleak.IgnoreTopFunction("github.com/onsi/ginkgo/v2/internal/interrupt_handler.(*InterruptHandler).registerForInterrupts.func2"),
	}

	return func(additionalLeakOptions ...goleak.Option) {
		goleak.VerifyNone(t, append(leakOptions, additionalLeakOptions...)...)
	}
}

// GoRoutineMonitor is a helper for monitoring goroutine leaks in tests
// This is useful for individual tests, and does not need `t *testing.T` which is unavailable in ginkgo tests
//
// It also allows for more fine-grained control over the leak detection by allowing arguments to be passed to the
//`ExpectNoLeaks` function, in order to allow certain "safe" or expected goroutines to be ignored
//
// The use of `Eventually` also makes this routine useful for tests that may have a delay in the cleanup of goroutines,
// such as when `cancel()` is called, and the next test should not be started until all goroutines are cleaned up
//
// Example usage:
// BeforeEach(func() {
//	monitor := NewGoRoutineMonitor()
//	...
// }
//
// AfterEach(func() {
//  monitor.ExpectNoLeaks(helpers.CommonLeakOptions...)
// }

type GoRoutineMonitor struct {
	goroutines []gleak.Goroutine
}

func NewGoRoutineMonitor() *GoRoutineMonitor {
	// this is a workaround for the fact that the wasm plugin creates goroutines
	// by calling this before the initial goroutines are recorded, we can ignore them in the leak check
	//_ = wasm.NewPlugin()

	return &GoRoutineMonitor{
		goroutines: gleak.Goroutines(),
	}
}

func (m *GoRoutineMonitor) ExpectNoLeaks(allowedRoutines ...types.GomegaMatcher) {
	// Need to gather up the arguments to pass to the leak detector, so need to make sure they are all interface{}s
	// Arguments are the initial goroutines, and any additional allowed goroutines passed in
	notLeaks := make([]interface{}, len(allowedRoutines)+1)
	// First element is the initial goroutines
	notLeaks[0] = m.goroutines
	// Cast the rest of the elements to interface{}
	for i, v := range allowedRoutines {
		notLeaks[i+1] = v
	}

	Eventually(gleak.Goroutines, 5*time.Second).ShouldNot(
		gleak.HaveLeaked(
			notLeaks...,
		),
	)
}

var CommonLeakOptions = []types.GomegaMatcher{
	gleak.IgnoringTopFunction("os/exec..."),
	gleak.IgnoringInBacktrace("github.com/solo-io/solo-projects/test/v1helpers.RunTestServer..."),
	gleak.IgnoringTopFunction("github.com/solo-io/solo-projects/test/v1helpers.RunTestServer.func2"),
	gleak.IgnoringTopFunction("internal/poll.runtime_pollWait"),
	gleak.IgnoringInBacktrace("github.com/solo-io/gloo/test/services.MustStopAndRemoveContainer"),
}
