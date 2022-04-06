package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gwdefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_json"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	glootest "github.com/solo-io/gloo/test/v1helpers/test_grpc_service/glootest/protos"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

var _ = Describe("GRPC to JSON Transcoding Plugin - Envoy API", func() {
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
				DisableFds:     true,
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)

		_, err = testClients.GatewayClient.Write(getGrpcJsonGateway(false), clients.WriteOpts{Ctx: ctx})
		Expect(err).ToNot(HaveOccurred())

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

		vs := getGrpcJsonRawVs(writeNamespace, tu.Upstream.Metadata.Ref())
		_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		body := []byte(`"foo"`) // this is valid JSON because of the quotes

		testRequest := basicReq(body)

		Eventually(testRequest, 5, 1).Should(Equal(`{"str":"foo"}`))

		Eventually(tu.C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
			"GRPCRequest": PointTo(Equal(glootest.TestRequest{Str: "foo"})),
		}))))

	})

	Describe("Route matching behavior", func() {
		BeforeEach(func() {
			// Write a virtual service with a single route that matches against the prefix "/glootest.TestService"
			vs := getGrpcJsonRawVs(writeNamespace, tu.Upstream.Metadata.Ref())
			vs.VirtualHost.Routes[0].Matchers[0].PathSpecifier.(*matchers.Matcher_Prefix).Prefix = "/glootest.TestService"
			_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("When the route prefix is set to match one of the GrpcJsonTranscoder's services, requests to /test should through", func() {
			body := []byte(`"foo"`) // this is valid JSON because of the quotes

			testRequest := basicReq(body)

			Eventually(testRequest, 5, 1).Should(Equal(`{"str":"foo"}`))

			Eventually(tu.C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
				"GRPCRequest": PointTo(Equal(glootest.TestRequest{Str: "foo"})),
			}))))
		})

		It("Route matching assumptions are not broken when MatchIncomingRequestRoute is set", func() {
			// overwrite grpcJsonGateway with one where MatchIncomingRequestRoute is true
			err := testClients.GatewayClient.Delete("gloo-system", "gateway-proxy", clients.DeleteOpts{Ctx: ctx})
			Expect(err).ToNot(HaveOccurred())
			gw := getGrpcJsonGateway(true)
			_, err = testClients.GatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx})
			Expect(err).ToNot(HaveOccurred())

			body := []byte(`"foo"`) // this is valid JSON because of the quotes

			testRequest := basicReq(body)

			Eventually(testRequest, 5, 1).ShouldNot(Equal(`{"str":"foo"}`))

			Eventually(tu.C).ShouldNot(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
				"GRPCRequest": PointTo(Equal(glootest.TestRequest{Str: "foo"})),
			}))))
		})
	})

})

func getGrpcJsonGateway(matchIncomingRequestRoute bool) *gatewayv1.Gateway {

	// Get the descriptor set bytes from the generated proto, rather than the go file (pb.go)
	// as the generated go file doesn't have the annotations we need for gRPC to JSON transcoding
	pathToDescriptors := "../v1helpers/test_grpc_service/descriptors/proto.pb"
	bytes, err := ioutil.ReadFile(pathToDescriptors)
	Expect(err).ToNot(HaveOccurred())

	return &gatewayv1.Gateway{
		BindAddress:        "::",
		BindPort:           defaults.HttpPort,
		NamespacedStatuses: &core.NamespacedStatuses{},
		Metadata: &core.Metadata{
			Name:      "gateway-proxy",
			Namespace: "gloo-system",
		},
		GatewayType: &gatewayv1.Gateway_HttpGateway{
			HttpGateway: &gatewayv1.HttpGateway{
				Options: &gloov1.HttpListenerOptions{
					GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
						DescriptorSet:             &grpc_json.GrpcJsonTranscoder_ProtoDescriptorBin{ProtoDescriptorBin: bytes},
						Services:                  []string{"glootest.TestService"},
						MatchIncomingRequestRoute: matchIncomingRequestRoute,
					},
				},
			},
		},
		ProxyNames: []string{"gateway-proxy"},
	}
}

func getGrpcJsonRawVs(writeNamespace string, usRef *core.ResourceRef) *gatewayv1.VirtualService {
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
							// the grpc_json transcoding filter clears the cache so it no longer would match on /test (this can be configured)
							Prefix: "/",
						},
					}},
					Action: &gatewayv1.Route_RouteAction{
						RouteAction: &gloov1.RouteAction{
							Destination: &gloov1.RouteAction_Single{
								Single: &gloov1.Destination{
									DestinationType: &gloov1.Destination_Upstream{
										Upstream: usRef,
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
