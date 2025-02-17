//go:build ignore

package helper_test

import (
	"time"

	"github.com/kgateway-dev/kgateway/v2/test/kube2e/helper"
	. "github.com/kgateway-dev/kgateway/v2/test/kube2e/helper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Curl tests", func() {
	When("GetTimeouts is called", func() {
		var (
			timeout         = helper.DefaultCurlTimeout + time.Second
			pollingInterval = helper.DefaultCurlPollingTimeout + time.Second
		)

		DescribeTable("should return correct timeouts", func(expectedTimeout, expectedPolling time.Duration, input ...time.Duration) {
			timeout, polling := GetTimeouts(input...)
			Expect(timeout).To(Equal(expectedTimeout))
			Expect(polling).To(Equal(expectedPolling))

		},
			Entry("default timeout", helper.DefaultCurlTimeout, helper.DefaultCurlPollingTimeout),
			Entry("pass timeout", timeout, helper.DefaultCurlPollingTimeout, timeout),
			Entry("pass timeout and polling", timeout, pollingInterval, timeout, pollingInterval),
			Entry("pass zeros", helper.DefaultCurlTimeout, helper.DefaultCurlPollingTimeout, 0*time.Second, 0*time.Second),
		)
	})
})
