package e2e_test

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/solo-io/gloo/test/services/envoy"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	pb "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/gogo/googleapis/google/rpc"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
)

var _ = Describe("CustomAuth", func() {

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		envoyInstance *envoy.Instance
		testUpstream  *v1helpers.TestUpstream
		testClients   services.TestClients
		srv           *grpc.Server
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		// Initialize Envoy instance
		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

		// Start custom extauth server and create upstream for it
		srv, err = startCustomExtauthServer(8095)
		Expect(err).NotTo(HaveOccurred())

		customAuthServerUs := &gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      "custom-auth",
				Namespace: "default",
			},
			UseHttp2: &wrappers.BoolValue{Value: true},
			UpstreamType: &gloov1.Upstream_Static{
				Static: &static.UpstreamSpec{
					Hosts: []*static.Host{{
						// this is a safe way of referring to localhost
						Addr: envoyInstance.GlooAddr,
						Port: 8095,
					}},
				},
			},
		}
		authUsRef := customAuthServerUs.Metadata.Ref()

		// Start Gloo
		testClients = services.RunGlooGatewayUdsFds(ctx, &services.RunOptions{
			NsToWrite: defaults.GlooSystem,
			NsToWatch: []string{"default", defaults.GlooSystem},
			WhatToRun: services.What{
				DisableGateway: true,
				DisableFds:     true,
				DisableUds:     true,
			},
			Settings: &gloov1.Settings{
				Extauth: &v1.Settings{
					ExtauthzServerRef: authUsRef,
				},
			},
		})

		// Create static upstream for auth server
		_, err = testClients.UpstreamClient.Write(customAuthServerUs, clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		// Run envoy
		err = envoyInstance.RunWithRoleAndRestXds(envoy.DefaultProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())

		// Create a test upstream
		testUpstream = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
		_, err = testClients.UpstreamClient.Write(testUpstream.Upstream, clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		// Create a proxy routing to the upstream and wait for it to be accepted
		proxy := getProxyExtAuth("default", "proxy", envoy.HttpPort, testUpstream.Upstream.Metadata.Ref())

		_, err = testClients.ProxyClient.Write(proxy, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return testClients.ProxyClient.Read(proxy.Metadata.Namespace, proxy.Metadata.Name, clients.ReadOpts{})
		})
	})

	AfterEach(func() {
		envoyInstance.Clean()
		srv.GracefulStop()
		cancel()
	})

	It("works as expected", func() {
		client := &http.Client{}

		getRequest := func(prefix string) *http.Request {
			req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d/%s", "localhost", envoy.HttpPort, prefix), nil)
			Expect(err).NotTo(HaveOccurred())
			return req
		}

		expectResponseForUser := func(req *http.Request, username string, expectedStatus int) {
			req.Header.Set("user", username)

			Eventually(func() (int, error) {
				resp, err := client.Do(req)
				if err != nil {
					return 0, err
				}
				return resp.StatusCode, nil
			}, "5s", "0.5s").Should(Equal(expectedStatus))
		}
		publicRoute := getRequest("public")
		userRoute := getRequest("user")
		adminRoute := getRequest("admin")

		// Public route, everyone allowed
		expectResponseForUser(publicRoute, "unknown", http.StatusOK)
		expectResponseForUser(publicRoute, "john", http.StatusOK)
		expectResponseForUser(publicRoute, "jane", http.StatusOK)

		// User route, only users and admins are allowed
		expectResponseForUser(userRoute, "unknown", http.StatusForbidden)
		expectResponseForUser(userRoute, "john", http.StatusOK)
		expectResponseForUser(userRoute, "jane", http.StatusOK)

		// Admin route, only admins are allowed
		expectResponseForUser(adminRoute, "unknown", http.StatusForbidden)
		expectResponseForUser(adminRoute, "john", http.StatusForbidden)
		expectResponseForUser(adminRoute, "jane", http.StatusOK)

	})
})

