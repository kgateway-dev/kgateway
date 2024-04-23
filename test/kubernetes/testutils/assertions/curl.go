package assertions

import (
	"context"
	"time"

	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/gomega/transforms"
	"github.com/solo-io/gloo/test/kube2e/helper"
)

// EphemeralCurlEventuallyResponds returns a ClusterAssertion to assert that a set of curl.Option will return the expected matchers.HttpResponse
// This implementation relies on executing from an ephemeral container.
// It is the caller's responsibility to ensure the curlPodMeta points to a pod that is alive and ready to accept traffic
func (p *Provider) EphemeralCurlEventuallyResponds(curlPodMeta metav1.ObjectMeta, curlOptions []curl.Option, expectedResponse *matchers.HttpResponse, timeout ...time.Duration) ClusterAssertion {
	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()
		currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)

		// for some useful-ish output
		tick := time.Tick(currentTimeout / 8)

		Eventually(func(g Gomega) {
			res := p.clusterContext.Cli.CurlFromEphemeralPod(ctx, curlPodMeta, curlOptions...)
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
// We recommend that developers rely on the typed EphemeralCurlEventuallyResponds but we want to provide the flexibility of other solutions as well
func (p *Provider) CurlFnEventuallyResponds(curlFn func() string, expectedResponse *matchers.HttpResponse, timeout ...time.Duration) ClusterAssertion {
	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()
		currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)

		// for some useful-ish output
		tick := time.Tick(currentTimeout / 8)

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
