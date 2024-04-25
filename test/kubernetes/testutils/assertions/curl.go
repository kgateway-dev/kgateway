package assertions

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/gomega/transforms"
	"github.com/solo-io/gloo/test/kube2e/helper"
)

// EphemeralCurlEventuallyResponds returns a ClusterAssertion to assert that a set of curl.Option will return the expected matchers.HttpResponse
// This implementation relies on executing from an ephemeral container.
// It is the caller's responsibility to ensure the curlPodMeta points to a pod that is alive and ready to accept traffic
func (p *Provider) EphemeralCurlEventuallyResponds(curlPod client.Object, curlOptions []curl.Option, expectedResponse *matchers.HttpResponse, timeout ...time.Duration) ClusterAssertion {
	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()

		// We rely on the curlPod to execute a curl, therefore we must assert that it actually exists
		p.ObjectsExist(curlPod)(ctx)

		currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)

		// for some useful-ish output
		tick := time.Tick(pollingInterval)

		Eventually(func(g Gomega) {
			res := p.clusterContext.Cli.CurlFromEphemeralPod(ctx, client.ObjectKeyFromObject(curlPod), curlOptions...)
			select {
			default:
				break
			case <-tick:
				ginkgo.GinkgoWriter.Printf("want %v\nhave: %s", expectedResponse, res)
			}

			expectedResponseMatcher := WithTransform(transforms.WithCurlHttpResponse, matchers.HaveHttpResponse(expectedResponse))
			g.Expect(res).To(expectedResponseMatcher)
			ginkgo.GinkgoWriter.Printf("success: %v", res)
		}).
			WithTimeout(currentTimeout).
			WithPolling(pollingInterval).
			WithContext(ctx).
			Should(Succeed())
	}
}

// CurlFnEventuallyResponds returns a ClusterAssertion that behaves similarly to EphemeralCurlEventuallyResponds
// The difference is that it accepts a generic function to execute the curl, instead of requiring the caller to pass explicit curl.Option
// Not all curl requests should be done from an ephemeral container, and this function allows for that to occur
func (p *Provider) CurlFnEventuallyResponds(curlFn func() string, expectedResponse *matchers.HttpResponse, timeout ...time.Duration) ClusterAssertion {
	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()
		currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)

		// for some useful-ish output
		tick := time.Tick(pollingInterval)

		Eventually(func(g Gomega) {
			res := curlFn()
			select {
			default:
				break
			case <-tick:
				ginkgo.GinkgoWriter.Printf("want %v\nhave: %s", expectedResponse, res)
			}

			expectedResponseMatcher := WithTransform(transforms.WithCurlHttpResponse, matchers.HaveHttpResponse(expectedResponse))
			g.Expect(res).To(expectedResponseMatcher)
			ginkgo.GinkgoWriter.Printf("success: %v", res)
		}).
			WithTimeout(currentTimeout).
			WithPolling(pollingInterval).
			WithContext(ctx).
			Should(Succeed())
	}
}
