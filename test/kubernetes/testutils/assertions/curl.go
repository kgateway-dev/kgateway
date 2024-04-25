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

// EventuallyEphemeralCurlEventuallyResponds asserts that a set of curl.Option will return the expected matchers.HttpResponse
// This implementation relies on executing from an ephemeral container.
// It is the caller's responsibility to ensure the curlPodMeta points to a pod that is alive and ready to accept traffic
func (p *Provider) EventuallyEphemeralCurlEventuallyResponds(
	ctx context.Context,
	curlPod client.Object,
	curlOptions []curl.Option,
	expectedResponse *matchers.HttpResponse,
	timeout ...time.Duration) {

	p.EventuallyObjectsExist(ctx, curlPod)

	currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)

	// for some useful-ish output
	tick := time.Tick(pollingInterval)

	p.Gomega.Eventually(func(g Gomega) {
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
