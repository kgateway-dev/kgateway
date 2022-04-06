package e2e_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

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

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance
		up            *gloov1.Upstream

		writeNamespace = defaults.GlooSystem
	)

	BeforeEach(func() {
		var err error
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()

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

		// write gateways and wait for them to be created
		err = gloohelpers.WriteDefaultGateways(writeNamespace, testClients.GatewayClient)
		Expect(err).NotTo(HaveOccurred(), "Should be able to write default gateways")
		Eventually(func() (gatewayv1.GatewayList, error) {
			return testClients.GatewayClient.List(writeNamespace, clients.ListOpts{})
		}, "10s", "0.1s").Should(HaveLen(2), "Gateways should be present")

		// run envoy
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
		err = envoyInstance.RunWithRoleAndRestXds(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())

		// write a test upstream
		// this is the upstream that will handle requests
		testUs := v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
		up = testUs.Upstream
		_, err = testClients.UpstreamClient.Write(up, clients.WriteOpts{OverwriteExisting: true})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		envoyInstance.Clean()
		cancel()
	})

	checkProxy := func() {
		// ensure the proxy is created
		Eventually(func() (*gloov1.Proxy, error) {
			return testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
		}, "5s", "0.1s").ShouldNot(BeNil())
	}

	checkVirtualService := func(testVs *gatewayv1.VirtualService) {
		Eventually(func() (*gatewayv1.VirtualService, error) {
			return testClients.VirtualServiceClient.Read(testVs.Metadata.GetNamespace(), testVs.Metadata.GetName(), clients.ReadOpts{})
		}, "5s", "0.1s").ShouldNot(BeNil())
	}

	testRequest := func(jsonStr string) string {
		By("Make request")
		responseBody := ""
		EventuallyWithOffset(1, func() error {
			var json = []byte(jsonStr)
			req, err := http.NewRequest("POST", fmt.Sprintf("http://%s:%d/test", "localhost", defaults.HttpPort), bytes.NewBuffer(json))
			if err != nil {
				return err
			}
			req.Header.Set("Accept-Encoding", "gzip")
			req.Header.Set("Content-Type", "application/json")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			p := new(bytes.Buffer)
			if _, err := io.Copy(p, res.Body); err != nil {
				return err
			}
			defer res.Body.Close()
			responseBody = p.String()
			return nil
		}, "10s", ".1s").Should(BeNil())
		return responseBody
	}

	Context("filter undefined", func() {

		JustBeforeEach(func() {
			// write a virtual service so we have a proxy to our test upstream
			testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
			_, err := testClients.VirtualServiceClient.Write(testVs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			checkProxy()
			checkVirtualService(testVs)
		})

		It("should return uncompressed json", func() {
			// json needs to be longer than default content length to trigger
			jsonStr := `{"value":"Hello, world! It's me. I've been wondering if after all these years you'd like to meet."}`
			testReq := testRequest(jsonStr)
			Expect(testReq).Should(Equal(jsonStr))
		})
	})

	Context("filter defined", func() {

		JustBeforeEach(func() {
			gatewayClient := testClients.GatewayClient
			gw, err := gatewayClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
			Expect(err).NotTo(HaveOccurred())

			// build a gzip policy
			gzipPolicy := &gloogzip.Gzip{
				MemoryLevel: &wrappers.UInt32Value{
					Value: 5,
				},
				CompressionLevel:    gloogzip.Gzip_CompressionLevel_SPEED,
				CompressionStrategy: gloogzip.Gzip_HUFFMAN,
				WindowBits: &wrappers.UInt32Value{
					Value: 12,
				},
			}

			// update the listener to include the gzip policy
			httpGateway := gw.GetHttpGateway()
			httpGateway.Options = &gloov1.HttpListenerOptions{
				Gzip: gzipPolicy,
			}
			_, err = gatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())

			// Wait until the gateway is written
			Eventually(func() error {
				readGateway, err := gatewayClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				if err != nil {
					return err
				}
				if readGateway.GetHttpGateway() == nil || readGateway.GetHttpGateway().GetOptions() == nil || readGateway.GetHttpGateway().GetOptions().Gzip == nil {
					return fmt.Errorf("gzip not set, new gateway is not yet written")
				}
				return nil
			}, "15s", "0.5s").ShouldNot(HaveOccurred())

			// write a virtual service so we have a proxy to our test upstream
			testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
			_, err = testClients.VirtualServiceClient.Write(testVs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			checkProxy()
			checkVirtualService(testVs)
		})

		It("should return compressed json", func() {
			// json needs to be longer than default content length to trigger
			// len(short json) < 30
			shortJsonStr := `{"value":"Hello, world!"}`
			testShortReq := testRequest(shortJsonStr)
			Expect(testShortReq).Should(Equal(shortJsonStr))

			// raw json should be compressed
			jsonStr := `{"value":"Hello, world! It's me. I've been wondering if after all these years you'd like to meet."}`
			testReqBody := testRequest(jsonStr)
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
