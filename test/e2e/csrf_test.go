package e2e_test

import (
	"context"
	"fmt"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloo_config_core "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	gloo_type_matcher "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	glootype "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/v3"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"io/ioutil"
	"net/http"
	"time"

	csrf "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/csrf/v3"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)


const (
	allowedOriginRegex = "allowThisOne.solo.io"
	unAllowedOriginRegex = "doNot.allowThisOne.solo.io"
)

var _ = Describe("CSRF", func() {

	var (
		err           error
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance
		testUs        *v1helpers.TestUpstream

		writeNamespace = defaults.GlooSystem
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()

		// run gloo
		writeNamespace = defaults.GlooSystem
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
		testUs = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
		_, err = testClients.UpstreamClient.Write(testUs.Upstream, clients.WriteOpts{OverwriteExisting: true})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if envoyInstance != nil {
			_ = envoyInstance.Clean()
		}
		cancel()
	})

	// A safe http method is one that doesn't alter the state of the server (ie read only)
	// A CSRF attack targets state changing requests, so the filter only acts on unsafe methods (ones that change state)
	// This is used to spoof requests from various origins
	buildRequestFromOrigin := func(origin string, safeRequest bool) func() (string, error) {
		return func() (string, error) {
			method := "POST"
			if safeRequest {
				method = "GET"
			}
			req, err := http.NewRequest(method, fmt.Sprintf("http://%s:%d/test", "localhost", defaults.HttpPort), nil)
			if err != nil {
				return "", err
			}
			req.Header.Set("Origin", origin)

			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return "", err
			}
			defer res.Body.Close()
			body, err := ioutil.ReadAll(res.Body)
			return string(body), err
		}
	}

	Context("defined on listener", func() {

		JustBeforeEach(func() {
			gatewayClient := testClients.GatewayClient
			gw, err := gatewayClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
			Expect(err).NotTo(HaveOccurred())

			// build a csrf policy
			csrfPolicy := getCsrfPolicyWithAllowedRegex()

			// update the listener to include the csrf policy
			httpGateway := gw.GetHttpGateway()
			httpGateway.Options = &gloov1.HttpListenerOptions{
				Csrf: csrfPolicy,
			}
			_, err = gatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())

			// write a virtual service so we have a proxy to our test upstream
			testVs := getTrivialVirtualServiceForUpstream(writeNamespace, testUs.Upstream.Metadata.Ref())
			_, err = testClients.VirtualServiceClient.Write(testVs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			// ensure the proxy and virtual service are created
			Eventually(func() (*gloov1.Proxy, error) {
				return testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
			}, "5s", "0.1s").ShouldNot(BeNil())
			Eventually(func() (*gatewayv1.VirtualService, error) {
				return testClients.VirtualServiceClient.Read(testVs.Metadata.GetNamespace(), testVs.Metadata.GetName(), clients.ReadOpts{})
			}, "5s", "0.1s").ShouldNot(BeNil())
		})

		It("should succeed with allowed origin, safe request", func() {
			spoofedRequest := buildRequestFromOrigin(allowedOriginRegex, true)
			Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(BeEmpty())
		})

		It("should succeed with allowed origin, unsafe request", func() {
			spoofedRequest := buildRequestFromOrigin(allowedOriginRegex, false)
			Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(BeEmpty())
		})

		It("should succeed with un-allowed origin, safe request", func() {
			// confirm that a safe (read only) request is not affected by filter
			spoofedRequest := buildRequestFromOrigin(unAllowedOriginRegex, true)
			Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(BeEmpty())
		})

		It("should fail with un-allowed origin", func() {
			spoofedRequest := buildRequestFromOrigin(unAllowedOriginRegex, false)
			Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(Equal("Invalid origin"))
		})

	})

	// TODO - add support for applying csrf filter to routes and virtual hosts...etc

})

func getCsrfPolicyWithAllowedRegex() *csrf.CsrfPolicy {
	return &csrf.CsrfPolicy{
		FilterEnabled: &gloo_config_core.RuntimeFractionalPercent{
			DefaultValue: &glootype.FractionalPercent{
				Numerator:   uint32(100),
				Denominator: glootype.FractionalPercent_HUNDRED,
			},
		},
		AdditionalOrigins: []*gloo_type_matcher.StringMatcher{{
			MatchPattern: &gloo_type_matcher.StringMatcher_SafeRegex{
				SafeRegex: &gloo_type_matcher.RegexMatcher{
					EngineType: &gloo_type_matcher.RegexMatcher_GoogleRe2{
						GoogleRe2: &gloo_type_matcher.RegexMatcher_GoogleRE2{},
					},
					Regex: allowedOriginRegex,
				},
			},
		}},
	}
}