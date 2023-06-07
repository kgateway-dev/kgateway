package e2e_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/solo-io/gloo/test/services/envoy"

	"github.com/solo-io/gloo/test/gomega/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pb "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/wrappers"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	gloov1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/go-utils/contextutils"
	rltypes "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type acceptOrDenyRateLimitServer struct {
	acceptAll bool
}

func (s *acceptOrDenyRateLimitServer) ShouldRateLimit(_ context.Context, req *pb.RateLimitRequest) (*pb.RateLimitResponse, error) {
	// users could implement their own logic in custom rate limit servers here
	// the request descriptors are present in the rate limit request object, e.g.
	Expect(req.Descriptors[0].Entries[0].Key).To(Equal("generic_key"))
	Expect(req.Descriptors[0].Entries[0].Value).To(Equal("test"))
	if s.acceptAll {
		return &pb.RateLimitResponse{
			OverallCode: pb.RateLimitResponse_OK,
		}, nil
	} else {
		return &pb.RateLimitResponse{
			OverallCode: pb.RateLimitResponse_OVER_LIMIT,
		}, nil
	}
}

// Returns the actions that should be used to generate the descriptors expected by the server.
func (s *acceptOrDenyRateLimitServer) getActionsForServer() []*rltypes.RateLimitActions {
	return []*rltypes.RateLimitActions{
		{
			Actions: []*rltypes.Action{{
				ActionSpecifier: &rltypes.Action_GenericKey_{
					GenericKey: &rltypes.Action_GenericKey{DescriptorValue: "test"},
				},
			}},
		},
	}
}

type metadataCheckingRateLimitServer struct {
	descriptorKey          string
	defaultDescriptorValue string
	metadataKey            string
	pathSegmentKey         string
	expectedMetadataValue  string
}

func (s *metadataCheckingRateLimitServer) ShouldRateLimit(ctx context.Context, req *pb.RateLimitRequest) (*pb.RateLimitResponse, error) {
	contextutils.LoggerFrom(ctx).Infow("rate limit request", zap.Any("req", req))

	Expect(req.Descriptors).To(HaveLen(1))
	Expect(req.Descriptors[0].Entries).To(HaveLen(1))

	descriptorEntry := req.Descriptors[0].Entries[0]
	Expect(descriptorEntry.GetKey()).To(Equal(s.descriptorKey))
	Expect(descriptorEntry.GetValue()).To(Or(Equal(s.expectedMetadataValue), Equal(s.defaultDescriptorValue)))

	if descriptorEntry.GetValue() == s.expectedMetadataValue {
		return &pb.RateLimitResponse{
			OverallCode: pb.RateLimitResponse_OVER_LIMIT,
		}, nil
	}

	return &pb.RateLimitResponse{
		OverallCode: pb.RateLimitResponse_OK,
	}, nil
}

// Returns the actions that should be used to generate the descriptors expected by the server.
func (s *metadataCheckingRateLimitServer) getActionsForServer() []*rltypes.RateLimitActions {
	return []*rltypes.RateLimitActions{
		{
			Actions: []*rltypes.Action{
				{
					ActionSpecifier: &rltypes.Action_Metadata{
						Metadata: &rltypes.Action_MetaData{
							DescriptorKey: s.descriptorKey,
							MetadataKey: &rltypes.Action_MetaData_MetadataKey{
								Key: s.metadataKey,
								Path: []*rltypes.Action_MetaData_MetadataKey_PathSegment{
									{
										Segment: &rltypes.Action_MetaData_MetadataKey_PathSegment_Key{
											Key: s.pathSegmentKey,
										},
									},
								},
							},
							DefaultValue: s.defaultDescriptorValue,
							Source:       rltypes.Action_MetaData_ROUTE_ENTRY,
						},
					},
				},
			},
		},
	}
}

