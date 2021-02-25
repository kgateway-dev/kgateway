package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloohelpers "github.com/solo-io/gloo/test/helpers"

	"github.com/elazarl/goproxy"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = FDescribe("tunneling", func() {

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
		err = envoyInstance.RunWithRoleAndRestXds(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())

		// write a test upstream
		// this is the upstream that will handle requests
		proxy := goproxy.NewProxyHttpServer()
		proxy.Verbose = true

		listener, err := net.Listen("tcp", ":0")
		if err != nil {
			panic(err)
		}

		addr := listener.Addr().String()
		_, portStr, err := net.SplitHostPort(addr)
		if err != nil {
			panic(err)
		}

		port, err := strconv.Atoi(portStr)
		if err != nil {
			panic(err)
		}

		go func() {
			defer GinkgoRecover()

			server := &http.Server{Addr: addr, Handler: proxy}
			server.Serve(listener)

			//http.ListenAndServe(envoyInstance.LocalAddr(), proxy)
		}()
		//testUs := v1helpers.NewTestHttpUpstreamWithReply(ctx, envoyInstance.LocalAddr(), "HTTP/1.1 200 OK\n\n")
		//up = testUs.Upstream

		up = &gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      "local-1",
				Namespace: "default",
			},
			UpstreamType: &gloov1.Upstream_Static{
				Static: &static_plugin_gloo.UpstreamSpec{
					Hosts: []*static_plugin_gloo.Host{
						{
							Addr:              envoyInstance.LocalAddr(),
							Port:              uint32(port),
							SniAddr:           "",
							HealthCheckConfig: nil,
						},
					},
				},
			},
		}

		up.HttpProxyHostname = "host.com:443" // enable HTTP tunneling
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
		// ensure the proxy is created
		Eventually(func() (*gloov1.Proxy, error) {
			return testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
		}, "5s", "0.1s").ShouldNot(BeNil())
	}

	checkVirtualService := func(testVs *gatewayv1.VirtualService) {
		Eventually(func() (*gatewayv1.VirtualService, error) {
			return testClients.VirtualServiceClient.Read(testVs.Metadata.GetNamespace(), testVs.Metadata.GetName(), clients.ReadOpts{})
		}, "5s", "0.1s").ShouldNot(BeNil())
	}

	testRequest := func() string {
		By("Make request")
		responseBody := ""
		EventuallyWithOffset(1, func() error {
			req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d/test", "localhost", defaults.HttpPort), nil)
			if err != nil {
				return err
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			p := new(bytes.Buffer)
			if _, err := io.Copy(p, res.Body); err != nil {
				return err
			}
			defer res.Body.Close()
			responseBody = p.String()
			return nil
		}, "10s", ".1s").Should(BeNil())
		return responseBody
	}

	Context("filter undefined", func() {

		JustBeforeEach(func() {
			// write a virtual service so we have a proxy to our test upstream
			testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
			_, err := testClients.VirtualServiceClient.Write(testVs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			checkProxy()
			checkVirtualService(testVs)
		})

		PIt("should return uncompressed json", func() {
			time.Sleep(1 * time.Second) //TODO(kdorosh) remove
			testReq := testRequest()
			Expect(testReq).Should(ContainSubstring("400 The plain HTTP request was sent to HTTPS port"))
		})

		Context("with SSL", func() {
			FIt("should return uncompressed json", func() {
				time.Sleep(1 * time.Second) //TODO(kdorosh) remove
				testReq := testRequest()
				Expect(testReq).Should(ContainSubstring("400 The plain HTTP request was sent to HTTPS port"))
			})
		})
	})

})
