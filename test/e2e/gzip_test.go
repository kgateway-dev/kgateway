package e2e_test

import (
	"bytes"

	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/gomega/transforms"

	"fmt"
	"net/http"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	"github.com/solo-io/gloo/test/e2e"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

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
			// json needs to be longer than default content length to trigger
			jsonStr := `{"value":"Hello, world! It's me. I've been wondering if after all these years you'd like to meet."}`
			Eventually(func(g Gomega) {
				req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s:%d/test", "localhost", defaults.HttpPort), bytes.NewBufferString(jsonStr))
				g.Expect(err).NotTo(HaveOccurred())
				req.Host = e2e.DefaultHost
				req.Header.Set("Accept-Encoding", "gzip")
				req.Header.Set("Content-Type", "application/json")

				res, err := http.DefaultClient.Do(req)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(res).Should(matchers.HaveExactResponseBody(jsonStr))
			}, "5s", ".1s").Should(Succeed())
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
			// json needs to be longer than default content length to trigger
			// len(short json) < 30
			shortJsonStr := `{"value":"Hello, world!"}`
			Eventually(func(g Gomega) {
				req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s:%d/test", "localhost", defaults.HttpPort), bytes.NewBufferString(shortJsonStr))
				g.Expect(err).NotTo(HaveOccurred())
				req.Host = e2e.DefaultHost
				req.Header.Set("Accept-Encoding", "gzip")
				req.Header.Set("Content-Type", "application/json")

				res, err := http.DefaultClient.Do(req)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(res).Should(matchers.HaveExactResponseBody(shortJsonStr))
			}).Should(Succeed())

			// raw json should be compressed
			longJsonStr := `{"value":"Hello, world! It's me. I've been wondering if after all these years you'd like to meet."}`
			Eventually(func(g Gomega) {
				req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s:%d/test", "localhost", defaults.HttpPort), bytes.NewBufferString(longJsonStr))
				g.Expect(err).NotTo(HaveOccurred())
				req.Host = e2e.DefaultHost
				req.Header.Set("Accept-Encoding", "gzip")
				req.Header.Set("Content-Type", "application/json")

				res, err := http.DefaultClient.Do(req)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(res).Should(matchers.HaveHttpResponse(&matchers.HttpResponse{
					StatusCode: http.StatusOK,
					Body:       WithTransform(transforms.WithDecompressorTransform(), Equal(longJsonStr)),
				}))
			}).Should(Succeed())
		})
	})
})
