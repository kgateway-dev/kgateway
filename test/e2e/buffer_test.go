package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/protobuf/ptypes/wrappers"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloohelpers "github.com/solo-io/gloo/test/helpers"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	buffer "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/buffer/v3"
)

var _ = Describe("buffer", func() {

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
		if envoyInstance != nil {
			_ = envoyInstance.Clean()
		}
		cancel()
	})

	checkProxy := func() {
		// ensure the proxy and virtual service are created
		Eventually(func() (*gloov1.Proxy, error) {
			return testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
		}, "5s", "0.1s").ShouldNot(BeNil())
	}

	checkVirtualService := func(testVs *gatewayv1.VirtualService) {
		Eventually(func() (*gatewayv1.VirtualService, error) {
			return testClients.VirtualServiceClient.Read(testVs.Metadata.GetNamespace(), testVs.Metadata.GetName(), clients.ReadOpts{})
		}, "5s", "0.1s").ShouldNot(BeNil())
	}

	testRequest := func() func() (string, error) {
		return func() (string, error) {
			var json = []byte(`{"value":"test"}`)
			req, err := http.NewRequest("POST", fmt.Sprintf("http://%s:%d/test", "localhost", defaults.HttpPort), bytes.NewBuffer(json))
			//req.Header.Add("Content-Length",size)
			req.Header.Set("Content-Type", "application/json")
			if err != nil {
				return "", err
			}

			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return "", err
			}
			defer res.Body.Close()
			body, err := ioutil.ReadAll(res.Body)
			return string(body), err
		}
	}

	Context("filter defined on listener", func() {

		Context("Large buffer ", func() {
			JustBeforeEach(func() {
				gatewayClient := testClients.GatewayClient
				gw, err := gatewayClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				// build a buffer policy
				bufferPolicy := &buffer.Buffer{
					MaxRequestBytes: &wrappers.UInt32Value{
						Value: 4098, // max size
					},
				}

				// update the listener to include the gzip policy
				httpGateway := gw.GetHttpGateway()
				httpGateway.Options = &gloov1.HttpListenerOptions{
					Buffer: bufferPolicy,
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

			It("valid buffer size should succeed", func() {
				testReq := testRequest()
				Eventually(testReq, 10*time.Second, 1*time.Second).Should(Equal("{\"value\":\"test\"}"))
			})

		})

		Context("Small buffer ", func() {
			JustBeforeEach(func() {
				gatewayClient := testClients.GatewayClient
				gw, err := gatewayClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				// build a buffer policy
				bufferPolicy := &buffer.Buffer{
					MaxRequestBytes: &wrappers.UInt32Value{
						Value: 1,
					},
				}

				// update the listener to include the buffer policy
				httpGateway := gw.GetHttpGateway()
				httpGateway.Options = &gloov1.HttpListenerOptions{
					Buffer: bufferPolicy,
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

			It("empty buffer should fail", func() {
				testReq := testRequest()
				Eventually(testReq, 10*time.Second, 1*time.Second).Should(Equal("Payload Too Large"))
			})
		})
	})

	Context("filter defined on listener and vhost", func() {
		Context("Large buffer ", func() {
			JustBeforeEach(func() {
				gatewayClient := testClients.GatewayClient
				gw, err := gatewayClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				// build a buffer policy
				bufferPolicy := &buffer.Buffer{
					MaxRequestBytes: &wrappers.UInt32Value{
						Value: 1,
					},
				}

				// update the listener to include the gzip policy
				httpGateway := gw.GetHttpGateway()
				httpGateway.Options = &gloov1.HttpListenerOptions{
					Buffer: bufferPolicy,
				}
				_, err = gatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())

				// build a buffer policy
				bufferPolicyVh := &buffer.BufferPerRoute{
					Override: &buffer.BufferPerRoute_Buffer{
						Buffer: &buffer.Buffer{
							MaxRequestBytes: &wrappers.UInt32Value{
								Value: 4098, // max size
							},
						},
					},
				}

				// write a virtual service so we have a proxy to our test upstream
				vhClient := testClients.VirtualServiceClient
				testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
				testVs.VirtualHost.Options = &gloov1.VirtualHostOptions{
					BufferPerRoute: bufferPolicyVh,
				}
				_, err = vhClient.Write(testVs, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())

				checkProxy()
				checkVirtualService(testVs)
			})

			It("valid buffer size should succeed", func() {
				testReq := testRequest()
				Eventually(testReq, 10*time.Second, 1*time.Second).Should(Equal("{\"value\":\"test\"}"))
			})

		})

		Context("Small buffer ", func() {
			JustBeforeEach(func() {
				gatewayClient := testClients.GatewayClient
				gw, err := gatewayClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				// build a buffer policy
				bufferPolicy := &buffer.Buffer{
					MaxRequestBytes: &wrappers.UInt32Value{
						Value: 4098, // max size
					},
				}

				// update the listener to include the gzip policy
				httpGateway := gw.GetHttpGateway()
				httpGateway.Options = &gloov1.HttpListenerOptions{
					Buffer: bufferPolicy,
				}
				_, err = gatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())

				// build a buffer policy
				bufferPolicyVh := &buffer.BufferPerRoute{
					Override: &buffer.BufferPerRoute_Buffer{
						Buffer: &buffer.Buffer{
							MaxRequestBytes: &wrappers.UInt32Value{
								Value: 1,
							},
						},
					},
				}

				// write a virtual service so we have a proxy to our test upstream
				vhClient := testClients.VirtualServiceClient
				testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
				testVs.VirtualHost.Options = &gloov1.VirtualHostOptions{
					BufferPerRoute: bufferPolicyVh,
				}
				_, err = vhClient.Write(testVs, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())

				checkProxy()
				checkVirtualService(testVs)
			})

			It("empty buffer should fail", func() {
				testReq := testRequest()
				Eventually(testReq, 10*time.Second, 1*time.Second).Should(Equal("Payload Too Large"))
			})
		})
	})

	Context("filter defined on listener and route", func() {

		Context("Large buffer ", func() {
			JustBeforeEach(func() {
				gatewayClient := testClients.GatewayClient
				gw, err := gatewayClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				// build a buffer policy
				bufferPolicy := &buffer.Buffer{
					MaxRequestBytes: &wrappers.UInt32Value{
						Value: 1,
					},
				}

				// update the listener to include the gzip policy
				httpGateway := gw.GetHttpGateway()
				httpGateway.Options = &gloov1.HttpListenerOptions{
					Buffer: bufferPolicy,
				}
				_, err = gatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())

				// build a buffer policy
				bufferPolicyRoute := &buffer.BufferPerRoute{
					Override: &buffer.BufferPerRoute_Buffer{
						Buffer: &buffer.Buffer{
							MaxRequestBytes: &wrappers.UInt32Value{
								Value: 4098, // max size
							},
						},
					},
				}

				// write a virtual service so we have a proxy to our test upstream
				vhClient := testClients.VirtualServiceClient
				testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
				// apply to route
				route := testVs.VirtualHost.Routes[0]
				route.Options = &gloov1.RouteOptions{
					BufferPerRoute: bufferPolicyRoute,
				}
				_, err = vhClient.Write(testVs, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())

				checkProxy()
				checkVirtualService(testVs)
			})

			It("valid buffer size should succeed", func() {
				testReq := testRequest()
				Eventually(testReq, 10*time.Second, 1*time.Second).Should(Equal("{\"value\":\"test\"}"))
			})

		})

		Context("Small buffer ", func() {
			JustBeforeEach(func() {
				gatewayClient := testClients.GatewayClient
				gw, err := gatewayClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				// build a buffer policy
				bufferPolicy := &buffer.Buffer{
					MaxRequestBytes: &wrappers.UInt32Value{
						Value: 4098, // max size
					},
				}

				// update the listener to include the gzip policy
				httpGateway := gw.GetHttpGateway()
				httpGateway.Options = &gloov1.HttpListenerOptions{
					Buffer: bufferPolicy,
				}
				_, err = gatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())

				// build a buffer policy
				bufferPolicyRoute := &buffer.BufferPerRoute{
					Override: &buffer.BufferPerRoute_Buffer{
						Buffer: &buffer.Buffer{
							MaxRequestBytes: &wrappers.UInt32Value{
								Value: 1,
							},
						},
					},
				}

				// write a virtual service so we have a proxy to our test upstream
				vhClient := testClients.VirtualServiceClient
				testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
				// apply to route
				route := testVs.VirtualHost.Routes[0]
				route.Options = &gloov1.RouteOptions{
					BufferPerRoute: bufferPolicyRoute,
				}
				_, err = vhClient.Write(testVs, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())

				checkProxy()
				checkVirtualService(testVs)
			})

			It("empty buffer should fail", func() {
				testReq := testRequest()
				Eventually(testReq, 10*time.Second, 1*time.Second).Should(Equal("Payload Too Large"))
			})
		})
	})

})
