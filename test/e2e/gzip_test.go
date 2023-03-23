package e2e_test

import (
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/gomega/transforms"
	"github.com/solo-io/gloo/test/testutils"

	"net/http"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	"github.com/solo-io/gloo/test/e2e"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloogzip "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/filter/http/gzip/v2"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("gzip", func() {

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

	Context("filter undefined", func() {

		BeforeEach(func() {
			gw := gatewaydefaults.DefaultGateway(writeNamespace)
			gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
				Gzip: nil,
			}

			testContext.ResourcesToCreate().Gateways = v1.GatewayList{
				gw,
			}
		})

		It("should return uncompressed json", func() {
			jsonStr := `{"value":"Hello, world! It's me. I've been wondering if after all these years you'd like to meet."}`
			jsonRequestBuilder := testContext.GetHttpRequestBuilder().
				WithContentType("application/json").
				WithAcceptEncoding("gzip").
				WithPostBody(jsonStr)
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(jsonRequestBuilder.Build())).Should(matchers.HaveExactResponseBody(jsonStr))
			}, "5s", ".1s").Should(Succeed(), "json shorter than default content length is not compressed")
		})
	})

	Context("filter defined", func() {

		BeforeEach(func() {
			gw := gatewaydefaults.DefaultGateway(writeNamespace)
			gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
				Gzip: &gloogzip.Gzip{
					MemoryLevel: &wrappers.UInt32Value{
						Value: 5,
					},
					CompressionLevel:    gloogzip.Gzip_CompressionLevel_SPEED,
					CompressionStrategy: gloogzip.Gzip_HUFFMAN,
					WindowBits: &wrappers.UInt32Value{
						Value: 12,
					},
				},
			}

			testContext.ResourcesToCreate().Gateways = v1.GatewayList{
				gw,
			}
		})

		It("should return compressed json", func() {
			jsonRequestBuilder := testContext.GetHttpRequestBuilder().
				WithContentType("application/json").
				WithAcceptEncoding("gzip")

			shortJsonStr := `{"value":"Hello, world!"}` // len(short json) < 30
			shortRequestBuilder := jsonRequestBuilder.WithPostBody(shortJsonStr)
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(shortRequestBuilder.Build())).Should(matchers.HaveExactResponseBody(shortJsonStr))
			}).Should(Succeed(), "json shorter than content length should not be compressed")

			longJsonStr := `{"value":"Hello, world! It's me. I've been wondering if after all these years you'd like to meet."}`
			longRequestBuilder := jsonRequestBuilder.WithPostBody(longJsonStr)
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(longRequestBuilder.Build())).Should(matchers.HaveHttpResponse(&matchers.HttpResponse{
					StatusCode: http.StatusOK,
					Body:       WithTransform(transforms.WithDecompressorTransform(), Equal(longJsonStr)),
				}))
			}).Should(Succeed(), "json longer than content length should be compressed")
		})
	})
})
