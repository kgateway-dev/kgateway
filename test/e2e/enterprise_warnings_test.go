package e2e_test

import (
	"context"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	ratelimitpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	gloohelpers "github.com/solo-io/gloo/test/helpers"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = FDescribe("warning when using enterprise features", func() {

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
		err = envoyInstance.RunWithRole(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		// write a test upstream
		// this is the upstream that will handle requests
		testUs := v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
		up = testUs.Upstream
		_, err = testClients.UpstreamClient.Write(up, clients.WriteOpts{OverwriteExisting: true})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if envoyInstance != nil {
			_ = envoyInstance.Clean()
		}
		cancel()
	})

	checkVirtualService := func(testVs *gatewayv1.VirtualService) {
		Eventually(func() (*gatewayv1.VirtualService, error) {
			vhost, err := testClients.VirtualServiceClient.Read(testVs.Metadata.GetNamespace(), testVs.Metadata.GetName(), clients.ReadOpts{})
			Expect(err).To(BeNil())
			return vhost, err
		}, "5s", "0.1s").ShouldNot(BeNil())
	}

	checkProxy := func() {
		// ensure the proxy is created
		Eventually(func() (*gloov1.Proxy, error) {
			return testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
		}, "5s", "0.1s").ShouldNot(BeNil())
	}

	Context("warning on ratelimit config", func() {

		It("Enterprise only warning on proxy", func() {
			// write a virtual service so we have a proxy to our test upstream
			rlConfigs := ratelimitpb.RateLimitConfigRefs{
				Refs: []*ratelimitpb.RateLimitConfigRef{{
					Name:      "myRlConfig",
					Namespace: "gloo-system",
				}},
			}

			testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
			testVs.VirtualHost.Options = &gloov1.VirtualHostOptions{
				RateLimitConfigType: &gloov1.VirtualHostOptions_RateLimitConfigs{
					RateLimitConfigs: &rlConfigs,
				},
			}
			_, err := testClients.VirtualServiceClient.Write(testVs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			checkVirtualService(testVs)
			checkProxy()

			p, err := testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
			Expect(p.GetStatus()).To(MatchRegexp("Could not apply ratelimit plugin - this is an Enterprise feature"))
		})

	})

})
