package e2e_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"

	"github.com/solo-io/gloo/test/services/envoy"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	"github.com/onsi/gomega/format"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gwdefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_json"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	glootest "github.com/solo-io/gloo/test/v1helpers/test_grpc_service/glootest/protos"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
)

var _ = Describe("GRPC to JSON Transcoding Plugin - Envoy API", func() {
	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *envoy.Instance
		tu            *v1helpers.TestUpstream
	)
	format.MaxLength = 0
	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		envoy.AdvanceRequestPorts()

		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

		ro := &services.RunOptions{
			NsToWrite: writeNamespace,
			NsToWatch: []string{"default", writeNamespace},
			WhatToRun: services.What{
				DisableGateway: false,
				DisableUds:     true,
				DisableFds:     true,
			},
			Settings: &gloov1.Settings{
				Gloo: &gloov1.GlooOptions{
					// https://github.com/solo-io/gloo/issues/7577
					RemoveUnusedFilters: &wrappers.BoolValue{Value: false},
				},
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)

		err = envoyInstance.RunWithRoleAndRestXds(writeNamespace+"~"+gwdefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())

		tu = v1helpers.NewTestGRPCUpstream(ctx, envoyInstance.LocalAddr(), 1)
		_, err = testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		envoyInstance.Clean()
		cancel()
	})

	testRequest := func(path string, shouldMatch bool) {
		body := "foo" // this is valid JSON because of the quotes
		resp := func() (*http.Response, error) {
			// send a request with a body
			return http.Post(fmt.Sprintf("http://%s:%d/%s", "localhost", defaults.HttpPort, path), "application/json", bytes.NewBufferString(body))
		}
		expectedResp := `{"str":"foo"}`
		expectedFields := Fields{
			"GRPCRequest": PointTo(Equal(glootest.TestRequest{Str: "foo"})),
		}
		if shouldMatch {
			EventuallyWithOffset(1, func(g Gomega) {
				g.Expect(resp).Should(testmatchers.HaveExactResponseBody(expectedResp), "Did not get expected response")
			}, 5, 1)
			EventuallyWithOffset(1, func(g Gomega) {
				g.Expect(tu.C).Should(Receive(PointTo(MatchFields(IgnoreExtras, expectedFields))), "Upstream did not record expected request")
			})
			//tu.C).Should(Receive(PointTo(MatchFields(IgnoreExtras, expectedFields))), "Upstream did not record expected request")
		} else {
			EventuallyWithOffset(1, func(g Gomega) {
				g.Expect(resp).ShouldNot(testmatchers.HaveExactResponseBody(expectedResp), "Got unexpected response")
			}, 5, 1)
			EventuallyWithOffset(1, func(g Gomega) {
				g.Expect(tu.C).ShouldNot(Receive(PointTo(MatchFields(IgnoreExtras, expectedFields))), "Upstream recorded unexpected request")
			})
		}
	}

	Context("Routes to GRPC Functions", func() {

		It("with protodescriptor specified on gateway", func() {
			gw := getGrpcJsonGateway()
			_, err := testClients.GatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx})
			Expect(err).ToNot(HaveOccurred())

			vs := getGrpcJsonRawVs(writeNamespace, tu.Upstream.Metadata.Ref())
			_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			testRequest("test", true)
		})

		It("with protodescriptor from configmap", func() {
			// create an artifact containing the proto descriptor data
			pathToDescriptors := "../v1helpers/test_grpc_service/descriptors/proto.pb"
			bytes, err := os.ReadFile(pathToDescriptors)
			Expect(err).ToNot(HaveOccurred())
			encoded := base64.StdEncoding.EncodeToString(bytes)
			artifact := &gloov1.Artifact{
				Metadata: &core.Metadata{
					Name:      "my-config-map",
					Namespace: "gloo-system",
				},
				Data: map[string]string{
					"protoDesc": encoded,
				},
			}
			_, err = testClients.ArtifactClient.Write(artifact, clients.WriteOpts{Ctx: ctx})
			Expect(err).ToNot(HaveOccurred())

			// use the configmap ref in the gateway
			gw := getGrpcJsonGateway()
			gw.GatewayType.(*gatewayv1.Gateway_HttpGateway).HttpGateway.Options.GrpcJsonTranscoder.DescriptorSet =
				&grpc_json.GrpcJsonTranscoder_ProtoDescriptorConfigMap{
					ProtoDescriptorConfigMap: &grpc_json.GrpcJsonTranscoder_DescriptorConfigMap{
						ConfigMapRef: &core.ResourceRef{Name: "my-config-map", Namespace: "gloo-system"},
					},
				}
			_, err = testClients.GatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx})
			Expect(err).ToNot(HaveOccurred())

			vs := getGrpcJsonRawVs(writeNamespace, tu.Upstream.Metadata.Ref())
			_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			testRequest("test", true)
		})
	})
	Describe("Route matching behavior", func() {
		It("When the route prefix is set to match one of the GrpcJsonTranscoder's services, requests to /test should go through", func() {
			// Write a virtual service with a single route that matches against the prefix "/glootest.TestService"
			vs := getGrpcJsonRawVs(writeNamespace, tu.Upstream.Metadata.Ref())
			vs.VirtualHost.Routes[0].Matchers[0].PathSpecifier.(*matchers.Matcher_Prefix).Prefix = "/glootest.TestService"
			_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			gw := getGrpcJsonGateway()
			_, err = testClients.GatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx})
			Expect(err).ToNot(HaveOccurred())

			testRequest("test", true)
		})

		It("When MatchIncomingRequestRoute is true and route prefix matches service, requests to /test should fail", func() {
			vs := getGrpcJsonRawVs(writeNamespace, tu.Upstream.Metadata.Ref())
			vs.VirtualHost.Routes[0].Matchers[0].PathSpecifier.(*matchers.Matcher_Prefix).Prefix = "/glootest.TestService"
			_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			gw := getGrpcJsonGateway()
			gw.GatewayType.(*gatewayv1.Gateway_HttpGateway).HttpGateway.Options.GrpcJsonTranscoder.MatchIncomingRequestRoute = true
			_, err = testClients.GatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx})
			Expect(err).ToNot(HaveOccurred())

			testRequest("test", false)
		})

		It("When MatchIncomingRequestRoute is true and route prefix matches request path, requests to /test should succeed", func() {
			vs := getGrpcJsonRawVs(writeNamespace, tu.Upstream.Metadata.Ref())
			vs.VirtualHost.Routes[0].Matchers[0].PathSpecifier.(*matchers.Matcher_Prefix).Prefix = "/test"
			_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			gw := getGrpcJsonGateway()
			gw.GatewayType.(*gatewayv1.Gateway_HttpGateway).HttpGateway.Options.GrpcJsonTranscoder.MatchIncomingRequestRoute = true
			_, err = testClients.GatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx})
			Expect(err).ToNot(HaveOccurred())

			testRequest("test", true)
		})
	})

	Context("GRPC configured on Upstream", func() {
		It("with protodescriptor on upstream", func() {

			gw := gwdefaults.DefaultGateway(writeNamespace)

			_, err := testClients.GatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx})
			Expect(err).ToNot(HaveOccurred())
			helpers.PatchResource(ctx, tu.Upstream.Metadata.Ref(), addGrpcJsonToUpstream, testClients.UpstreamClient.BaseClient())
			Expect(err).NotTo(HaveOccurred())
			vs := getGrpcJsonRawVs(writeNamespace, tu.Upstream.Metadata.Ref())
			_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())
			testRequest("test", true)
		})
	})
})

