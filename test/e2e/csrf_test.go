package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/onsi/gomega/types"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloo_config_core "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	csrf "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/csrf/v3"
	gloo_type_matcher "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	glootype "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/v3"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	allowedOrigin   = "allowThisOne.solo.io"
	unAllowedOrigin = "doNot.allowThisOne.solo.io"
)

var (
	invalidOriginResponseMatcher = Equal("Invalid origin")
	validOriginResponseMatcher   = BeEmpty()
)

var _ = Describe("CSRF", func() {

	var (
		err           error
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance
		up            *gloov1.Upstream

		writeNamespace = defaults.GlooSystem
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()

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

	// A safe http method is one that doesn't alter the state of the server (ie read only)
	// A CSRF attack targets state changing requests, so the filter only acts on unsafe methods (ones that change state)
	// This is used to spoof requests from various origins
	buildRequestFromOrigin := func(origin string) func() (string, error) {
		return func() (string, error) {
			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s:%d/test", "localhost", defaults.HttpPort), nil)
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

	getEnvoyStats := func() string {
		By("Get stats")
		envoyStats := ""
		EventuallyWithOffset(1, func() error {
			statsUrl := fmt.Sprintf("http://%s:%d/stats",
				envoyInstance.LocalAddr(),
				envoyInstance.AdminPort)
			r, err := http.Get(statsUrl)
			if err != nil {
				return err
			}
			p := new(bytes.Buffer)
			if _, err := io.Copy(p, r.Body); err != nil {
				return err
			}
			defer r.Body.Close()
			envoyStats = p.String()
			return nil
		}, "10s", ".1s").Should(BeNil())
		return envoyStats
	}

	checkProxy := func() {
		// ensure the proxy and virtual service are created
		Eventually(func() (*gloov1.Proxy, error) {
			p, err := testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
			return p, err
		}, "5s", "0.1s").ShouldNot(BeNil())
	}

	checkVirtualService := func(testVs *gatewayv1.VirtualService) {
		Eventually(func() (*gatewayv1.VirtualService, error) {
			return testClients.VirtualServiceClient.Read(testVs.Metadata.GetNamespace(), testVs.Metadata.GetName(), clients.ReadOpts{})
		}, "5s", "0.1s").ShouldNot(BeNil())
	}

	Context("no filter defined", func() {

		JustBeforeEach(func() {
			// write a virtual service so we have a proxy to our test upstream
			testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
			_, err = testClients.VirtualServiceClient.Write(testVs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			checkProxy()
			checkVirtualService(testVs)
		})

		It("should succeed with allowed origin", func() {
			spoofedRequest := buildRequestFromOrigin(allowedOrigin)
			Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(validOriginResponseMatcher)
		})

		It("should succeed with un-allowed origin", func() {
			spoofedRequest := buildRequestFromOrigin(unAllowedOrigin)
			Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(validOriginResponseMatcher)
		})

	})

	Context("defined on listener", func() {

		Context("only on listener", func() {

			JustBeforeEach(func() {
				gatewayClient := testClients.GatewayClient
				gw, err := gatewayClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				// build a csrf policy
				csrfPolicy := getCsrfPolicyWithFilterEnabled(allowedOrigin)

				// update the listener to include the csrf policy
				httpGateway := gw.GetHttpGateway()
				httpGateway.Options = &gloov1.HttpListenerOptions{
					Csrf: csrfPolicy,
				}
				_, err = gatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())

				// write a virtual service so we have a proxy to our test upstream
				testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
				_, err = testClients.VirtualServiceClient.Write(testVs, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				checkProxy()
				checkVirtualService(testVs)
			})

			It("should succeed with allowed origin", func() {
				spoofedRequest := buildRequestFromOrigin(allowedOrigin)
				Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(validOriginResponseMatcher)

				statistics := getEnvoyStats()
				Expect(statistics).To(matchInvalidRequestEqualTo(0))
				Expect(statistics).To(matchValidRequestEqualTo(1))
			})

			It("should fail with un-allowed origin", func() {
				spoofedRequest := buildRequestFromOrigin(unAllowedOrigin)
				Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(invalidOriginResponseMatcher)

				statistics := getEnvoyStats()
				Expect(statistics).To(matchInvalidRequestEqualTo(1))
				Expect(statistics).To(matchValidRequestEqualTo(0))
			})
		})

		Context("defined on listener with shadow mode config", func() {

			JustBeforeEach(func() {
				gatewayClient := testClients.GatewayClient
				gw, err := gatewayClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				// build a csrf policy
				csrfPolicy := getCsrfPolicyWithShadowEnabled(allowedOrigin)

				// update the listener to include the csrf policy
				httpGateway := gw.GetHttpGateway()
				httpGateway.Options = &gloov1.HttpListenerOptions{
					Csrf: csrfPolicy,
				}
				_, err = gatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())

				// write a virtual service so we have a proxy to our test upstream
				testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
				_, err = testClients.VirtualServiceClient.Write(testVs, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				checkProxy()
				checkVirtualService(testVs)
			})

			It("should succeed with allowed origin, unsafe request", func() {
				spoofedRequest := buildRequestFromOrigin(allowedOrigin)
				Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(validOriginResponseMatcher)
				statistics := getEnvoyStats()
				Expect(statistics).To(matchInvalidRequestEqualTo(0))
				Expect(statistics).To(matchValidRequestEqualTo(1))
			})

			It("should succeed with un-allowed origin and update invalid count", func() {
				spoofedRequest := buildRequestFromOrigin(unAllowedOrigin)
				Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(validOriginResponseMatcher)
				statistics := getEnvoyStats()
				Expect(statistics).To(matchInvalidRequestEqualTo(1))
				Expect(statistics).To(matchValidRequestEqualTo(0))
			})
		})

		Context("defined on listener with filter enabled and shadow mode config", func() {
			JustBeforeEach(func() {
				gatewayClient := testClients.GatewayClient
				gw, err := gatewayClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				// build a csrf policy
				csrfPolicy := getCsrfPolicyWithFilterEnabledAndShadow(allowedOrigin)

				// update the listener to include the csrf policy
				httpGateway := gw.GetHttpGateway()
				httpGateway.Options = &gloov1.HttpListenerOptions{
					Csrf: csrfPolicy,
				}
				_, err = gatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())

				// write a virtual service so we have a proxy to our test upstream
				testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
				_, err = testClients.VirtualServiceClient.Write(testVs, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				checkProxy()
				checkVirtualService(testVs)
			})

			It("should succeed with allowed origin, unsafe request", func() {
				spoofedRequest := buildRequestFromOrigin(allowedOrigin)
				Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(validOriginResponseMatcher)
				statistics := getEnvoyStats()
				Expect(statistics).To(matchInvalidRequestEqualTo(0))
				Expect(statistics).To(matchValidRequestEqualTo(1))
			})

			It("should fail with un-allowed origin and update invalid count", func() {
				// shadow mode is ignored when filter is enabled
				spoofedRequest := buildRequestFromOrigin(unAllowedOrigin)
				Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(invalidOriginResponseMatcher)
				statistics := getEnvoyStats()
				Expect(statistics).To(matchInvalidRequestEqualTo(1))
				Expect(statistics).To(matchValidRequestEqualTo(0))
			})
		})

	})

	Context("enabled on route", func() {

		JustBeforeEach(func() {

			// build a csrf policy
			csrfPolicy := getCsrfPolicyWithFilterEnabled(allowedOrigin)

			// write a virtual service so we have a proxy to our test upstream
			vhClient := testClients.VirtualServiceClient
			testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
			// apply to route
			route := testVs.VirtualHost.Routes[0]
			route.Options = &gloov1.RouteOptions{
				Csrf: csrfPolicy,
			}
			_, err = vhClient.Write(testVs, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())

			checkProxy()
			checkVirtualService(testVs)
		})

		It("should succeed with allowed origin, unsafe request", func() {
			spoofedRequest := buildRequestFromOrigin(allowedOrigin)
			Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(validOriginResponseMatcher)
			statistics := getEnvoyStats()
			Expect(statistics).To(matchInvalidRequestEqualTo(0))
			Expect(statistics).To(matchValidRequestEqualTo(1))
		})

		It("should fail with un-allowed origin", func() {
			spoofedRequest := buildRequestFromOrigin(unAllowedOrigin)
			Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(invalidOriginResponseMatcher)
			statistics := getEnvoyStats()
			Expect(statistics).To(matchInvalidRequestEqualTo(1))
			Expect(statistics).To(matchValidRequestEqualTo(0))
		})

	})

	Context("enabled defined on vhost", func() {

		JustBeforeEach(func() {

			// build a csrf policy
			csrfPolicy := getCsrfPolicyWithFilterEnabled(allowedOrigin)

			// write a virtual service so we have a proxy to our test upstream
			vhClient := testClients.VirtualServiceClient
			testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
			testVs.VirtualHost.Options = &gloov1.VirtualHostOptions{
				Csrf: csrfPolicy,
			}
			_, err = vhClient.Write(testVs, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())

			checkProxy()
			checkVirtualService(testVs)
		})

		It("should succeed with allowed origin, unsafe request", func() {
			spoofedRequest := buildRequestFromOrigin(allowedOrigin)
			Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(validOriginResponseMatcher)
			statistics := getEnvoyStats()
			Expect(statistics).To(matchInvalidRequestEqualTo(0))
			Expect(statistics).To(matchValidRequestEqualTo(1))
		})

		It("should fail with un-allowed origin", func() {
			spoofedRequest := buildRequestFromOrigin(unAllowedOrigin)
			Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(invalidOriginResponseMatcher)
			statistics := getEnvoyStats()
			Expect(statistics).To(matchInvalidRequestEqualTo(1))
			Expect(statistics).To(matchValidRequestEqualTo(0))
		})

	})

	Context("defined on weighted dest", func() {

		JustBeforeEach(func() {

			// build a csrf policy
			csrfPolicy := getCsrfPolicyWithFilterEnabled(allowedOrigin)

			// write a virtual service so we have a proxy to our test upstream
			vhClient := testClients.VirtualServiceClient
			testVs := getTrivialVirtualServiceForUpstreamDest(writeNamespace, up)
			// apply to weighted destination
			route := testVs.VirtualHost.Routes[0]

			dest := route.GetRouteAction().GetMulti().GetDestinations()[0]
			dest.Options = &gloov1.WeightedDestinationOptions{
				Csrf: csrfPolicy,
			}

			_, err = vhClient.Write(testVs, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())

			checkProxy()
			checkVirtualService(testVs)
		})

		It("should succeed with allowed origin, unsafe request", func() {
			spoofedRequest := buildRequestFromOrigin(allowedOrigin)
			Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(validOriginResponseMatcher)
			statistics := getEnvoyStats()
			Expect(statistics).To(matchInvalidRequestEqualTo(0))
			Expect(statistics).To(matchValidRequestEqualTo(1))
		})

		It("should fail with un-allowed origin", func() {
			spoofedRequest := buildRequestFromOrigin(unAllowedOrigin)
			Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(invalidOriginResponseMatcher)
			statistics := getEnvoyStats()
			Expect(statistics).To(matchInvalidRequestEqualTo(1))
			Expect(statistics).To(matchValidRequestEqualTo(0))
		})

	})

	Context("defined on listener and vhost, should use vhost definition", func() {

		JustBeforeEach(func() {
			gatewayClient := testClients.GatewayClient
			gw, err := gatewayClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
			Expect(err).NotTo(HaveOccurred())

			// update the listener to include the csrf policy
			httpGateway := gw.GetHttpGateway()
			httpGateway.Options = &gloov1.HttpListenerOptions{
				Csrf: getCsrfPolicyWithFilterEnabled(unAllowedOrigin),
			}
			_, err = gatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())

			// write a virtual service so we have a proxy to our test upstream
			vhClient := testClients.VirtualServiceClient
			testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
			testVs.VirtualHost.Options = &gloov1.VirtualHostOptions{
				Csrf: getCsrfPolicyWithFilterEnabled(allowedOrigin),
			}
			_, err = vhClient.Write(testVs, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())

			checkProxy()
			checkVirtualService(testVs)
		})

		It("should succeed with allowed origin, unsafe request", func() {
			spoofedRequest := buildRequestFromOrigin(allowedOrigin)
			Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(validOriginResponseMatcher)
			statistics := getEnvoyStats()
			Expect(statistics).To(matchInvalidRequestEqualTo(0))
			Expect(statistics).To(matchValidRequestEqualTo(1))
		})

		It("should fail with un-allowed origin", func() {
			spoofedRequest := buildRequestFromOrigin(unAllowedOrigin)
			Eventually(spoofedRequest, 10*time.Second, 1*time.Second).Should(invalidOriginResponseMatcher)
			statistics := getEnvoyStats()
			Expect(statistics).To(matchInvalidRequestEqualTo(1))
			Expect(statistics).To(matchValidRequestEqualTo(0))
		})

	})

})

func matchValidRequestEqualTo(count int) types.GomegaMatcher {
	return MatchRegexp("http.http.csrf.request_valid: %d", count)
}

func matchInvalidRequestEqualTo(count int) types.GomegaMatcher {
	return MatchRegexp("http.http.csrf.request_invalid: %d", count)
}

func getCsrfPolicyWithFilterEnabled(origin string) *csrf.CsrfPolicy {
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
					Regex: origin,
				},
			},
		}},
	}
}

