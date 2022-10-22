package e2e_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	"github.com/golang/protobuf/ptypes/wrappers"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloohelpers "github.com/solo-io/gloo/test/helpers"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloogzip "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/filter/http/gzip/v2"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("gzip", func() {

	// Re-run CI (Attempt #4)

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		envoyInstance *services.EnvoyInstance

		testClients  services.TestClients
		testUpstream *v1helpers.TestUpstream

		resourcesToCreate *gloosnapshot.ApiSnapshot
		writeNamespace    = defaults.GlooSystem
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		// run gloo
		ro := &services.RunOptions{
			NsToWrite: writeNamespace,
			NsToWatch: []string{"default", writeNamespace},
			WhatToRun: services.What{
				DisableFds: true,
				DisableUds: true,
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)

		// run envoy
		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
		err = envoyInstance.RunWithRole(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		// this is the upstream that will handle requests
		testUpstream = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

		vsToTestUpstream := gloohelpers.NewVirtualServiceBuilder().
			WithName("vs-test").
			WithNamespace(writeNamespace).
			WithDomain("test.com").
			WithRoutePrefixMatcher("test", "/").
			WithRouteActionToUpstream("test", testUpstream.Upstream).
			Build()

		// The set of resources that these tests will generate
		resourcesToCreate = &gloosnapshot.ApiSnapshot{
			Gateways: gatewayv1.GatewayList{
				gatewaydefaults.DefaultGateway(writeNamespace),
			},
			VirtualServices: gatewayv1.VirtualServiceList{
				vsToTestUpstream,
			},
			Upstreams: gloov1.UpstreamList{
				testUpstream.Upstream,
			},
		}
	})

	AfterEach(func() {
		envoyInstance.Clean()
		cancel()
	})

	JustBeforeEach(func() {
		// Create Resources
		err := testClients.WriteSnapshot(ctx, resourcesToCreate)
		Expect(err).NotTo(HaveOccurred())

		// Wait for a proxy to be accepted
		gloohelpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
		})

		// Ensure the testUpstream is reachable
		v1helpers.ExpectCurlWithOffset(
			0,
			v1helpers.CurlRequest{
				RootCA: nil,
				Port:   defaults.HttpPort,
				Host:   "test.com", // to match the vs-test
				Path:   "/",
				Body:   []byte("solo.io test"),
			},
			v1helpers.CurlResponse{
				Status:  http.StatusOK,
				Message: "",
			},
		)
	})

	JustAfterEach(func() {
		// We do not need to clean up the Snapshot that was written in the JustBeforeEach
		// That is because each test uses its own InMemoryCache
	})

	Context("filter undefined", func() {

		BeforeEach(func() {
			resourcesToCreate.Gateways[0].GetHttpGateway().Options = &gloov1.HttpListenerOptions{
				Gzip: nil,
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
			resourcesToCreate.Gateways[0].GetHttpGateway().Options = &gloov1.HttpListenerOptions{
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
