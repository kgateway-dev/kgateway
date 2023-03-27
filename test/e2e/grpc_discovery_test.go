package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/gloo/test/e2e"

	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

var _ = Describe("GRPC to JSON Transcoding Plugin - Discovery", func() {

	var (
		testContext *e2e.TestContext
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContext(testutils.LinuxOnly("Relies on FDS"))
		testContext.SetUpstreamGenerator(func(ctx context.Context, addr string) *v1helpers.TestUpstream {
			return v1helpers.NewTestGRPCUpstream(ctx, addr, 1)
		})
		testContext.BeforeEach()

		testContext.SetRunServices(services.What{
			DisableGateway: false,
			DisableUds:     true,
			// test relies on FDS to discover the grpc spec via reflection
			DisableFds: false,
		})
		testContext.SetRunSettings(&gloov1.Settings{
			Gloo: &gloov1.GlooOptions{
				// https://github.com/solo-io/gloo/issues/7577
				RemoveUnusedFilters: &wrappers.BoolValue{Value: false},
			},
			Discovery: &gloov1.Settings_DiscoveryOptions{
				FdsMode: gloov1.Settings_DiscoveryOptions_BLACKLIST,
			},
		})
	})
	JustBeforeEach(func() {
		testContext.JustBeforeEach()
	})
	JustAfterEach(func() {
		testContext.JustAfterEach()
	})
	AfterEach(func() {
		testContext.AfterEach()
	})

	basicReq := func(body string, expected string) func(g Gomega) {
		return func(g Gomega) {
			// send a POST request with grpc parameters in the body
			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s:%d/test", "localhost", defaults.HttpPort), bytes.NewBufferString(body))
			g.Expect(err).NotTo(HaveOccurred())
			req.Host = e2e.DefaultHost
			g.Expect(testutils.DefaultHttpClient.Do(req)).Should(testmatchers.HaveExactResponseBody(expected))
		}
	}

	It("Routes to GRPC Functions", func() {

		body := `"foo"`
		testRequest := basicReq(body, `{"str":"foo"}`)

		Eventually(testRequest, 30, 1).Should(Succeed())

		Eventually(testContext.TestUpstream().C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
			"GRPCRequest": PointTo(MatchFields(IgnoreExtras, Fields{"Str": Equal("foo")})),
		}))))
	})

	It("Routes to GRPC Functions with parameters in URL", func() {

		testRequest := func(g Gomega) {
			// GET request with parameters in URL
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:%d/t/foo", "localhost", defaults.HttpPort), nil)
			g.Expect(err).NotTo(HaveOccurred())
			req.Host = e2e.DefaultHost
			g.Expect(testutils.DefaultHttpClient.Do(req)).Should(testmatchers.HaveExactResponseBody(`{"str":"foo"}`))
		}
		Eventually(testRequest, 30, 1).Should(Succeed())
		Eventually(testContext.TestUpstream().C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
			"GRPCRequest": PointTo(MatchFields(IgnoreExtras, Fields{"Str": Equal("foo")})),
		}))))
	})
})
