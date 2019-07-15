package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	types "github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/utils"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	grpc "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/grpc"
	transformation "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/transformation"
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

var _ = FDescribe("GRPC Plugin", func() {
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
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)
		err = envoyInstance.RunWithRole(writeNamespace+"~gateway-proxy", testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		tu = v1helpers.NewTestGRPCUpstream(ctx, envoyInstance.LocalAddr())
		_, err = testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if envoyInstance != nil {
			_ = envoyInstance.Clean()
		}
		cancel()
	})

	getGrpcVs := func() *gatewayv1.VirtualService {
		return &gatewayv1.VirtualService{
			Metadata: core.Metadata{
				Name:      "default",
				Namespace: writeNamespace,
			},
			VirtualHost: &gloov1.VirtualHost{
				Routes: []*gloov1.Route{
					{
						Matcher: &gloov1.Matcher{
							PathSpecifier: &gloov1.Matcher_Prefix{
								Prefix: "/test",
							},
						},
						Action: &gloov1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: utils.ResourceRefPtr(tu.Upstream.Metadata.Ref()),
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

	It("Routes to GRPC Functions", func() {

		vs := getGrpcVs()
		_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		body := []byte(`{"str": "foo"}`)

		testRequest := func() (string, error) {
			// send a request with a body
			var buf bytes.Buffer
			buf.Write(body)
			res, err := http.Post(fmt.Sprintf("http://%s:%d/test", "localhost", defaults.HttpPort), "application/json", &buf)
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

	It("Routes to GRPC Functions with parameters", func() {

		vs := getGrpcVs()
		grpc := vs.VirtualHost.Routes[0].GetRouteAction().GetSingle().GetDestinationSpec().GetGrpc()
		grpc.Parameters = &transformation.Parameters{
			Path: &types.StringValue{Value: "/test/{str}"},
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
