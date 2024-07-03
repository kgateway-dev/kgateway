package assertions

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/gomega/transforms"
	"github.com/solo-io/gloo/test/kube2e/helper"
)

func (p *Provider) AssertEventualCurlReturnResponse(
	ctx context.Context,
	podOpts kubectl.PodExecOptions,
	curlOptions []curl.Option,
	expectedResponse *matchers.HttpResponse,
	timeout ...time.Duration,
) *http.Response {
	// We rely on the curlPod to execute a curl, therefore we must assert that it actually exists
	p.EventuallyObjectsExist(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podOpts.Name, Namespace: podOpts.Namespace,
		},
	})

	currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)

	var curlHttpResponse *http.Response
	p.Gomega.Eventually(func(g Gomega) {
		curlResponse, err := p.clusterContext.Cli.CurlFromPod(ctx, podOpts, curlOptions...)
		fmt.Printf("want:\n%+v\nstdout:\n%s\nstderr:%s\n\n", expectedResponse, curlResponse.StdOut, curlResponse.StdErr)
		g.Expect(err).NotTo(HaveOccurred())

		// Do the transform in a separate step instead of a WithTransform to avoid having to do it twice
		//nolint:bodyclose // The caller of this assertion should be responsible for ensuring the body close - if the response is not needed for the test, AssertEventualCurlResponse should be used instead
		curlHttpResponse = transforms.WithCurlResponse(curlResponse)
		g.Expect(curlHttpResponse).To(matchers.HaveHttpResponse(expectedResponse))
		fmt.Printf("success: %+v", curlResponse)
	}).
		WithTimeout(currentTimeout).
		WithPolling(pollingInterval).
		WithContext(ctx).
		Should(Succeed(), "failed to get expected response")

	return curlHttpResponse
}

// We can't use one function and ignore the response because the response body must be closed
func (p *Provider) AssertEventualCurlResponse(
	ctx context.Context,
	podOpts kubectl.PodExecOptions,
	curlOptions []curl.Option,
	expectedResponse *matchers.HttpResponse,
	timeout ...time.Duration,
) {
	resp := p.AssertEventualCurlReturnResponse(ctx, podOpts, curlOptions, expectedResponse, timeout...)
	resp.Body.Close()
}

func (p *Provider) AssertCurlReturnResponse(
	ctx context.Context,
	podOpts kubectl.PodExecOptions,
	curlOptions []curl.Option,
	expectedResponse *matchers.HttpResponse,
) *http.Response {
	// We rely on the curlPod to execute a curl, therefore we must assert that it actually exists
	p.EventuallyObjectsExist(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podOpts.Name, Namespace: podOpts.Namespace,
		},
	})

	// Rely on default timeouts set in CurlFromPod
	curlResponse, err := p.clusterContext.Cli.CurlFromPod(ctx, podOpts, curlOptions...)
	fmt.Printf("want:\n%+v\nstdout:\n%s\nstderr:%s\n\n", expectedResponse, curlResponse.StdOut, curlResponse.StdErr)
	Expect(err).NotTo(HaveOccurred())

	// Do the transform in a separate step instead of a WithTransform to avoid having to do it twice
	curlHttpResponse := transforms.WithCurlResponse(curlResponse)
	Expect(curlHttpResponse).To(matchers.HaveHttpResponse(expectedResponse))
	fmt.Printf("success: %+v", curlResponse)

	return curlHttpResponse
}

func (p *Provider) AssertCurlResponse(
	ctx context.Context,
	podOpts kubectl.PodExecOptions,
	curlOptions []curl.Option,
	expectedResponse *matchers.HttpResponse,
) {
	resp := p.AssertCurlReturnResponse(ctx, podOpts, curlOptions, expectedResponse)
	resp.Body.Close()
}

// AssertEventuallyConsistentCurlResponse asserts that the response from a curl command
// eventually and then consistently matches the expected response
func (p *Provider) AssertEventuallyConsistentCurlResponse(
	ctx context.Context,
	podOpts kubectl.PodExecOptions,
	curlOptions []curl.Option,
	expectedResponse *matchers.HttpResponse,
	timeout ...time.Duration,
) {
	p.AssertEventualCurlResponse(ctx, podOpts, curlOptions, expectedResponse)

	pollTimeout := 3 * time.Second
	pollInterval := 1 * time.Second
	if len(timeout) > 0 {
		pollTimeout, pollInterval = helper.GetTimeouts(timeout...)
	}

	p.Gomega.Consistently(func(g Gomega) {
		res, err := p.clusterContext.Cli.CurlFromPod(ctx, podOpts, curlOptions...)
		fmt.Printf("want:\n%+v\nstdout:\n%s\nstderr:%s\n\n", expectedResponse, res.StdOut, res.StdErr)
		g.Expect(err).NotTo(HaveOccurred())

		expectedResponseMatcher := WithTransform(transforms.WithCurlResponse, matchers.HaveHttpResponse(expectedResponse))
		g.Expect(res).To(expectedResponseMatcher)
		fmt.Printf("success: %+v", res)
	}).
		WithTimeout(pollTimeout).
		WithPolling(pollInterval).
		WithContext(ctx).
		Should(Succeed())
}