func getCsrfPolicyWithShadowEnabled(origin string) *csrf.CsrfPolicy {
	return &csrf.CsrfPolicy{
		ShadowEnabled: &gloo_config_core.RuntimeFractionalPercent{
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
					Regex: origin,
				},
			},
		}},
	}
}

func getCsrfPolicyWithFilterEnabledAndShadow(origin string) *csrf.CsrfPolicy {
	return &csrf.CsrfPolicy{
		FilterEnabled: &gloo_config_core.RuntimeFractionalPercent{
			DefaultValue: &glootype.FractionalPercent{
				Numerator:   uint32(100),
				Denominator: glootype.FractionalPercent_HUNDRED,
			},
		},
		ShadowEnabled: &gloo_config_core.RuntimeFractionalPercent{
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
					Regex: origin,
				},
			},
		}},
	}
}

func getTrivialVirtualServiceForUpstreamDest(ns string, up *gloov1.Upstream) *gatewayv1.VirtualService {
	vs := getVirtualServiceMultiDest(ns, up)
	vs.VirtualHost.Routes[0].GetRouteAction().GetMulti().GetDestinations()[0].GetDestination().DestinationType = &gloov1.Destination_Upstream{
		Upstream: up.Metadata.Ref(),
	}
	return vs
}

func getVirtualServiceMultiDest(ns string, up *gloov1.Upstream) *gatewayv1.VirtualService {
	return &gatewayv1.VirtualService{
		Metadata: &core.Metadata{
			Name:      "vs",
			Namespace: ns,
		},
		VirtualHost: &gatewayv1.VirtualHost{
			Domains: []string{"*"},
			Routes: []*gatewayv1.Route{{
				Action: &gatewayv1.Route_RouteAction{
					RouteAction: &gloov1.RouteAction{
						Destination: &gloov1.RouteAction_Multi{
							Multi: &gloov1.MultiDestination{
								Destinations: []*gloov1.WeightedDestination{
									{
										Weight: 1,
										Destination: &gloov1.Destination{

											DestinationType: &gloov1.Destination_Upstream{
												Upstream: up.Metadata.Ref(),
											},
										},
									},
								},
							},
						},
					},
				},
				Matchers: []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/",
						},
						Headers: []*matchers.HeaderMatcher{
							{
								Name:        "this-header-must-not-be-present",
								InvertMatch: true,
							},
						},
					},
				},
			}},
		},
	}
}
