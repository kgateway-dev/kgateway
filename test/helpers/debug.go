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
// This is useful for individual tests and does not need `t *testing.T` which is unavailable in ginkgo tests
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
	// Store the initial goroutines
	return &GoRoutineMonitor{
		goroutines: gleak.Goroutines(),
	}
}

type ExpectNoLeaksArgs struct {
	AllowedRoutines []types.GomegaMatcher // Additional allowed goroutines to ignore. See CommonLeakOptions for example.
	Timeouts        []time.Duration       // Additional arguments to pass to Eventually to control the timeout/polling interval
}

func (m *GoRoutineMonitor) ExpectNoLeaks(args *ExpectNoLeaksArgs) {
	// Need to gather up the arguments to pass to the leak detector, so need to make sure they are all interface{}s
	// Arguments are the initial goroutines, and any additional allowed goroutines passed in
	notLeaks := make([]interface{}, len(args.AllowedRoutines)+1)
	// First element is the initial goroutines
	notLeaks[0] = m.goroutines
	// Cast the rest of the elements to interface{}
	for i, v := range args.AllowedRoutines {
		notLeaks[i+1] = v
	}

	// Determine the time intervals to pass to Eventually
	var (
		timeouts                 []interface{}
		defaultEventuallyTimeout = 5 * time.Second
	)
	if len(args.Timeouts) > 0 {
		timeouts = make([]interface{}, len(args.Timeouts))
		for i, v := range args.Timeouts {
			timeouts[i] = v
		}
	} else {
		timeouts = make([]interface{}, 1)
		timeouts[0] = defaultEventuallyTimeout
	}

	Eventually(gleak.Goroutines, timeouts...).ShouldNot(
		gleak.HaveLeaked(
			notLeaks...,
		),
	)
}

// CommonLeakOptions are options to ignore in the goroutine leak detector
// If we are running tests, we will likely have the test framework running and will expect to see these goroutines
var CommonLeakOptions = []types.GomegaMatcher{
	gleak.IgnoringTopFunction("os/exec..."),
	gleak.IgnoringTopFunction("internal/poll.runtime_pollWait"),
}
