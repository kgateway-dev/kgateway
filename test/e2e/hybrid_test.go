package e2e_test

import (
	"context"
	"fmt"
	"net/http"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	matchers2 "github.com/solo-io/gloo/test/matchers"

	"github.com/golang/protobuf/ptypes/wrappers"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("Hybrid Gateway", func() {

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		envoyInstance *services.EnvoyInstance
		testClients   services.TestClients

		resourcesToCreate *gloosnapshot.ApiSnapshot
		writeNamespace    = defaults.GlooSystem
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		// Run gloo
		ro := &services.RunOptions{
			NsToWrite: writeNamespace,
			NsToWatch: []string{"default", writeNamespace},
			WhatToRun: services.What{
				DisableFds: true,
				DisableUds: true,
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)

		// Run Envoy
		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
		role := fmt.Sprintf("%s~%s", writeNamespace, gatewaydefaults.GatewayProxyName)
		err = envoyInstance.RunWithRole(role, testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		vsToTestUpstream := helpers.NewVirtualServiceBuilder().
			WithName("vs-test").
			WithNamespace(writeNamespace).
			WithDomain("test.com").
			WithRoutePrefixMatcher("test", "/").
			WithRouteDirectResponseAction("test", &gloov1.DirectResponseAction{
				Status: http.StatusOK,
			}).
			Build()

		// The set of resources that these tests will generate
		resourcesToCreate = &gloosnapshot.ApiSnapshot{
			Gateways: v1.GatewayList{
				// Let each test create the appropriate Gateway
			},
			VirtualServices: v1.VirtualServiceList{
				vsToTestUpstream,
			},
		}
	})

	AfterEach(func() {
		// Stop Envoy
		envoyInstance.Clean()

		cancel()
	})

	JustBeforeEach(func() {
		// Create Resources
		err := testClients.WriteSnapshot(ctx, resourcesToCreate)
		Expect(err).NotTo(HaveOccurred())

		// Wait for a proxy to be accepted
		helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
		})
	})

	JustAfterEach(func() {
		// We do not need to clean up the Snapshot that was written in the JustBeforeEach
		// That is because each test uses its own InMemoryCache
	})

	buildHttpRequestToHybridGateway := func() *http.Request {
		req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d/", "localhost", defaults.HybridPort), nil)
		Expect(err).NotTo(HaveOccurred())
		req.Host = "test.com" // to match the vs-test

		return req
	}

	Context("catchall match for http", func() {

		BeforeEach(func() {
			gw := gatewaydefaults.DefaultHybridGateway(writeNamespace)

			gw.GetHybridGateway().MatchedGateways = []*v1.MatchedGateway{
				// HttpGateway gets a catchall matcher
				{
					GatewayType: &v1.MatchedGateway_HttpGateway{
						HttpGateway: &v1.HttpGateway{},
					},
				},

				// TcpGateway gets a matcher our request *will not* hit
				{
					Matcher: &v1.Matcher{
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "1.2.3.4",
								PrefixLen: &wrappers.UInt32Value{
									Value: 32,
								},
							},
						},
					},
					GatewayType: &v1.MatchedGateway_TcpGateway{
						TcpGateway: &v1.TcpGateway{},
					},
				},
			}

			resourcesToCreate.Gateways = v1.GatewayList{
				gw,
			}
		})

		It("http request works as expected", func() {
			client := &http.Client{}
			req := buildHttpRequestToHybridGateway()

			Eventually(func() (*http.Response, error) {
				return client.Do(req)
			}, "5s", "0.5s").Should(matchers2.MatchHttpResponse(&http.Response{
				StatusCode: http.StatusOK,
			}))
		})

	})

	Context("SourcePrefixRanges match for http", func() {

		BeforeEach(func() {
			gw := gatewaydefaults.DefaultHybridGateway(writeNamespace)

			gw.GetHybridGateway().MatchedGateways = []*v1.MatchedGateway{
				// HttpGateway gets a matcher our request will hit
				{
					Matcher: &v1.Matcher{
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "255.0.0.0",
								PrefixLen: &wrappers.UInt32Value{
									Value: 1,
								},
							},
							{
								AddressPrefix: "0.0.0.0",
								PrefixLen: &wrappers.UInt32Value{
									Value: 1,
								},
							},
						},
					},
					GatewayType: &v1.MatchedGateway_HttpGateway{
						HttpGateway: &v1.HttpGateway{},
					},
				},
			}

			resourcesToCreate.Gateways = v1.GatewayList{
				gw,
			}
		})

		It("http request works as expected", func() {
			client := &http.Client{}
			req := buildHttpRequestToHybridGateway()

			Eventually(func() (*http.Response, error) {
				return client.Do(req)
			}, "5s", "0.5s").Should(matchers2.MatchHttpResponse(&http.Response{
				StatusCode: http.StatusOK,
			}))

		})

	})

	Context("SourcePrefixRanges miss for tcp", func() {

		BeforeEach(func() {
			gw := gatewaydefaults.DefaultHybridGateway(writeNamespace)

			gw.GetHybridGateway().MatchedGateways = []*v1.MatchedGateway{
				// HttpGateway gets a filter our request *will not* hit
				{
					Matcher: &v1.Matcher{
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "1.2.3.4",
								PrefixLen: &wrappers.UInt32Value{
									Value: 32,
								},
							},
						},
					},
					GatewayType: &v1.MatchedGateway_HttpGateway{
						HttpGateway: &v1.HttpGateway{},
					},
				},
			}

			resourcesToCreate.Gateways = v1.GatewayList{
				gw,
			}
		})

		It("http request fails", func() {
			client := &http.Client{}
			req := buildHttpRequestToHybridGateway()

			Consistently(func() error {
				_, err := client.Do(req)
				return err
			}, "3s", "0.5s").Should(HaveOccurred())

		})

	})

})
