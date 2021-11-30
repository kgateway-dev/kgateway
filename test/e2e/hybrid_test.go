package e2e_test

import (
	"context"
	"fmt"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"google.golang.org/grpc"
	"net"
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
		srv           *grpc.Server
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		// Initialize Envoy instance
		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

		// Start custom tcp server and create upstream for it
		srv, err = startTcpServer(8095)
		Expect(err).NotTo(HaveOccurred())

		tcpServerUs := &gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      "tcp",
				Namespace: "default",
			},
			UseHttp2: &wrappers.BoolValue{Value: true},
			UpstreamType: &gloov1.Upstream_Static{
				Static: &static.UpstreamSpec{
					Hosts: []*static.Host{{
						// this is a safe way of referring to localhost
						Addr: envoyInstance.GlooAddr,
						Port: 8095,
					}},
				},
			},
		}
		tcpUsRef := tcpServerUs.Metadata.Ref()

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

		// Create static upstream for tcp server
		_, err = testClients.UpstreamClient.Write(tcpServerUs, clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		// Run envoy
		err = envoyInstance.RunWithRoleAndRestXds(services.DefaultProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())


		// Create a hybrid proxy routing to the upstream and wait for it to be accepted
		proxy := getProxyHybrid("default", "proxy", defaults.HttpPort, tcpUsRef, true)

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

		srv.GracefulStop()
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

func getProxyHybrid(namespace, name string, envoyPort uint32, tcpUpsteam *core.ResourceRef, matchTcp bool) *gloov1.Proxy {
	matcher := &gloov1.Matcher{
		SourcePrefixRanges: []*v3.CidrRange{
			{
				AddressPrefix: "1.2.3.4",
				PrefixLen: &wrappers.UInt32Value{
					Value: 32,
				},
			},
		},
	}

	proxy := &gloov1.Proxy{
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
						{
							ListenerType: &gloov1.MatchedListener_TcpListener{
								TcpListener: &gloov1.TcpListener{
									TcpHosts: []*gloov1.TcpHost{
										{
											Name: "test",
											Destination: &gloov1.TcpHost_TcpAction{
												Destination: &gloov1.TcpHost_TcpAction_Single{
													Single: &gloov1.Destination{
														DestinationType: &gloov1.Destination_Upstream{
															Upstream: tcpUpsteam,
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

	if matchTcp {
		proxy.Listeners[0].GetHybridListener().MatchedListeners[1].Matcher = matcher
	} else {
		proxy.Listeners[0].GetHybridListener().MatchedListeners[0].Matcher = matcher
	}
	return proxy
}

func startTcpServer(port uint) (*grpc.Server, error) {
	srv := grpc.NewServer()

	addr := fmt.Sprintf(":%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	go func() {
		defer GinkgoRecover()
		err := srv.Serve(lis)
		Expect(err).ToNot(HaveOccurred())
	}()
	return srv, nil
}
