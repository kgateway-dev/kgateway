package assertions

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/go-utils/log"
)

func CurlEventuallyRespondsAssertion(ctx context.Context, curlFunc func() string, logResponses bool, expectedResponse interface{}, ginkgoOffset int, timeout ...time.Duration) ClusterAssertion {
	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()
		currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)
		// for some useful-ish output
		tick := time.Tick(currentTimeout / 8)

		EventuallyWithOffset(ginkgoOffset+1, func(g Gomega) {
			res := curlFunc()
			select {
			default:
				break
			case <-tick:
				if logResponses {
					log.GreyPrintf("want %v\nhave: %s", expectedResponse, res)
				}
			}

			expectedResponseMatcher := helper.GetExpectedResponseMatcher(expectedResponse)
			g.Expect(res).To(expectedResponseMatcher)
			if logResponses {
				log.GreyPrintf("success: %v", res)
			}

		}, currentTimeout, pollingInterval).Should(Succeed())
	}
}
