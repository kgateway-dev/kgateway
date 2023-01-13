package e2e_test

import (
	"bytes"
	"context"
	"encoding/base64"
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

	basicReq := func(b []byte, path string) func() (string, error) {
		return func() (string, error) {
			// send a request with a body
			var buf bytes.Buffer
			buf.Write(b)
			res, err := http.Post(fmt.Sprintf("http://%s:%d/%s", "localhost", defaults.HttpPort, path), "application/json", &buf)
			if err != nil {
				return "", err
			}
			defer res.Body.Close()
			body, err := ioutil.ReadAll(res.Body)
			return string(body), err
		}
	}

	testRequest := func(path string, shouldMatch bool) {
		body := []byte(`"foo"`) // this is valid JSON because of the quotes
		resp := basicReq(body, path)
		expectedResp := `{"str":"foo"}`
		expectedFields := Fields{
			"GRPCRequest": PointTo(Equal(glootest.TestRequest{Str: "foo"})),
		}
		if shouldMatch {
			EventuallyWithOffset(1, resp, 5, 1).Should(Equal(expectedResp))
			EventuallyWithOffset(1, tu.C).Should(Receive(PointTo(MatchFields(IgnoreExtras, expectedFields))))
		} else {
			EventuallyWithOffset(1, resp, 5, 1).ShouldNot(Equal(expectedResp))
			EventuallyWithOffset(1, tu.C).ShouldNot(Receive(PointTo(MatchFields(IgnoreExtras, expectedFields))))
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
			bytes, err := ioutil.ReadFile(pathToDescriptors)
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

})

func getGrpcJsonGateway() *gatewayv1.Gateway {
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
						DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptorBin{ProtoDescriptorBin: bytes},
						Services:      []string{"glootest.TestService"},
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
