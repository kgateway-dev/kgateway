package assertions

import (
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gleak"
	"github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/test/helpers"
)

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
	// Goroutines to ignore in addition to those stored in the GoroutineMonitor's goroutines field. See CommonLeakOptions for example.
	AllowedRoutines []types.GomegaMatcher
	// Additional arguments to pass to Eventually to control the timeout/polling interval.
	// If not set, defaults to 5 second timeout and the Gomega default polling interval (10ms)
	Timeouts []interface{}
}

var (
	defaultEventuallyTimeout = 5 * time.Second
	getEventuallyTimings     = helpers.GetEventuallyTimingsTransform(defaultEventuallyTimeout)
)

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

	timeout, pollingInterval := getEventuallyTimings(args.Timeouts...)
	Eventually(gleak.Goroutines, timeout, pollingInterval).ShouldNot(
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
