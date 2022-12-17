package e2e_test

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	"github.com/solo-io/gloo/test/e2e"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	. "github.com/onsi/ginkgo"
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
			testReq := executePostRequest("test.com", jsonStr)
			Expect(testReq).Should(Equal(jsonStr))
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
			testShortReq := executePostRequest("test.com", shortJsonStr)
			Expect(testShortReq).Should(Equal(shortJsonStr))

			// raw json should be compressed
			jsonStr := `{"value":"Hello, world! It's me. I've been wondering if after all these years you'd like to meet."}`
			testReqBody := executePostRequest("test.com", jsonStr)
			Expect(testReqBody).ShouldNot(Equal(jsonStr))

			// decompressed json from response should equal original
			reader, err := gzip.NewReader(bytes.NewBuffer([]byte(testReqBody)))
			defer reader.Close()
			Expect(err).NotTo(HaveOccurred())
			body, err := ioutil.ReadAll(reader)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal(jsonStr))
		})
	})
})

func executePostRequest(host, jsonStr string) string {
	By("Make request")
	responseBody := ""
	EventuallyWithOffset(1, func(g Gomega) error {
		var json = []byte(jsonStr)
		req, err := http.NewRequest("POST", fmt.Sprintf("http://%s:%d/test", "localhost", defaults.HttpPort), bytes.NewBuffer(json))
		g.ExpectWithOffset(1, err).NotTo(HaveOccurred())
		req.Host = host
		req.Header.Set("Accept-Encoding", "gzip")
		req.Header.Set("Content-Type", "application/json")

		res, err := http.DefaultClient.Do(req)
		g.ExpectWithOffset(1, err).NotTo(HaveOccurred())
		g.ExpectWithOffset(1, res.StatusCode).Should(Equal(http.StatusOK))

		p := new(bytes.Buffer)
		_, err = io.Copy(p, res.Body)
		g.ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
		defer res.Body.Close()
		responseBody = p.String()
		return nil
	}, "10s", ".1s").Should(Succeed())
	return responseBody
}
