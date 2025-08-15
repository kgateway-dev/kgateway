package assertions

import (
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gleak"
	"github.com/onsi/gomega/types"

	"github.com/kgateway-dev/kgateway/v2/test/helpers"
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
//	monitor = NewGoRoutineMonitor()
//	...
// }
//
// AfterEach(func() {
// // This example is from the controller tests. Different tests may have different allowed routines.
// // The way to figure out what routines to allow is to run the test with the goroutine leak detector enabled
// // and examining the reported goroutines if/when it fails. Those routines can be individually examined and
// // either added to the list or somehow mitigated (longer timeout, passing context to the goroutine)
// var allowedRoutines = []types.GomegaMatcher{
// 	gleak.IgnoringTopFunction("sync.runtime_notifyListWait [sync.Cond.Wait]"),
// 	gleak.IgnoringTopFunction("istio.io/istio/pkg/kube/krt.(*processorListener[...]).run [select]"),
// 	gleak.IgnoringTopFunction("istio.io/istio/pkg/kube/krt.(*processorListener[...]).pop [select]"),
// 	gleak.IgnoringTopFunction(`istio.io/istio/pkg/queue.(*queueImpl).Run.func2 [chan receive]`),
// }
//  monitor.AssertNoLeaks(&AssertNoLeaksArgs{
//		AllowedRoutines: allowedRoutines,
//	})
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

type AssertNoLeaksArgs struct {
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

func (m *GoRoutineMonitor) AssertNoLeaks(args *AssertNoLeaksArgs) {
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
