package e2e_test

import (
	"sync"
	"time"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/connection_limit"
	fault "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/gomega/matchers"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	"google.golang.org/protobuf/types/known/wrapperspb"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Connection Limit", func() {

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

	Context("Filter not defined", func() {

		BeforeEach(func() {
			injectRouteFaultDelay(testContext)
		})

		It("Should not drop any connections", func() {
			var wg sync.WaitGroup
			httpClient := testutils.DefaultClientBuilder().WithTimeout(time.Second * 10).Build()
			requestBuilder := testContext.GetHttpRequestBuilder()

			expectSuccess := func() {
				defer GinkgoRecover()
				defer wg.Done()
				response, err := httpClient.Do(requestBuilder.Build())
				Expect(response).Should(matchers.HaveOkResponse())
				Expect(err).NotTo(HaveOccurred(), "The connection should not be dropped")
			}

			wg.Add(2)

			go expectSuccess()
			go expectSuccess()

			wg.Wait()
		})
	})

	Context("Filter defined", func() {

		BeforeEach(func() {
			gw := gatewaydefaults.DefaultGateway(writeNamespace)
			gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
				ConnectionLimit: &connection_limit.ConnectionLimit{
					MaxActiveConnections: &wrapperspb.UInt64Value{
						Value: 1,
					},
				},
			}
			testContext.ResourcesToCreate().Gateways = v1.GatewayList{
				gw,
			}

			injectRouteFaultDelay(testContext)
		})

		// This test has flaked before with the following envoy error :
		// [error][envoy_bug] [external/envoy/source/common/http/conn_manager_impl.cc:527] envoy bug failure: !local_close_reason.empty(). Details: Local Close Reason was not set!
		// This has been fixed in envoy v1.27.0 but since we still use v1.26.x, this issue intermittently occurs.
		// Since this bug doesn't affect the functionally of the ConnectionLimit filter (requests still do not cross the limit), we're adding FlakeAttempts until we move to a version of envoy with this fix.
		// More info can be found here : https://github.com/solo-io/gloo/issues/8531
		It("Should drop connections after limit is reached", FlakeAttempts(5), func() {
			var wg sync.WaitGroup
			httpClient := testutils.DefaultClientBuilder().WithTimeout(time.Second * 10).Build()
			requestBuilder := testContext.GetHttpRequestBuilder()

			expectSuccess := func() {
				defer GinkgoRecover()
				defer wg.Done()
				response, err := httpClient.Do(requestBuilder.Build())
				Expect(response).Should(matchers.HaveOkResponse())
				Expect(err).NotTo(HaveOccurred(), "The connection should not be dropped")
			}

			expectTimeout := func() {
				defer GinkgoRecover()
				defer wg.Done()
				_, err := httpClient.Do(requestBuilder.Build())
				Expect(err).Should(MatchError(ContainSubstring("EOF")), "The connection should close")
			}

			wg.Add(2)

			go expectSuccess()
			// Since we're sending requests concurrently to test the limits on active connections,
			// it is sometimes flaky and the second request gets served first.
			// That's why we're adding a delay between the first and second one
			time.Sleep(100 * time.Millisecond)
			go expectTimeout()

			wg.Wait()
		})
	})
})

func injectRouteFaultDelay(testContext *e2e.TestContext) {
	// Since we are testing concurrent connections, introducing a delay to ensure that a connection remains open while we attempt to open another one
	vs := gloohelpers.NewVirtualServiceBuilder().
		WithNamespace(writeNamespace).
		WithName(e2e.DefaultVirtualServiceName).
		WithDomain(e2e.DefaultHost).
		WithRoutePrefixMatcher("route", "/").
		WithRouteActionToUpstream("route", testContext.TestUpstream().Upstream).
		WithRouteOptions("route", &gloov1.RouteOptions{
			Faults: &fault.RouteFaults{
				Delay: &fault.RouteDelay{
					FixedDelay: prototime.DurationToProto(time.Second * 1),
					Percentage: float32(100),
				},
			},
		}).
		Build()
	testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{
		vs,
	}
}
