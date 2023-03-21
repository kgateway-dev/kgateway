package e2e_test

import (
	"net/http"
	"time"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/solo-io/gloo/test/e2e"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	fault "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
)

var _ = Describe("Fault Injection", func() {

	var (
		testContext *e2e.TestContext
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContext()
		testContext.BeforeEach()
	})

	AfterEach(func() {
		testContext.AfterEach()
	})

	JustBeforeEach(func() {
		testContext.JustBeforeEach()
	})

	JustAfterEach(func() {
		testContext.JustAfterEach()
	})

	Context("Envoy Abort Fault", func() {

		It("works", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vs.GetVirtualHost().GetRoutes()[0].Options = &gloov1.RouteOptions{
					Faults: &fault.RouteFaults{
						Abort: &fault.RouteAbort{
							HttpStatus: uint32(503),
							Percentage: float32(100),
						},
					},
				}
				return vs
			})

			requestBuilder := testContext.GetHttpRequestBuilder()
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).To(matchers.HaveHttpResponse(&matchers.HttpResponse{
					StatusCode: http.StatusServiceUnavailable,
					Body:       "fault filter abort",
				}))
			}, "5s", ".5s").Should(Succeed())
		})
	})

	Context("Envoy Delay Fault", func() {

		It("works", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vs.GetVirtualHost().GetRoutes()[0].Options = &gloov1.RouteOptions{
					Faults: &fault.RouteFaults{
						Delay: &fault.RouteDelay{
							FixedDelay: prototime.DurationToProto(time.Second * 3),
							Percentage: float32(100),
						},
					},
				}
				return vs
			})

			// We need a client with a longer timeout than efault to allow for the fixed delay
			httpClient := testutils.DefaultClientBuilder().WithTimeout(time.Second * 10).Build()
			requestBuilder := testContext.GetHttpRequestBuilder()
			Eventually(func(g Gomega) {
				start := time.Now()
				response, err := httpClient.Do(requestBuilder.Build())
				elapsed := time.Now().Sub(start)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(response).Should(matchers.HaveOkResponse())

				// This test regularly flakes, and the error is usually of the form:
				// "Elapsed time 2.998280684s not longer than delay 3s"
				// There's a small precision issue when communicating with Envoy, so including a small
				// margin of error to eliminate the test flake.
				marginOfError := 100 * time.Millisecond
				g.Expect(elapsed + marginOfError).To(BeNumerically(">", 3*time.Second))
			}, "20s", ".1s").Should(Succeed())

		})
	})
})
