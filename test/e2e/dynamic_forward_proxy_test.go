package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
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
	})

	JustBeforeEach(func() {

		removeMeUs := &gloov1.Upstream{Metadata: &core.Metadata{
			Name:            "placeholder",
			Namespace:       "gloo-system",
		}}
		_, err := testClients.UpstreamClient.Write(removeMeUs, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		// write a virtual service so we have a proxy to our test upstream
		testVs := getTrivialVirtualService(writeNamespace)
		testVs.VirtualHost.Routes[0].GetRouteAction().GetSingle().DestinationType = &gloov1.Destination_DynamicForwardProxy{DynamicForwardProxy: &empty.Empty{}}

		// TODO(kdorosh) move to before each
		//testVs.VirtualHost.Routes[0].Options = &gloov1.RouteOptions{}
		//testVs.VirtualHost.Routes[0].Options.StagedTransformations = &transformation.TransformationStages{
		//	Early: &transformation.RequestResponseTransformations{
		//		RequestTransforms: []*transformation.RequestMatch{{
		//			Matcher:               nil,
		//			ClearRouteCache:       true,
		//			RequestTransformation: &transformation.Transformation{
		//				TransformationType: &transformation.Transformation_TransformationTemplate{
		//					TransformationTemplate: &envoytransformation.TransformationTemplate{
		//						ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
		//						Headers: map[string]*envoytransformation.InjaTemplate{
		//							"x-rewrite-me": {Text: "postman-echo.com"},
		//						},
		//					},
		//				},
		//			},
		//		}},
		//	},
		//}

		// write a virtual service so we have a proxy to our test upstream
		_, err = testClients.VirtualServiceClient.Write(testVs, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		checkProxy()
		checkVirtualService(testVs)
	})

	AfterEach(func() {
		if envoyInstance != nil {
			_ = envoyInstance.Clean()
		}
		cancel()
	})

	testRequest := func(dest string) string {
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

			// TODO(kdorosh) ensure works with transformations for via use case
			// use https://github.com/envoyproxy/envoy/blob/935868923883b731f81140c613e8cc3b78e023f9/api/envoy/extensions/filters/http/dynamic_forward_proxy/v3/dynamic_forward_proxy.proto#L39
			//req.Header.Set(":authority", "jsonplaceholder.typicode.com")
			req.Header.Set("x-rewrite-me", dest)

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
		}, "10s", "3s").Should(BeNil()) // TODO(kdorosh) make .1s interval
		return responseBody
	}

	It("should proxy http", func() {
		destEcho := `postman-echo.com`
		expectedSubstr := `"host":"postman-echo.com"`
		testReq := testRequest(destEcho)
		Expect(testReq).Should(ContainSubstring(expectedSubstr))
	})

	Context("with transformation can grab and set header to rewrite authority", func() {
		FIt("should proxy http", func() {
			destEcho := `postman-echo.com`
			expectedSubstr := `"host":"postman-echo.com"`
			testReq := testRequest(destEcho)
			Expect(testReq).Should(ContainSubstring(expectedSubstr))
		})
	})

})
