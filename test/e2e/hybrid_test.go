package e2e_test

import (
	"context"
	"fmt"
	"net/http"

	"github.com/golang/protobuf/ptypes/wrappers"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = FDescribe("Hybrid", func() {

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		envoyInstance *services.EnvoyInstance
		testClients   services.TestClients
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		// Initialize Envoy instance
		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

		// Start Gloo
		testClients = services.RunGlooGatewayUdsFds(ctx, &services.RunOptions{
			NsToWrite: defaults.GlooSystem,
			NsToWatch: []string{"default", defaults.GlooSystem},
			WhatToRun: services.What{
				DisableGateway: true,
				DisableFds:     true,
				DisableUds:     true,
			},
		})

		// Run envoy
		err = envoyInstance.RunWithRoleAndRestXds(services.DefaultProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())

		// Create a hybrid proxy routing to the upstream and wait for it to be accepted
		proxy := getProxyHybrid("default", "proxy", defaults.HttpPort)

		_, err = testClients.ProxyClient.Write(proxy, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return testClients.ProxyClient.Read(proxy.Metadata.Namespace, proxy.Metadata.Name, clients.ReadOpts{})
		})
	})

	AfterEach(func() {
		cancel()

		if envoyInstance != nil {
			_ = envoyInstance.Clean()
		}
	})

	It("works as expected", func() {
		client := &http.Client{}

		req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d/", "localhost", defaults.HttpPort), nil)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() (int, error) {
			resp, err := client.Do(req)
			if err != nil {
				return 0, err
			}
			return resp.StatusCode, nil
		}, "5s", "0.5s").Should(Equal(http.StatusOK))

	})
})

func getProxyHybrid(namespace, name string, envoyPort uint32) *gloov1.Proxy {
	return &gloov1.Proxy{
		Metadata: &core.Metadata{
			Name:      name,
			Namespace: namespace,
		},
		Listeners: []*gloov1.Listener{{
			Name:        "listener",
			BindAddress: "0.0.0.0",
			BindPort:    envoyPort,
			ListenerType: &gloov1.Listener_HybridListener{
				HybridListener: &gloov1.HybridListener{
					MatchedListeners: []*gloov1.MatchedListener{
						{
							ListenerType: &gloov1.MatchedListener_HttpListener{
								HttpListener: &gloov1.HttpListener{
									VirtualHosts: []*gloov1.VirtualHost{
										{
											Name:    "gloo-system.virt1",
											Domains: []string{"*"},
											Options: &gloov1.VirtualHostOptions{},
											Routes: []*gloov1.Route{
												{
													Matchers: []*matchers.Matcher{{
														PathSpecifier: &matchers.Matcher_Prefix{
															Prefix: "/",
														},
													}},
													Options: &gloov1.RouteOptions{
														PrefixRewrite: &wrappers.StringValue{Value: "/"},
													},
													Action: &gloov1.Route_DirectResponseAction{
														DirectResponseAction: &gloov1.DirectResponseAction{
															Status: http.StatusTeapot,
														},
													},
												},
											},
										},
									},
								},
							},
							Matcher: &gloov1.Matcher{
								SourcePrefixRanges: []*v3.CidrRange{
									{
										AddressPrefix: "1.2.3.4",
										PrefixLen: &wrappers.UInt32Value{
											Value: 32,
										},
									},
								},
							},
						},
						{
							ListenerType: &gloov1.MatchedListener_HttpListener{
								HttpListener: &gloov1.HttpListener{

									VirtualHosts: []*gloov1.VirtualHost{
										{
											Name:    "gloo-system.virt2",
											Domains: []string{"*"},
											Options: &gloov1.VirtualHostOptions{},
											Routes: []*gloov1.Route{
												{
													Matchers: []*matchers.Matcher{{
														PathSpecifier: &matchers.Matcher_Prefix{
															Prefix: "/",
														},
													}},
													Options: &gloov1.RouteOptions{
														PrefixRewrite: &wrappers.StringValue{Value: "/"},
													},
													Action: &gloov1.Route_DirectResponseAction{
														DirectResponseAction: &gloov1.DirectResponseAction{
															Status: http.StatusOK,
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}},
	}
}
