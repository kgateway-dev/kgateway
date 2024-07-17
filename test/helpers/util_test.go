package helpers_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/helpers"
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
		overrideTimeout = 4 * time.Second
		overridePolling = 314 * time.Millisecond
	)

	DescribeTable("getDefaultTimingsTransform", func(getTimeouts func(intervals ...interface{}) (interface{}, interface{}), defaultTimeout, defaultPolling interface{}) {
		timeout, pollingInterval := getTimeouts()
		Expect(timeout).To(Equal(defaultTimeout))
		Expect(pollingInterval).To(Equal(defaultPolling))

		timeout, pollingInterval = getTimeouts(10 * time.Second)
		Expect(timeout).To(Equal(10 * time.Second))
		Expect(pollingInterval).To(Equal(defaultPolling))

		timeout, pollingInterval = getTimeouts(10*time.Second, 20*time.Second)
		Expect(timeout).To(Equal(10 * time.Second))
		Expect(pollingInterval).To(Equal(20 * time.Second))
	},
		Entry("no defaults are provided for Eventually",
			helpers.GetEventuallyTimingsTransform(),
			helpers.GomegaDefaultEventuallyTimeout,
			helpers.GomegaDefaultEventuallyPollingInterval,
		),
		Entry("timeout default is provided for Eventually",
			helpers.GetEventuallyTimingsTransform(overrideTimeout),
			overrideTimeout,
			helpers.GomegaDefaultEventuallyPollingInterval,
		),
		Entry("timeout and polling interval defaults are provided for Eventually",
			helpers.GetEventuallyTimingsTransform(overrideTimeout, overridePolling),
			overrideTimeout,
			overridePolling,
		),
		Entry("no defaults are provided for Consistently",
			helpers.GetConsistentlyTimingsTransform(),
			helpers.GomegaDefaultConsistentlyTimeout,
			helpers.GomegaDefaultConsistentlyPollingInterval,
		),
		Entry("timeout default is provided for Consistently",
			helpers.GetConsistentlyTimingsTransform(overrideTimeout),
			overrideTimeout,
			helpers.GomegaDefaultConsistentlyPollingInterval,
		),
		Entry("timeout and polling interval defaults are provided for Consistently",
			helpers.GetConsistentlyTimingsTransform(overrideTimeout, overridePolling),
			overrideTimeout,
			overridePolling,
		),
	)

})
