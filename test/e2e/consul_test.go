package e2e_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Consul e2e", func() {

	var (
		ctx                context.Context
		cancel             context.CancelFunc
		testClients        services.TestClients
		containerFactory   *services.EchoContainerFactory
		consulInstance     *services.ConsulInstance
		envoyInstance      *services.EnvoyInstance
		envoyPort          uint32
		svc1Port, svc2Port int
		err                error
	)

	const (
		writeNamespace = defaults.GlooSystem
		container1Name = "consul-svc-1"
		container2Name = "consul-svc-2"
		container1Msg  = "hello from svc-1"
		container2Msg  = "hello from svc-2"
	)

	queryService := func() (string, error) {
		response, err := http.Get(fmt.Sprintf("http://localhost:%d", envoyPort))
		if err != nil {
			return "", err
		}
		//noinspection GoUnhandledErrorResult
		defer response.Body.Close()

		body, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return "", err
		}
		return string(body), nil
	}

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()

		// Start Consul
		consulInstance, err = consulFactory.NewConsulInstance()
		Expect(err).NotTo(HaveOccurred())
		err = consulInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		// Run two simple web applications locally
		containerFactory, err = services.NewEchoContainerFactory()
		Expect(err).NotTo(HaveOccurred())
		svc1Port, err = containerFactory.RunEchoContainer(container1Name, container1Msg)
		Expect(err).NotTo(HaveOccurred())
		svc2Port, err = containerFactory.RunEchoContainer(container2Name, container2Msg)
		Expect(err).NotTo(HaveOccurred())

		// Start Gloo
		consulClient, err := consul.NewConsulWatcher(nil)
		Expect(err).NotTo(HaveOccurred())

		ro := &services.RunOptions{
			NsToWrite: writeNamespace,
			NsToWatch: []string{"default", writeNamespace},
			WhatToRun: services.What{
				DisableGateway: true,
				DisableUds:     true,
				DisableFds:     true,
			},
			ConsulClient: consulClient,
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)

		// Start Envoy
		envoyPort = uint32(defaults.HttpPort)
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
		err = envoyInstance.RunWithRole(writeNamespace+"~"+translator.GatewayProxyName, testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		// Register services with consul
		err = consulInstance.RegisterService("my-svc", "my-svc-1", envoyInstance.GlooAddr, []string{"svc", "1"}, svc1Port)
		Expect(err).NotTo(HaveOccurred())
		err = consulInstance.RegisterService("my-svc", "my-svc-2", envoyInstance.GlooAddr, []string{"svc", "1"}, svc2Port)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if consulInstance != nil {
			err = consulInstance.Clean()
			Expect(err).NotTo(HaveOccurred())
		}
		if envoyInstance != nil {
			err = envoyInstance.Clean()
			Expect(err).NotTo(HaveOccurred())
		}
		if containerFactory != nil {
			err = containerFactory.CleanUp()
			Expect(err).NotTo(HaveOccurred())
		}

		cancel()
	})

	It("works as expected", func() {
		_, err := testClients.ProxyClient.Write(getProxyWithConsulRoute(writeNamespace, envoyPort), clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		// Wait for proxy to be accepted
		var proxy *gloov1.Proxy
		Eventually(func() bool {
			proxy, err = testClients.ProxyClient.Read(writeNamespace, "gateway-proxy", clients.ReadOpts{Ctx: ctx})
			if err != nil {
				return false
			}
			return proxy.Status.State == core.Status_Accepted
		}, "10s", "0.2s").Should(BeTrue())

		By("requests are load balanced between the two services")
		Eventually(func() (string, error) {
			return queryService()
		}, "10s", "0.2s").Should(ContainSubstring(container1Msg))

		Eventually(func() (string, error) {
			return queryService()
		}, "10s", "0.2s").Should(ContainSubstring(container2Msg))

		By("update consul service definition")
		err = consulInstance.RegisterService("my-svc", "my-svc-2", envoyInstance.GlooAddr, []string{"svc", "2"}, svc2Port)
		Expect(err).NotTo(HaveOccurred())

		// Wait a bit for the new endpoint information to propagate
		time.Sleep(3 * time.Second)

		// Service 2 does not match the tags on the route anymore, so we should get only requests from service 1
		Consistently(func() (string, error) {
			return queryService()
		}, "2s", "0.2s").Should(ContainSubstring(container1Msg))
	})
})

func getProxyWithConsulRoute(ns string, bindPort uint32) *gloov1.Proxy {
	return &gloov1.Proxy{
		Metadata: core.Metadata{
			Name:      "gateway-proxy",
			Namespace: ns,
		},
		Listeners: []*gloov1.Listener{{
			Name:        "listener",
			BindAddress: "::",
			BindPort:    bindPort,
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					VirtualHosts: []*gloov1.VirtualHost{{
						Name:    "vh-1",
						Domains: []string{"*"},
						Routes: []*gloov1.Route{{
							Matcher: &gloov1.Matcher{
								PathSpecifier: &gloov1.Matcher_Prefix{
									Prefix: "/",
								},
							},
							Action: &gloov1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: &gloov1.Destination{
											DestinationType: &gloov1.Destination_Consul{
												Consul: &gloov1.ConsulServiceDestination{
													ServiceName: "my-svc",
													Tags:        []string{"svc", "1"},
												},
											},
										},
									},
								},
							},
						}},
					}},
				},
			},
		}},
	}
}