func getProxyExtAuth(namespace, name string, envoyPort uint32, upstream *core.ResourceRef) *gloov1.Proxy {
	return &gloov1.Proxy{
		Metadata: &core.Metadata{
			Name:      name,
			Namespace: namespace,
		},
		Listeners: []*gloov1.Listener{{
			Name:        "listener",
			BindAddress: "0.0.0.0",
			BindPort:    envoyPort,
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					VirtualHosts: []*gloov1.VirtualHost{
						{
							Name:    "gloo-system.virt1",
							Domains: []string{"*"},
							Options: &gloov1.VirtualHostOptions{
								Extauth: &v1.ExtAuthExtension{
									Spec: &v1.ExtAuthExtension_CustomAuth{
										CustomAuth: &v1.CustomAuth{
											ContextExtensions: map[string]string{
												"must-be": "user", // Only authenticated users can access this vhost
											},
										},
									},
								},
							},
							Routes: []*gloov1.Route{
								{ // This route can be accessed by users
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/user",
										},
									}},
									Options: &gloov1.RouteOptions{
										PrefixRewrite: &wrappers.StringValue{Value: "/"},
									},
									Action: &gloov1.Route_RouteAction{
										RouteAction: &gloov1.RouteAction{
											Destination: &gloov1.RouteAction_Single{
												Single: &gloov1.Destination{
													DestinationType: &gloov1.Destination_Upstream{
														Upstream: upstream,
													},
												},
											},
										},
									},
								},
								{ // This route can be accessed only by admins
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/admin",
										},
									}},
									Options: &gloov1.RouteOptions{
										PrefixRewrite: &wrappers.StringValue{Value: "/"},
										Extauth: &v1.ExtAuthExtension{
											Spec: &v1.ExtAuthExtension_CustomAuth{
												CustomAuth: &v1.CustomAuth{
													ContextExtensions: map[string]string{
														"must-be": "admin", // Only authenticated users can access this vhost
													},
												},
											},
										},
									},
									Action: &gloov1.Route_RouteAction{
										RouteAction: &gloov1.RouteAction{
											Destination: &gloov1.RouteAction_Single{
												Single: &gloov1.Destination{
													DestinationType: &gloov1.Destination_Upstream{
														Upstream: upstream,
													},
												},
											},
										},
									},
								},
								{ // This route can be accessed by anyone
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/public",
										},
									}},
									Options: &gloov1.RouteOptions{
										PrefixRewrite: &wrappers.StringValue{Value: "/"},
										Extauth: &v1.ExtAuthExtension{
											Spec: &v1.ExtAuthExtension_Disable{
												Disable: true,
											},
										},
									},
									Action: &gloov1.Route_RouteAction{
										RouteAction: &gloov1.RouteAction{
											Destination: &gloov1.RouteAction_Single{
												Single: &gloov1.Destination{
													DestinationType: &gloov1.Destination_Upstream{
														Upstream: upstream,
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
			},
		}},
	}
}

func startCustomExtauthServer(port uint) (*grpc.Server, error) {
	srv := grpc.NewServer()
	pb.RegisterAuthorizationServer(srv, &customAuthServer{
		// maps a username to a user type
		users: map[string]string{
			"john": "user",
			"jane": "admin",
		},
	})

	addr := fmt.Sprintf(":%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	go func() {
		defer GinkgoRecover()
		err := srv.Serve(lis)
		Expect(err).ToNot(HaveOccurred())
	}()
	return srv, nil
}

var (
	deny  = &pb.CheckResponse{Status: &status.Status{Code: int32(rpc.PERMISSION_DENIED)}}
	allow = &pb.CheckResponse{Status: &status.Status{Code: int32(rpc.OK)}}
)

// This custom auth server expects requests to provide the username in a "user" header.
// It checks the username against an internal set of users and denies the request if the user is not known or
// if they don't match the expected user type.
type customAuthServer struct {
	users map[string]string // maps a username to a user type
}

func (c *customAuthServer) Check(ctx context.Context, request *pb.CheckRequest) (*pb.CheckResponse, error) {
	ctxExtensions := request.GetAttributes().GetContextExtensions()

	if len(ctxExtensions) == 0 {
		return deny, nil
	}

	requiredUserType, ok := ctxExtensions["must-be"]
	if !ok {
		return deny, nil
	}

	headers := request.GetAttributes().GetRequest().GetHttp().GetHeaders()
	if len(headers) == 0 {
		return deny, nil
	}

	username, ok := headers["user"]
	if !ok {
		return deny, nil
	}

	actualUserType, ok := c.users[username]
	if !ok {
		return deny, nil
	}

	// If we require an admin, only admin users are allowed
	if requiredUserType == "admin" {
		if actualUserType == "admin" {
			return allow, nil
		}
		return deny, nil
	}

	// If we require a user, both users and admin users are allowed
	if requiredUserType == "user" {
		if actualUserType == "admin" || actualUserType == "user" {
			return allow, nil
		}
		return deny, nil
	}

	return deny, nil
}
