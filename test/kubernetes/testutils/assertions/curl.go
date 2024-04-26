package assertions

import (
	"context"
	"fmt"
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

func (p *Provider) AssertEventualCurlResponse(
	ctx context.Context,
	podOpts kubectl.PodExecOptions,
	curlOptions []curl.Option,
	expectedResponse *matchers.HttpResponse,
	timeout ...time.Duration,
) {
	// We rely on the curlPod to execute a curl, therefore we must assert that it actually exists
	p.EventuallyObjectsExist(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podOpts.Name, Namespace: podOpts.Namespace,
		},
	})

	currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)

	p.Gomega.Eventually(func(g Gomega) {
		res, err := p.clusterContext.Cli.CurlFromPod(ctx, podOpts, curlOptions...)
		g.Expect(err).NotTo(HaveOccurred())
		fmt.Printf("want:\n%+v\nhave:\n%s\n\n", expectedResponse, res)

		expectedResponseMatcher := WithTransform(transforms.WithCurlHttpResponse, matchers.HaveHttpResponse(expectedResponse))
		g.Expect(res).To(expectedResponseMatcher)
		fmt.Printf("success: %v", res)
	}).
		WithTimeout(currentTimeout).
		WithPolling(pollingInterval).
		WithContext(ctx).
		Should(Succeed())
}
