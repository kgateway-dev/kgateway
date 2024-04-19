package assertions

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/gomega/transforms"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/go-utils/log"
)

func CurlEventuallyRespondsAssertion(curlFunc func() string, expectedResponse *matchers.HttpResponse, timeout ...time.Duration) ClusterAssertion {
	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()
		currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)
		// for some useful-ish output
		tick := time.Tick(currentTimeout / 8)

		Eventually(func(g Gomega) {
			res := curlFunc()
			select {
			default:
				break
			case <-tick:
				log.GreyPrintf("want %v\nhave: %s", expectedResponse, res)
			}

			expectedResponseMatcher := WithTransform(transforms.WithCurlHttpResponse, matchers.HaveHttpResponse(expectedResponse))
			g.Expect(res).To(expectedResponseMatcher)
			log.GreyPrintf("success: %v", res)

		}, currentTimeout, pollingInterval).Should(Succeed())
	}
}
