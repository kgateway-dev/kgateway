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
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	"google.golang.org/protobuf/types/known/wrapperspb"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("Connection Limit", func() {

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

		It("Should not drop any connections", func() {
			injectFaultDelay(testContext)

			var wg sync.WaitGroup
			httpClient := testutils.DefaultClientBuilder().WithTimeout(time.Second * 10).Build()
			requestBuilder := testContext.GetHttpRequestBuilder()

			expectSuccess := func() {
				defer GinkgoRecover()
				defer wg.Done()
				_, err := httpClient.Do(requestBuilder.Build())
				Expect(err).To(BeNil(), "The connection should not be dropped")
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
		})

		It("Should drop connections after limit is reached", func() {
			injectFaultDelay(testContext)

			var wg sync.WaitGroup
			httpClient := testutils.DefaultClientBuilder().WithTimeout(time.Second * 10).Build()
			requestBuilder := testContext.GetHttpRequestBuilder()

			expectSuccess := func() {
				defer GinkgoRecover()
				defer wg.Done()
				_, err := httpClient.Do(requestBuilder.Build())
				Expect(err).ToNot(HaveOccurred(), "The connection should not be dropped")
			}

			expectTimeout := func() {
				defer GinkgoRecover()
				defer wg.Done()
				_, err := httpClient.Do(requestBuilder.Build())
				Expect(err).Should(MatchError(ContainSubstring("EOF")), "The connection should close")
			}

			wg.Add(2)

			go expectSuccess()
			time.Sleep(1 * time.Second)
			go expectTimeout()

			wg.Wait()
		})
	})
})

func injectFaultDelay(testContext *e2e.TestContext) {
	// Since we are testing concurrent connections, introducing a delay to ensure that a connection remains open while we attempt to open another one
	testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
		vs.GetVirtualHost().GetRoutes()[0].Options = &gloov1.RouteOptions{
			Faults: &fault.RouteFaults{
				Delay: &fault.RouteDelay{
					FixedDelay: prototime.DurationToProto(1 * time.Second),
					Percentage: float32(100),
				},
			},
		}
		return vs
	})

	Eventually(func(g Gomega) {
		cfg, err := testContext.EnvoyInstance().ConfigDump()
		g.Expect(err).NotTo(HaveOccurred())

		g.Expect(cfg).To(ContainSubstring("fixed_delay"))
	}, "5s", ".5s").Should(Succeed())
}
