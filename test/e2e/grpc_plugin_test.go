package e2e_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/solo-io/gloo/test/services/envoy"

	"github.com/solo-io/gloo/test/e2e"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"

	"github.com/golang/protobuf/ptypes/wrappers"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gwdefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

var _ = Describe("GRPC to JSON Transcoding Plugin - Gloo API", func() {

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *envoy.Instance
		tu            *v1helpers.TestUpstream
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		envoyInstance = envoyFactory.NewInstance()

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
					// https://github.com/solo-io/gloo/issues/8374
					RemoveUnusedFilters: &wrappers.BoolValue{Value: false},
				},
				Discovery: &gloov1.Settings_DiscoveryOptions{
					FdsMode: gloov1.Settings_DiscoveryOptions_DISABLED,
				},
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)
		err := helpers.WriteDefaultGateways(writeNamespace, testClients.GatewayClient)
		Expect(err).NotTo(HaveOccurred(), "Should be able to create the default gateways")
		err = envoyInstance.RunWithRoleAndRestXds(writeNamespace+"~"+gwdefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())

		tu = v1helpers.NewTestGRPCUpstream(ctx, envoyInstance.LocalAddr(), 1)
		_, err = testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())
		// Discovery is off so we fill in the upstream here.
		helpers.PatchResource(ctx, tu.Upstream.Metadata.Ref(), populateDeprecatedApi, testClients.UpstreamClient.BaseClient())
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
			res, err := http.Post(fmt.Sprintf("http://%s:%d/test", "localhost", envoyInstance.HttpPort), "application/json", &buf)
			if err != nil {
				return "", err
			}
			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
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
			body, err := io.ReadAll(res.Body)
			return string(body), err
		}

		Eventually(testRequest, 30, 1).Should(Equal(`{"str":"foo"}`))

	})
})

func getGrpcVs(writeNamespace string, usRef *core.ResourceRef) *gatewayv1.VirtualService {
	return &gatewayv1.VirtualService{
		Metadata: &core.Metadata{
			Name:      e2e.DefaultVirtualServiceName,
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

func populateDeprecatedApi(res resources.Resource) resources.Resource {
	tu := res.(*gloov1.Upstream)
	pathToDescriptors := "../v1helpers/test_grpc_service/descriptors/proto.pb"
	bytes, err := os.ReadFile(pathToDescriptors)
	Expect(err).ToNot(HaveOccurred())
	singleEncoded := []byte(base64.StdEncoding.EncodeToString(bytes))
	grpcServices := []*grpc.ServiceSpec_GrpcService{
		{
			ServiceName: "TestService",
			PackageName: "glootest",
		},
	}
	t := tu.GetUpstreamType().(*gloov1.Upstream_Static)
	t.SetServiceSpec(&options.ServiceSpec{
		PluginType: &options.ServiceSpec_Grpc{
			Grpc: &grpc.ServiceSpec{
				Descriptors:  singleEncoded,
				GrpcServices: grpcServices,
			},
		}})
	return tu
}
