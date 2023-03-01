package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_json"

	"github.com/golang/protobuf/ptypes/wrappers"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gwdefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

var _ = Describe("GRPC to JSON Transcoding Plugin - Gloo API", func() {

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance
		tu            *v1helpers.TestUpstream
	)

	BeforeEach(func() {
		//	testutils.ValidateRequirementsAndNotifyGinkgo(
		//		testutils.LinuxOnly("Relies on FDS"),
		//	)

		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()

		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

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
				Gloo: &gloov1.GlooOptions{
					// https://github.com/solo-io/gloo/issues/7577
					RemoveUnusedFilters: &wrappers.BoolValue{Value: false},
				},
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

	FIt("Routes to GRPC Functions", func() {

		vs := getGrpcTranscoderVs(writeNamespace, tu.Upstream.Metadata.Ref())
		_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		body := []byte(`{"str": "foo"}`)

		testRequest := basicReq(body)

		Eventually(testRequest, 30, 1).Should(Equal(`{"str":"foo"}`))

		Eventually(tu.C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
			"GRPCRequest": PointTo(MatchFields(IgnoreExtras, Fields{"Str": Equal("foo")})),
		}))))
	})

	FIt("Routes to GRPC Functions with parameters", func() {

		vs := getGrpcTranscoderVs(writeNamespace, tu.Upstream.Metadata.Ref())
		_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		testRequest := func() (string, error) {
			res, err := http.Get(fmt.Sprintf("http://%s:%d/t/foo", "localhost", defaults.HttpPort))
			if err != nil {
				return "", err
			}
			defer res.Body.Close()
			body, err := ioutil.ReadAll(res.Body)
			return string(body), err
		}
		// set up upstream manually to test everything except discovery locally
		//updateUpstreamDescriptors(tu.Upstream)
		//_, err = testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{OverwriteExisting: true})
		//Expect(err).NotTo(HaveOccurred())
		Eventually(testRequest, 30, 1).Should(Equal(`{"str":"foo"}`))
		Eventually(tu.C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
			"GRPCRequest": PointTo(MatchFields(IgnoreExtras, Fields{"Str": Equal("foo")})),
		}))))
	})
})

func getGrpcTranscoderVs(writeNamespace string, usRef *core.ResourceRef) *gatewayv1.VirtualService {
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

//TODO using this to test locally without discovery, remove before merging
func updateUpstreamDescriptors(tu *gloov1.Upstream) {
	// Get the descriptor set bytes from the generated proto, rather than the go file (pb.go)
	// as the generated go file doesn't have the annotations we need for gRPC to JSON transcoding
	pathToDescriptors := "../v1helpers/test_grpc_service/descriptors/proto.pb"
	bytes, err := ioutil.ReadFile(pathToDescriptors)
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
	tu.Metadata.ResourceVersion = "2"

}