var _ = Describe("Rate Limit", func() {

	var (
		ctx         context.Context
		cancel      context.CancelFunc
		testClients services.TestClients
	)

	const (
		rlPort = uint32(18081)
	)

	Context("with envoy", func() {

		var (
			envoyInstance *envoy.Instance
			testUpstream  *v1helpers.TestUpstream
			envoyPort     uint32
			srv           *grpc.Server
		)

		BeforeEach(func() {
			var err error
			envoyInstance, err = envoyFactory.NewEnvoyInstance()
			Expect(err).NotTo(HaveOccurred())

			envoyPort = envoy.HttpPort

			// add the rl service as a static upstream
			rlserver := &gloov1.Upstream{
				Metadata: &core.Metadata{
					Name:      "rl-server",
					Namespace: "default",
				},
				UseHttp2: &wrappers.BoolValue{Value: true},
				UpstreamType: &gloov1.Upstream_Static{
					Static: &gloov1static.UpstreamSpec{
						Hosts: []*gloov1static.Host{{
							Addr: envoyInstance.GlooAddr,
							Port: rlPort,
						}},
					},
				},
			}
			ref := rlserver.Metadata.Ref()
			rlSettings := &ratelimit.Settings{
				RatelimitServerRef:      ref,
				EnableXRatelimitHeaders: true,
			}

			ctx, cancel = context.WithCancel(context.Background())
			ro := &services.RunOptions{
				NsToWrite: defaults.GlooSystem,
				NsToWatch: []string{"default", defaults.GlooSystem},
				WhatToRun: services.What{
					DisableGateway: true,
					DisableUds:     true,
					DisableFds:     true,
				},
				Settings: &gloov1.Settings{
					RatelimitServer: rlSettings,
				},
			}

			testClients = services.RunGlooGatewayUdsFds(ctx, ro)
			_, err = testClients.UpstreamClient.Write(rlserver, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			err = helpers.WriteDefaultGateways(defaults.GlooSystem, testClients.GatewayClient)
			Expect(err).NotTo(HaveOccurred(), "Should be able to write the default gateways")

			err = envoyInstance.RunWithRoleAndRestXds(envoy.DefaultProxyName, testClients.GlooPort, testClients.RestXdsPort)
			Expect(err).NotTo(HaveOccurred())

			testUpstream = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
			var opts clients.WriteOpts
			up := testUpstream.Upstream
			_, err = testClients.UpstreamClient.Write(up, opts)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			envoyInstance.Clean()
			srv.GracefulStop()
			if cancel != nil {
				cancel()
			}
		})

		It("should rate limit", func() {
			rlService := &acceptOrDenyRateLimitServer{acceptAll: false}
			srv = startRateLimitServer(rlService, rlPort)

			hosts := map[string]*configForHost{"host1": {actionsForHost: rlService.getActionsForServer()}}
			proxy := getProxy(envoyPort, testUpstream.Upstream.Metadata.Ref(), hosts)
			_, err := testClients.ProxyClient.Write(proxy, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			EventuallyRateLimited("host1", envoyPort)
		})

		It("shouldn't rate limit", func() {
			rlService := &acceptOrDenyRateLimitServer{acceptAll: true}
			srv = startRateLimitServer(rlService, rlPort)

			hosts := map[string]*configForHost{"host1": {actionsForHost: rlService.getActionsForServer()}}
			proxy := getProxy(envoyPort, testUpstream.Upstream.Metadata.Ref(), hosts)
			_, err := testClients.ProxyClient.Write(proxy, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			ConsistentlyNotRateLimited("host1", envoyPort)
		})

		It("should rate limit based on route metadata", func() {
			rlService := &metadataCheckingRateLimitServer{
				descriptorKey:          "md-desc",
				defaultDescriptorValue: "default-value",
				metadataKey:            "io.solo.test",
				pathSegmentKey:         "foo",
				expectedMetadataValue:  "bar",
			}

			srv = startRateLimitServer(rlService, rlPort)

			hosts := map[string]*configForHost{
				"host1": {
					actionsForHost: rlService.getActionsForServer(),
					routeMetadata: map[string]*structpb.Struct{
						rlService.metadataKey: {
							Fields: map[string]*structpb.Value{
								rlService.pathSegmentKey: {
									Kind: &structpb.Value_StringValue{
										StringValue: rlService.expectedMetadataValue,
									},
								},
							},
						},
					},
				},
				"host2": {
					actionsForHost: rlService.getActionsForServer(),
					// no metadata here, so we should send the `defaultDescriptorValue` to the server
				},
			}
			proxy := getProxy(envoyPort, testUpstream.Upstream.Metadata.Ref(), hosts)
			_, err := testClients.ProxyClient.Write(proxy, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			// Host 1 should be rate limited
			EventuallyRateLimited("host1", envoyPort)

			// Host 2 should never be rate limited
			ConsistentlyNotRateLimited("host2", envoyPort)
		})
	})
})

func startRateLimitServer(service pb.RateLimitServiceServer, rlport uint32) *grpc.Server {
	srv := grpc.NewServer()
	pb.RegisterRateLimitServiceServer(srv, service)
	reflection.Register(srv)
	addr := fmt.Sprintf(":%d", rlport)
	lis, err := net.Listen("tcp", addr)
	Expect(err).To(BeNil())
	go func() {
		defer GinkgoRecover()
		err := srv.Serve(lis)
		Expect(err).ToNot(HaveOccurred())
	}()
	return srv
}

func EventuallyOk(hostname string, port uint32) {
	// wait for three seconds so gloo race can be waited out
	// it's possible gloo upstreams hit after the proxy does
	// (gloo resyncs once per second)
	time.Sleep(3 * time.Second)
	EventuallyWithOffset(1, func(g Gomega) {
		g.Expect(get(hostname, port)).To(matchers.HaveOkResponse())
	}, "5s", ".1s").Should(Succeed())
}

func ConsistentlyNotRateLimited(hostname string, port uint32) {
	// waiting for envoy to start, so that consistently works
	EventuallyOk(hostname, port)

	ConsistentlyWithOffset(1, func(g Gomega) {
		g.Expect(get(hostname, port)).To(matchers.HaveOkResponse())
	}, "5s", ".1s").Should(Succeed())
}

func EventuallyRateLimited(hostname string, port uint32) {
	EventuallyWithOffset(1, func(g Gomega) {
		g.Expect(get(hostname, port)).To(matchers.HaveStatusCode(http.StatusTooManyRequests))
	}, "5s", ".1s").Should(Succeed())
}

func get(hostname string, port uint32) (*http.Response, error) {
	parts := strings.SplitN(hostname, "/", 2)
	hostname = parts[0]
	path := "1"
	if len(parts) > 1 {
		path = parts[1]
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d/"+path, "localhost", port), nil)
	Expect(err).NotTo(HaveOccurred())

	// remove password part if exists
	parts = strings.SplitN(hostname, "@", 2)
	if len(parts) > 1 {
		hostname = parts[1]
		auth := strings.Split(parts[0], ":")
		req.SetBasicAuth(auth[0], auth[1])
	}

	req.Host = hostname
	return http.DefaultClient.Do(req)
}

func getProxy(envoyPort uint32, upstream *core.ResourceRef, hostsToRateLimits map[string]*configForHost) *gloov1.Proxy {

	rlb := RlProxyBuilder{
		envoyPort:         envoyPort,
		upstream:          upstream,
		hostsToRateLimits: hostsToRateLimits,
	}
	return rlb.getProxy()
}

type configForHost struct {
	actionsForHost []*rltypes.RateLimitActions
	routeMetadata  map[string]*structpb.Struct
}

type RlProxyBuilder struct {
	upstream          *core.ResourceRef
	hostsToRateLimits map[string]*configForHost
	envoyPort         uint32
}

func (b *RlProxyBuilder) getProxy() *gloov1.Proxy {
	var vhosts []*gloov1.VirtualHost

	for hostname, hostConfig := range b.hostsToRateLimits {
		vhost := &gloov1.VirtualHost{
			Name:    "gloo-system.virt" + hostname,
			Domains: []string{hostname},
			Routes: []*gloov1.Route{
				{
					Action: &gloov1.Route_RouteAction{
						RouteAction: &gloov1.RouteAction{
							Destination: &gloov1.RouteAction_Single{
								Single: &gloov1.Destination{
									DestinationType: &gloov1.Destination_Upstream{
										Upstream: b.upstream,
									},
								},
							},
						},
					},
				},
			},
		}

		if len(hostConfig.routeMetadata) > 0 {
			vhost.Routes[0].Options = &gloov1.RouteOptions{
				EnvoyMetadata: hostConfig.routeMetadata,
			}
		}

		if len(hostConfig.actionsForHost) > 0 {
			vhost.Options = &gloov1.VirtualHostOptions{
				RateLimitConfigType: &gloov1.VirtualHostOptions_Ratelimit{
					Ratelimit: &ratelimit.RateLimitVhostExtension{
						RateLimits: hostConfig.actionsForHost,
					},
				},
			}
		}
		vhosts = append(vhosts, vhost)
	}

	p := &gloov1.Proxy{
		Metadata: &core.Metadata{
			Name:      "proxy",
			Namespace: "default",
		},
		Listeners: []*gloov1.Listener{{
			Name:        "listener",
			BindAddress: "0.0.0.0",
			BindPort:    b.envoyPort,
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					VirtualHosts: vhosts,
				},
			},
		}},
	}

	return p
}
