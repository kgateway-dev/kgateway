package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = FDescribe("Validation", func() {

	var (
		ctx            context.Context
		cancel         context.CancelFunc
		testClients    services.TestClients
		writeNamespace string
	)

	Describe("in memory", func() {

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			defaults.HttpPort = services.NextBindPort()
			defaults.HttpsPort = services.NextBindPort()

			writeNamespace = "gloo-system"
			ro := &services.RunOptions{
				NsToWrite: writeNamespace,
				NsToWatch: []string{"default", writeNamespace},
				WhatToRun: services.What{
					DisableFds: false,
					DisableUds: false,
				},
				ValidationPort: 9988,
			}

			testClients = services.RunGlooGatewayUdsFds(ctx, ro)
			err := gloohelpers.WriteDefaultGateways(writeNamespace, testClients.GatewayClient)
			Expect(err).NotTo(HaveOccurred(), "Should be able to write default gateways")

			// wait for the two gateways to be created.
			Eventually(func() (gatewayv1.GatewayList, error) {
				return testClients.GatewayClient.List(writeNamespace, clients.ListOpts{})
			}, "10s", "0.1s").Should(HaveLen(2), "Gateways should be present")
		})

		AfterEach(func() {
			cancel()
		})

		Context("gloo and gateway", func() {

			var (
				envoyInstance *services.EnvoyInstance
				tu            *v1helpers.TestUpstream
			)

			TestUpstreamReachable := func() {
				v1helpers.TestUpstreamReachable(defaults.HttpPort, tu, nil)
			}

			BeforeEach(func() {
				ctx, cancel = context.WithCancel(context.Background())
				var err error
				envoyInstance, err = envoyFactory.NewEnvoyInstance()
				Expect(err).NotTo(HaveOccurred())

				tu = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

				_, err = testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				err = envoyInstance.RunWithRole(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort)
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				if envoyInstance != nil {
					_ = envoyInstance.Clean()
				}
			})

			FIt("validation server doesn't drop notifications", func() {
				up := tu.Upstream
				vs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
				_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				time.Sleep(5 * time.Second)

				p, err := testClients.ProxyClient.List(writeNamespace, clients.ListOpts{})
				Expect(err).NotTo(HaveOccurred())
				Expect(p).To(BeNil())
			})

			It("should work with no ssl and cleans up the envoy config when the virtual service is deleted", func() {
				up := tu.Upstream
				vs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
				_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				TestUpstreamReachable()

				// Delete the Virtual Service
				err = testClients.VirtualServiceClient.Delete(writeNamespace, vs.GetMetadata().Name, clients.DeleteOpts{})
				Expect(err).NotTo(HaveOccurred())

				// Wait for proxy to be deleted
				var proxyList gloov1.ProxyList
				Eventually(func() bool {
					proxyList, err = testClients.ProxyClient.List(writeNamespace, clients.ListOpts{})
					if err != nil {
						return false
					}
					return len(proxyList) == 0
				}, "10s", "0.1s").Should(BeTrue())

				// Create a regular request
				request, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d", defaults.HttpPort), nil)
				Expect(err).NotTo(HaveOccurred())
				request = request.WithContext(ctx)

				// Check that we can no longer reach the upstream
				client := &http.Client{}
				Eventually(func() int {
					response, err := client.Do(request)
					if err != nil {
						return 503
					}
					return response.StatusCode
				}, 20*time.Second, 500*time.Millisecond).Should(Equal(503))
			})
		})
	})
})
