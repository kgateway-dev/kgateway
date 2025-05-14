//go:build ignore

package helpers_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/test/gomega"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
)

var _ = Describe("PercentileIndex", func() {
	It("panics on percentile <= 0", func() {
		Expect(func() { helpers.PercentileIndex(100, -1) }).To(Panic())
	})

	It("panics on percentile > 100", func() {
		Expect(func() { helpers.PercentileIndex(100, 101) }).To(Panic())
	})

	It("returns 0 for 1st percentile for len <=100", func() {
		for i := 1; i <= 100; i++ {
			Expect(helpers.PercentileIndex(i, 1)).To(Equal(0))
		}
	})

	It("returns 1 for 1st percentile for len >100, <=200", func() {
		for i := 101; i <= 200; i++ {
			Expect(helpers.PercentileIndex(i, 1)).To(Equal(1))
		}
	})

	It("always returns len-1 for 100th percentile", func() {
		for i := 1; i <= 200; i++ {
			Expect(helpers.PercentileIndex(i, 100)).To(Equal(i - 1))
		}
	})

	It("returns index 3 for 80th percentile and length 5", func() {
		Expect(helpers.PercentileIndex(5, 80)).To(Equal(3))
	})
})

var _ = Describe("transforms for eventually/consistency timing parameters", func() {

	const (
		overrideTimeout       = 4 * time.Second
		overridePolling       = 314 * time.Millisecond
		overrideTimeoutString = "4s"
		overridePollingString = "314ms"
	)

	DescribeTable("GetDefaultTimingsTransform", func(getTimeouts func(intervals ...interface{}) (interface{}, interface{}), defaultTimeout, defaultPolling interface{}) {
		// Use defaults
		timeout, pollingInterval := getTimeouts()
		Expect(timeout).To(Equal(defaultTimeout))
		Expect(pollingInterval).To(Equal(defaultPolling))

		// Specify timeout
		timeout, pollingInterval = getTimeouts(10 * time.Second)
		Expect(timeout).To(Equal(10 * time.Second))
		Expect(pollingInterval).To(Equal(defaultPolling))

		// Specify timout and polling interval
		timeout, pollingInterval = getTimeouts(10*time.Second, 20*time.Second)
		Expect(timeout).To(Equal(10 * time.Second))
		Expect(pollingInterval).To(Equal(20 * time.Second))

		// Check 0's are handled correctly
		timeout, pollingInterval = getTimeouts(0, 0)
		Expect(timeout).To(Equal(defaultTimeout))
		Expect(pollingInterval).To(Equal(defaultPolling))

		// Check 0 durations are handled correctly
		timeout, pollingInterval = getTimeouts(0*time.Second, 0*time.Second)
		Expect(timeout).To(Equal(defaultTimeout))
		Expect(pollingInterval).To(Equal(defaultPolling))

		// Check string durations are handled correctly
		timeout, pollingInterval = getTimeouts(overrideTimeoutString, overridePollingString)
		Expect(timeout).To(Equal(overrideTimeout))
		Expect(pollingInterval).To(Equal(overridePolling))
	},
		Entry("no defaults are provided for Eventually",
			helpers.GetEventuallyTimingsTransform(),
			gomega.DefaultEventuallyTimeout,
			gomega.DefaultEventuallyPollingInterval,
		),
		Entry("timeout default is provided for Eventually",
			helpers.GetEventuallyTimingsTransform(overrideTimeout),
			overrideTimeout,
			gomega.DefaultEventuallyPollingInterval,
		),
		Entry("timeout and polling interval defaults are provided for Eventually",
			helpers.GetEventuallyTimingsTransform(overrideTimeout, overridePolling),
			overrideTimeout,
			overridePolling,
		),
		Entry("no defaults are provided for Consistently",
			helpers.GetConsistentlyTimingsTransform(),
			gomega.DefaultConsistentlyDuration,
			gomega.DefaultConsistentlyPollingInterval,
		),
		Entry("timeout default is provided for Consistently",
			helpers.GetConsistentlyTimingsTransform(overrideTimeout),
			overrideTimeout,
			gomega.DefaultConsistentlyPollingInterval,
		),
		Entry("timeout and polling interval defaults are provided for Consistently",
			helpers.GetConsistentlyTimingsTransform(overrideTimeout, overridePolling),
			overrideTimeout,
			overridePolling,
		),
	)

})
