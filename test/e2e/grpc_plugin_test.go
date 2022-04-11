package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/golang/protobuf/ptypes/wrappers"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gwdefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	glootest "github.com/solo-io/gloo/test/v1helpers/test_grpc_service/glootest/protos"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

var _ = Describe("GRPC to JSON Transcoding Plugin - Gloo API", func() {
	var (
		ctx            context.Context
		cancel         context.CancelFunc
		testClients    services.TestClients
		envoyInstance  *services.EnvoyInstance
		tu             *v1helpers.TestUpstream
		writeNamespace string
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()

		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

		writeNamespace = defaults.GlooSystem
		ro := &services.RunOptions{
			NsToWrite: writeNamespace,
			NsToWatch: []string{"default", writeNamespace},
			WhatToRun: services.What{
				DisableGateway: false,
				DisableUds:     true,
				// test relies on FDS to discover the grpc spec via reflection
				DisableFds: false,
			},
			Settings: &gloov1.Settings{
				Discovery: &gloov1.Settings_DiscoveryOptions{
					FdsMode: gloov1.Settings_DiscoveryOptions_BLACKLIST,
				},
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)
		err = helpers.WriteDefaultGateways(writeNamespace, testClients.GatewayClient)
		Expect(err).NotTo(HaveOccurred(), "Should be able to create the default gateways")
		err = envoyInstance.RunWithRoleAndRestXds(writeNamespace+"~"+gwdefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())

		tu = v1helpers.NewTestGRPCUpstream(ctx, envoyInstance.LocalAddr(), 1)
		_, err = testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		envoyInstance.Clean()
		cancel()
	})

	basicReq := func(b []byte) func() (string, error) {
		return func() (string, error) {
			// send a request with a body
			var buf bytes.Buffer
			buf.Write(b)
			res, err := http.Post(fmt.Sprintf("http://%s:%d/test", "localhost", defaults.HttpPort), "application/json", &buf)
			if err != nil {
				return "", err
			}
			defer res.Body.Close()
			body, err := ioutil.ReadAll(res.Body)
			return string(body), err
		}
	}

	It("Routes to GRPC Functions", func() {

		vs := getGrpcVs(writeNamespace, tu.Upstream.Metadata.Ref())
		_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		body := []byte(`{"str": "foo"}`)

		testRequest := basicReq(body)

		Eventually(testRequest, 30, 1).Should(Equal(`{"str":"foo"}`))

		Eventually(tu.C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
			"GRPCRequest": PointTo(Equal(glootest.TestRequest{Str: "foo"})),
		}))))
	})

	It("Routes to GRPC Functions with parameters", func() {

		vs := getGrpcVs(writeNamespace, tu.Upstream.Metadata.Ref())
		grpc := vs.VirtualHost.Routes[0].GetRouteAction().GetSingle().GetDestinationSpec().GetGrpc()
		grpc.Parameters = &transformation.Parameters{
			Path: &wrappers.StringValue{Value: "/test/{str}"},
		}
		_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		testRequest := func() (string, error) {
			res, err := http.Get(fmt.Sprintf("http://%s:%d/test/foo", "localhost", defaults.HttpPort))
			if err != nil {
				return "", err
			}
			defer res.Body.Close()
			body, err := ioutil.ReadAll(res.Body)
			return string(body), err
		}

		Eventually(testRequest, 30, 1).Should(Equal(`{"str":"foo"}`))

		Eventually(tu.C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
			"GRPCRequest": PointTo(Equal(glootest.TestRequest{Str: "foo"})),
		}))))
	})
})

func getGrpcVs(writeNamespace string, usRef *core.ResourceRef) *gatewayv1.VirtualService {
	return &gatewayv1.VirtualService{
		Metadata: &core.Metadata{
			Name:      "default",
			Namespace: writeNamespace,
		},
		VirtualHost: &gatewayv1.VirtualHost{
			Routes: []*gatewayv1.Route{
				{
					Matchers: []*matchers.Matcher{{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/test",
						},
					}},
					Action: &gatewayv1.Route_RouteAction{
						RouteAction: &gloov1.RouteAction{
							Destination: &gloov1.RouteAction_Single{
								Single: &gloov1.Destination{
									DestinationType: &gloov1.Destination_Upstream{
										Upstream: usRef,
									},
									DestinationSpec: &gloov1.DestinationSpec{
										DestinationType: &gloov1.DestinationSpec_Grpc{
											Grpc: &grpc.DestinationSpec{
												Package:  "glootest",
												Function: "TestMethod",
												Service:  "TestService",
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
	}
}