func getGrpcJsonGateway() *gatewayv1.Gateway {
	// Get the descriptor set bytes from the generated proto, rather than the go file (pb.go)
	// as the generated go file doesn't have the annotations we need for gRPC to JSON transcoding
	pathToDescriptors := "../v1helpers/test_grpc_service/descriptors/proto.pb"
	bytes, err := os.ReadFile(pathToDescriptors)
	Expect(err).ToNot(HaveOccurred())

	return &gatewayv1.Gateway{
		BindAddress:        "::",
		BindPort:           envoy.HttpPort,
		NamespacedStatuses: &core.NamespacedStatuses{},
		Metadata: &core.Metadata{
			Name:      "gateway-proxy",
			Namespace: "gloo-system",
		},
		GatewayType: &gatewayv1.Gateway_HttpGateway{
			HttpGateway: &gatewayv1.HttpGateway{
				Options: &gloov1.HttpListenerOptions{
					GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
						DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptorBin{ProtoDescriptorBin: bytes},
						Services:      []string{"glootest.TestService"},
					},
				},
			},
		},
		ProxyNames: []string{"gateway-proxy"},
	}
}
func addGrpcJsonToUpstream(res resources.Resource) resources.Resource {
	// Get the descriptor set bytes from the generated proto, rather than the go file (pb.go)
	// as the generated go file doesn't have the annotations we need for gRPC to JSON transcoding
	tu := res.(*gloov1.Upstream)
	pathToDescriptors := "../v1helpers/test_grpc_service/descriptors/proto.pb"
	bytes, err := os.ReadFile(pathToDescriptors)
	Expect(err).ToNot(HaveOccurred())
	t := tu.GetUpstreamType().(*gloov1.Upstream_Static)
	t.SetServiceSpec(&options.ServiceSpec{
		PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
			GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
				DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptorBin{
					ProtoDescriptorBin: bytes,
				},
				Services: []string{"glootest.TestService"},
			},
		}})
	return tu
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
