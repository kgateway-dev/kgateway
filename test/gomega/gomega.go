package gomega

import (
	"time"

	"github.com/onsi/gomega"
)

var (
	DefaultConsistentlyDuration        = time.Millisecond * 100
	DefaultConsistentlyPollingInterval = time.Millisecond * 10
	DefaultEventuallyTimeout           = time.Second * 1
	DefaultEventuallyPollingInterval   = time.Millisecond * 10
)

type AsyncAssertionDefaults struct {
	DefaultConsistentlyDuration        time.Duration
	DefaultConsistentlyPollingInterval time.Duration
	DefaultEventuallyTimeout           time.Duration
	DefaultEventuallyPollingInterval   time.Duration
}

// SetAsyncAssertionDefaults sets the default duration/timeout and polling interval for
// the default Gomega's Consistently and Eventually assertions. Values omitted from
// the passed in AsyncAssertionDefaults parameter will be set to the Gomega default
// values (also defined explicitly in this package).
func SetAsyncAssertionDefaults(asyncDefaults AsyncAssertionDefaults) {
	consistentlyDuration := asyncDefaults.DefaultConsistentlyDuration
	if consistentlyDuration <= 0 {
		consistentlyDuration = DefaultConsistentlyDuration
	}
	consistentlyPollingInterval := asyncDefaults.DefaultConsistentlyPollingInterval
	if consistentlyPollingInterval <= 0 {
		consistentlyPollingInterval = DefaultConsistentlyPollingInterval
	}
	eventuallyTimeout := asyncDefaults.DefaultEventuallyTimeout
	if eventuallyTimeout <= 0 {
		eventuallyTimeout = DefaultEventuallyTimeout
	}
	eventuallyPollingInterval := asyncDefaults.DefaultEventuallyPollingInterval
	if eventuallyPollingInterval <= 0 {
		eventuallyPollingInterval = DefaultEventuallyPollingInterval
	}
	gomega.SetDefaultConsistentlyDuration(consistentlyDuration)
	gomega.SetDefaultConsistentlyPollingInterval(consistentlyPollingInterval)
	gomega.SetDefaultEventuallyTimeout(eventuallyTimeout)
	gomega.SetDefaultEventuallyPollingInterval(eventuallyPollingInterval)
}
