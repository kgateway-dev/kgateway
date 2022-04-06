package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golang/protobuf/ptypes/wrappers"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/dynamic_forward_proxy"

	envoytransformation "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("dynamic forward proxy", func() {

	var (
		ctx            context.Context
		cancel         context.CancelFunc
		testClients    services.TestClients
		envoyInstance  *services.EnvoyInstance
		writeNamespace = defaults.GlooSystem
		testVs         *gatewayv1.VirtualService
	)

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
			Settings: &gloov1.Settings{
				Gateway: &gloov1.GatewayOptions{
					Validation: &gloov1.GatewayOptions_ValidationOptions{
						DisableTransformationValidation: &wrappers.BoolValue{Value: true},
					},
				},
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)

		// Write Gateway
		gateway := gatewaydefaults.DefaultGateway(writeNamespace)
		gateway.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
			DynamicForwardProxy: &dynamic_forward_proxy.FilterConfig{}, // pick up system defaults to resolve DNS
		}
		_, err = testClients.GatewayClient.Write(gateway, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		// run envoy
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
		err = envoyInstance.RunWithRoleAndRestXds(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())

		// write a virtual service so we have a proxy to our test upstream
		testVs = getTrivialVirtualService(writeNamespace)
		testVs.VirtualHost.Routes[0].Options = &gloov1.RouteOptions{}
		testVs.VirtualHost.Routes[0].GetRouteAction().Destination = &gloov1.RouteAction_DynamicForwardProxy{
			DynamicForwardProxy: &dynamic_forward_proxy.PerRouteConfig{
				HostRewriteSpecifier: &dynamic_forward_proxy.PerRouteConfig_AutoHostRewriteHeader{AutoHostRewriteHeader: "x-rewrite-me"},
			},
		}
	})

	JustBeforeEach(func() {
		// write a virtual service so we have a proxy to our test upstream
		_, err := testClients.VirtualServiceClient.Write(testVs, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		checkProxy()
		checkVirtualService(testVs)
	})

	AfterEach(func() {
		envoyInstance.Clean()
		cancel()
	})

	testRequest := func(dest string, updateReq func(r *http.Request)) string {
		By("Make request")
		responseBody := ""
		EventuallyWithOffset(1, func() error {
			var client http.Client
			scheme := "http"
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s://%s:%d/get", scheme, "localhost", defaults.HttpPort), nil)
			if err != nil {
				return err
			}
			if updateReq != nil {
				updateReq(req)
			}
			res, err := client.Do(req)
			if err != nil {
				return err
			}
			if res.StatusCode != http.StatusOK {
				return fmt.Errorf("not ok")
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

	// simpler e2e test without transformation to validate basic behavior
	It("should proxy http if dynamic forward proxy header provided on request", func() {
		destEcho := `postman-echo.com`
		expectedSubstr := `"host":"postman-echo.com"`
		testReq := testRequest(destEcho, func(r *http.Request) {
			r.Header.Set("x-rewrite-me", destEcho)
		})
		Expect(testReq).Should(ContainSubstring(expectedSubstr))
	})

	Context("with transformation can set dynamic forward proxy header to rewrite authority", func() {

		BeforeEach(func() {
			testVs.VirtualHost.Routes[0].Options.StagedTransformations = &transformation.TransformationStages{
				Early: &transformation.RequestResponseTransformations{
					RequestTransforms: []*transformation.RequestMatch{{
						RequestTransformation: &transformation.Transformation{
							TransformationType: &transformation.Transformation_TransformationTemplate{
								TransformationTemplate: &envoytransformation.TransformationTemplate{
									ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
									Headers: map[string]*envoytransformation.InjaTemplate{
										"x-rewrite-me": {Text: "postman-echo.com"},
									},
								},
							},
						},
					}},
				},
			}
		})

		// This is an important test since the most common use case here will be to grab information from the
		// request using a transformation and use that to determine the upstream destination to route to
		It("should proxy http", func() {
			destEcho := `postman-echo.com`
			expectedSubstr := `"host":"postman-echo.com"`
			testReq := testRequest(destEcho, nil)
			Expect(testReq).Should(ContainSubstring(expectedSubstr))
		})
	})

})
