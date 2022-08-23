package e2e_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/consul/api"
	"github.com/rotisserie/eris"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	consulplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Consul e2e", func() {

	var (
		ctx            context.Context
		cancel         context.CancelFunc
		testClients    services.TestClients
		consulInstance *services.ConsulInstance
		envoyInstance  *services.EnvoyInstance
		envoyPort      uint32
		svc1, svc2     *v1helpers.TestUpstream
		err            error
		consulClient   consul.ConsulWatcher
	)

	const writeNamespace = defaults.GlooSystem

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
		if response.StatusCode != 200 {
			return "", eris.Errorf("bad status code: %v (%v)", response.StatusCode, string(body))
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

		// init consul client
		client, err := api.NewClient(api.DefaultConfig())
		Expect(err).NotTo(HaveOccurred())

		// Start Gloo
		consulClient, err = consul.NewConsulWatcher(client, nil)
		Expect(err).NotTo(HaveOccurred())

		// ro := &services.RunOptions{
		// 	NsToWrite: writeNamespace,
		// 	NsToWatch: []string{"default", writeNamespace},
		// 	WhatToRun: services.What{
		// 		DisableGateway: true,
		// 		DisableUds:     true,
		// 		DisableFds:     true,
		// 	},
		// 	ConsulClient:     consulClient,
		// 	ConsulDnsAddress: consul2.DefaultDnsAddress,
		// }
		// testClients = services.RunGlooGatewayUdsFds(ctx, ro)

		// Start Envoy
		// envoyPort = defaults.HttpPort
		// envoyInstance, err = envoyFactory.NewEnvoyInstance()
		// Expect(err).NotTo(HaveOccurred())
		// envoyInstance.RestXdsPort = uint32(testClients.RestXdsPort)
		// err = envoyInstance.RunWithRoleAndRestXds(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
		// Expect(err).NotTo(HaveOccurred())

		// // Run two simple web applications locally
		// svc1 = v1helpers.NewTestHttpUpstreamWithReply(ctx, envoyInstance.LocalAddr(), "svc-1")
		// svc2 = v1helpers.NewTestHttpUpstreamWithReply(ctx, envoyInstance.LocalAddr(), "svc-2")

		// Register services with consul
		// update service one to point to test upstream 2 port
		// err = consulInstance.LiveUpdateServiceInstance("my-svc", "my-svc-1", envoyInstance.GlooAddr, []string{"svc", "1"}, svc1.Port)
		// Expect(err).NotTo(HaveOccurred())

		// TODO(kdorosh) write e2e with just consul client that proves service updates even if cli watch on services does not...

		// err = consulInstance.LiveUpdateServiceInstance("my-svc", "my-svc-1", envoyInstance.GlooAddr, []string{"svc", "1"}, svc2.Port)
		// Expect(err).NotTo(HaveOccurred())

		// Expect(1).To(Equal(2))
		// err = consulInstance.RegisterService("my-svc", "my-svc-1", envoyInstance.GlooAddr, []string{"svc", "1"}, svc1.Port)
		// Expect(err).NotTo(HaveOccurred())
		// err = consulInstance.RegisterService("my-svc", "my-svc-2", envoyInstance.GlooAddr, []string{"svc", "2"}, svc2.Port)
		// Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if consulInstance != nil {
			err = consulInstance.Clean()
			Expect(err).NotTo(HaveOccurred())
		}
		envoyInstance.Clean()
		cancel()
	})

	It("works as expected", func() {
		_, err := testClients.ProxyClient.Write(getProxyWithConsulRoute(writeNamespace, envoyPort), clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		// Wait for proxy to be accepted
		helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
		})

		By("requests only go to service with tag '1'")

		// Wait for the endpoints to be registered
		Eventually(func() (<-chan *v1helpers.ReceivedRequest, error) {
			_, err := queryService()
			if err != nil {
				return svc1.C, err
			}
			return svc1.C, nil
		}, "20s", "0.2s").Should(Receive())
		// Service 2 does not match the tags on the route, so we should get only requests from service 1
		Consistently(func() (<-chan *v1helpers.ReceivedRequest, error) {
			_, err := queryService()
			if err != nil {
				return svc1.C, err
			}
			return svc1.C, nil
		}, "2s", "0.2s").Should(Receive())

		err = consulInstance.RegisterService("my-svc", "my-svc-2", envoyInstance.GlooAddr, []string{"svc", "1"}, svc2.Port)
		Expect(err).NotTo(HaveOccurred())

		By("requests are load balanced between the two services")

		// svc2 first to ensure we also still route to svc1 after registering svc2
		Eventually(func() (<-chan *v1helpers.ReceivedRequest, error) {
			_, err := queryService()
			if err != nil {
				return svc2.C, err
			}
			return svc2.C, nil
		}, "10s", "0.2s").Should(Receive())
		Eventually(func() (<-chan *v1helpers.ReceivedRequest, error) {
			_, err := queryService()
			if err != nil {
				return svc1.C, err
			}
			return svc1.C, nil
		}, "10s", "0.2s").Should(Receive())

	})

	FIt("resolves eds even if services aren't updated", func() {

		// cfg := api.DefaultConfig()
		// consulClient, err := api.NewClient(cfg)
		// Expect(err).NotTo(HaveOccurred())
		// consulClientWrapper, err := consul.NewConsulWatcher(consulClient, nil) // dcs
		// Expect(err).NotTo(HaveOccurred())

		// TODO(kdorosh) switch to endpoints once reproed
		svcsChan, errChan := consulClient.WatchServices(ctx, []string{"dc1"}, consulplugin.ConsulConsistencyModes_DefaultMode, nil)

		Eventually(func() <-chan []*consul.ServiceMeta {
			return svcsChan
		}, "2s", "0.2s").Should(Receive())

		Consistently(func() <-chan []*consul.ServiceMeta {
			return svcsChan
		}, "2s", "0.2s").ShouldNot(Receive())

		Consistently(func() <-chan error {
			return errChan
		}, "0.5s", "0.2s").ShouldNot(Receive())

		// fmt.Printf("%v", consulClient)

		err = consulInstance.LiveUpdateServiceInstance("my-svc", "my-svc-1", "127.0.0.1", []string{"svc", "1"}, 80)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() <-chan []*consul.ServiceMeta {
			return svcsChan
		}, "2s", "0.2s").Should(Receive())

		Consistently(func() <-chan []*consul.ServiceMeta {
			return svcsChan
		}, "2s", "0.2s").ShouldNot(Receive())

		Consistently(func() <-chan error {
			return errChan
		}, "0.5s", "0.2s").ShouldNot(Receive())

		err = consulInstance.LiveUpdateServiceInstance("my-svc", "my-svc-1", "127.0.0.1", []string{"svc", "1"}, 81)
		Expect(err).NotTo(HaveOccurred())

		// time.Sleep(time.Second * 2)

		// this is where golang client differs from cli!
		Eventually(func() <-chan []*consul.ServiceMeta {
			return svcsChan
		}, "2s", "0.2s").Should(Receive())

		Consistently(func() <-chan []*consul.ServiceMeta {
			return svcsChan
		}, "2s", "0.2s").ShouldNot(Receive())

		Consistently(func() <-chan error {
			return errChan
		}, "0.5s", "0.2s").ShouldNot(Receive())

		// Eventually(func() <-chan []*consul.ServiceMeta {
		// 	return svcsChan
		// }, "5s", "0.2s").ShouldNot(Receive())

		// Consistently(func() <-chan []*consul.ServiceMeta {
		// 	return svcsChan
		// }, "5s", "0.2s").ShouldNot(Receive())

		// new port!
		// err = consulInstance.LiveUpdateServiceInstance("my-svc", "my-svc-1", "127.0.0.1", []string{"svc", "1"}, 81)
		// Expect(err).NotTo(HaveOccurred())

		// Consistently(func() <-chan error {
		// 	return errChan
		// }, "2s", "0.2s").ShouldNot(Receive())

		// Eventually(func() <-chan []*consul.ServiceMeta {
		// 	return svcsChan
		// }, "2s", "0.2s").Should(Receive())

		// _, err := testClients.ProxyClient.Write(getProxyWithConsulRoute(writeNamespace, envoyPort), clients.WriteOpts{Ctx: ctx})
		// Expect(err).NotTo(HaveOccurred())

		// // Wait for proxy to be accepted
		// helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
		// 	return testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
		// })

		// By("requests only go to endpoints behind test upstream 1")

		// // Wait for the endpoints to be registered
		// Eventually(func() (<-chan *v1helpers.ReceivedRequest, error) {
		// 	_, err := queryService()
		// 	if err != nil {
		// 		return svc1.C, err
		// 	}
		// 	return svc1.C, nil
		// }, "20s", "0.2s").Should(Receive())
		// // Service 2 does not match the tags on the route, so we should get only requests from service 1 with test upstream 1 endpoint
		// Consistently(func() (<-chan *v1helpers.ReceivedRequest, error) {
		// 	_, err := queryService()
		// 	if err != nil {
		// 		return svc1.C, err
		// 	}
		// 	return svc1.C, nil
		// }, "2s", "0.2s").Should(Receive())

		// // update service one to point to test upstream 2 port
		// err = consulInstance.LiveUpdateServiceInstance("my-svc", "my-svc-1", envoyInstance.GlooAddr, []string{"svc", "1"}, svc2.Port)
		// Expect(err).NotTo(HaveOccurred())

		// By("requests only go to endpoints behind test upstream 2")

		// // ensure EDS picked up this endpoint-only change
		// Eventually(func() (<-chan *v1helpers.ReceivedRequest, error) {
		// 	_, err := queryService()
		// 	if err != nil {
		// 		return svc2.C, err
		// 	}
		// 	return svc2.C, nil
		// }, "20s", "0.2s").Should(Receive())
		// // test upstream 1 endpoint is now stale; should only get requests to endpoints for test upstream 2 for svc1
		// Consistently(func() (<-chan *v1helpers.ReceivedRequest, error) {
		// 	_, err := queryService()
		// 	if err != nil {
		// 		return svc2.C, err
		// 	}
		// 	return svc2.C, nil
		// }, "2s", "0.2s").Should(Receive())

	})

	It("resolves consul services with hostname addresses (as opposed to IPs addresses)", func() {
		err = consulInstance.RegisterService("my-svc", "my-svc-1", "my-svc.service.dc1.consul", []string{"svc", "1"}, svc1.Port)
		Expect(err).NotTo(HaveOccurred())

		_, err := testClients.ProxyClient.Write(getProxyWithConsulRoute(writeNamespace, envoyPort), clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		// Wait for proxy to be accepted
		helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
		})

		// Wait for endpoints to be discovered
		Eventually(func() (<-chan *v1helpers.ReceivedRequest, error) {
			_, err := queryService()
			if err != nil {
				return svc1.C, err
			}
			return svc1.C, nil
		}, "20s", "0.2s").Should(Receive())

		By("requests only go to service with tag '1'")

		// Service 2 does not match the tags on the route, so we should get only requests from service 1
		Consistently(func() (<-chan *v1helpers.ReceivedRequest, error) {
			_, err := queryService()
			if err != nil {
				return svc1.C, err
			}
			return svc1.C, nil
		}, "2s", "0.2s").Should(Receive())
	})
})

func getProxyWithConsulRoute(ns string, bindPort uint32) *gloov1.Proxy {
	return &gloov1.Proxy{
		Metadata: &core.Metadata{
			Name:      gatewaydefaults.GatewayProxyName,
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
