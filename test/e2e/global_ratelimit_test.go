//go:build ignore

package e2e_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gloov1 "github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1"
	gloov1static "github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1/options/static"
	"github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/services"
	"github.com/kgateway-dev/kgateway/v2/test/services/envoy"
	"github.com/kgateway-dev/kgateway/v2/test/v1helpers"

	pb "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// EnvoyRateLimitServer provides a mocked rate limit service implementation
// that can test different descriptor scenarios
type EnvoyRateLimitServer struct {
	// Domain to match
	Domain string

	// Control whether all requests are limited
	LimitAll bool

	// Records all descriptors received for verification
	ReceivedDescriptors []*pb.RateLimitDescriptor

	// Specific descriptor keys to check and limit
	// If empty, all requests will be handled based on LimitAll
	LimitDescriptors map[string]string

	// If true, return an error to simulate service unavailable
	SimulateServiceFailure bool
}

func (s *EnvoyRateLimitServer) ShouldRateLimit(ctx context.Context, req *pb.RateLimitRequest) (*pb.RateLimitResponse, error) {
	logger := contextutils.LoggerFrom(ctx)
	logger.Infow("rate limit request received", zap.Any("request", req))

	// Verify domain
	Expect(req.Domain).To(Equal(s.Domain), "Domain should match expected value")

	// Simulate service failure if configured
	if s.SimulateServiceFailure {
		return nil, fmt.Errorf("simulated service failure")
	}

	// Store received descriptors for later verification
	s.ReceivedDescriptors = append(s.ReceivedDescriptors, req.Descriptors...)

	// If configured to limit all requests
	if s.LimitAll {
		return &pb.RateLimitResponse{
			OverallCode: pb.RateLimitResponse_OVER_LIMIT,
		}, nil
	}

	// If no specific descriptors to check, allow
	if len(s.LimitDescriptors) == 0 {
		return &pb.RateLimitResponse{
			OverallCode: pb.RateLimitResponse_OK,
		}, nil
	}

	// Check for specific descriptor match
	for _, descriptor := range req.Descriptors {
		for _, entry := range descriptor.Entries {
			expectedValue, exists := s.LimitDescriptors[entry.Key]
			if exists && entry.Value == expectedValue {
				return &pb.RateLimitResponse{
					OverallCode: pb.RateLimitResponse_OVER_LIMIT,
				}, nil
			}
		}
	}

	// No specific matches found
	return &pb.RateLimitResponse{
		OverallCode: pb.RateLimitResponse_OK,
	}, nil
}

var _ = Describe("Global Rate Limit with Envoy Approach", Serial, func() {
	// These tests use the Serial decorator because they rely on a hard-coded port for the RateLimit server

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *envoy.Instance
		testUpstream  *v1helpers.TestUpstream
		envoyPort     uint32
		srv           *grpc.Server
		rlServer      *EnvoyRateLimitServer
	)

	const (
		rlPort = uint32(18082) // Using a different port than the other test
		domain = "kgateway-test-domain"
	)

	BeforeEach(func() {
		envoyInstance = envoyFactory.NewInstance()
		envoyPort = envoyInstance.HttpPort

		// Create a rate limit service upstream
		rlservice := &gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      "rl-service",
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

		// Configure rate limit settings
		ref := rlservice.Metadata.Ref()
		rlSettings := &ratelimit.Settings{
			RatelimitServerRef:      ref,
			EnableXRatelimitHeaders: true,
		}

		// Start the gateway and services
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

		// Write the rate limit service upstream
		_, err := testClients.UpstreamClient.Write(rlservice, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		// Write default gateways
		err = helpers.WriteDefaultGateways(defaults.GlooSystem, testClients.GatewayClient)
		Expect(err).NotTo(HaveOccurred(), "Should be able to write the default gateways")

		// Run Envoy
		err = envoyInstance.RunWithRoleAndRestXds(envoy.DefaultProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())

		// Create test upstream
		testUpstream = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
		_, err = testClients.UpstreamClient.Write(testUpstream.Upstream, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		// Create a new rate limit server instance with default configuration
		rlServer = &EnvoyRateLimitServer{
			Domain:           domain,
			LimitAll:         false,
			LimitDescriptors: make(map[string]string),
		}
	})

	AfterEach(func() {
		if envoyInstance != nil {
			envoyInstance.Clean()
		}
		if srv != nil {
			srv.GracefulStop()
		}
		if cancel != nil {
			cancel()
		}
	})

	// Helper to start rate limit gRPC server
	startRateLimitServer := func() {
		srv = grpc.NewServer()
		pb.RegisterRateLimitServiceServer(srv, rlServer)
		reflection.Register(srv)

		addr := fmt.Sprintf(":%d", rlPort)
		lis, err := net.Listen("tcp", addr)
		Expect(err).NotTo(HaveOccurred())

		go func() {
			defer GinkgoRecover()
			err := srv.Serve(lis)
			Expect(err).ToNot(HaveOccurred())
		}()

		// Give server time to start
		time.Sleep(100 * time.Millisecond)
	}

	// Helper to create a proxy with rate limit configuration
	createRateLimitProxy := func(virtualHosts []*gloov1.VirtualHost) {
		proxy := &gloov1.Proxy{
			Metadata: &core.Metadata{
				Name:      "rate-limit-proxy",
				Namespace: "default",
			},
			Listeners: []*gloov1.Listener{{
				Name:        "listener",
				BindAddress: "0.0.0.0",
				BindPort:    envoyPort,
				ListenerType: &gloov1.Listener_HttpListener{
					HttpListener: &gloov1.HttpListener{
						VirtualHosts: virtualHosts,
					},
				},
			}},
		}

		_, err := testClients.ProxyClient.Write(proxy, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		// Wait for proxy to be configured
		time.Sleep(2 * time.Second)
	}

	// Helper to create a virtual host with static rate limit actions
	createVirtualHostWithRateLimits := func(hostname string, actions []*ratelimit.RateLimitActions) *gloov1.VirtualHost {
		vhost := &gloov1.VirtualHost{
			Name:    "virt-" + hostname,
			Domains: []string{hostname},
			Routes: []*gloov1.Route{
				{
					Action: &gloov1.Route_RouteAction{
						RouteAction: &gloov1.RouteAction{
							Destination: &gloov1.RouteAction_Single{
								Single: &gloov1.Destination{
									DestinationType: &gloov1.Destination_Upstream{
										Upstream: testUpstream.Upstream.Metadata.Ref(),
									},
								},
							},
						},
					},
				},
			},
		}

		if actions != nil {
			vhost.Options = &gloov1.VirtualHostOptions{
				RateLimitConfigType: &gloov1.VirtualHostOptions_Ratelimit{
					Ratelimit: &ratelimit.RateLimitVhostExtension{
						RateLimits:      actions,
						RateLimitDomain: domain, // Set the domain to match our server
					},
				},
			}
		}

		return vhost
	}

	// Helper for making a request
	makeRequest := func(hostname string) (*http.Response, error) {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%d/test", envoyPort), nil)
		Expect(err).NotTo(HaveOccurred())
		req.Host = hostname
		return http.DefaultClient.Do(req)
	}

	// Test scenario: Basic rate limiting with no specific descriptors
	Context("basic rate limiting", func() {
		It("should rate limit when server returns OVER_LIMIT", func() {
			// Configure rate limit server to limit all requests
			rlServer.LimitAll = true
			startRateLimitServer()

			// Create virtual host with basic rate limit action
			actions := []*ratelimit.RateLimitActions{
				{
					Actions: []*ratelimit.Action{
						{
							ActionSpecifier: &ratelimit.Action_GenericKey_{
								GenericKey: &ratelimit.Action_GenericKey{
									DescriptorValue: "test-value",
									DescriptorKey:   "test-key",
								},
							},
						},
					},
				},
			}

			vhost := createVirtualHostWithRateLimits("rate-limited-host", actions)
			createRateLimitProxy([]*gloov1.VirtualHost{vhost})

			// Expect the request to be rate limited
			EventuallyWithOffset(1, func(g Gomega) {
				resp, err := makeRequest("rate-limited-host")
				g.Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				g.Expect(resp.StatusCode).To(Equal(http.StatusTooManyRequests))
			}, "5s", "0.1s").Should(Succeed())

			// Verify descriptors were received
			Expect(rlServer.ReceivedDescriptors).NotTo(BeEmpty(), "Should have received descriptors")
		})

		It("should not rate limit when server returns OK", func() {
			// Configure rate limit server to allow all requests
			rlServer.LimitAll = false
			startRateLimitServer()

			// Create virtual host with basic rate limit action
			actions := []*ratelimit.RateLimitActions{
				{
					Actions: []*ratelimit.Action{
						{
							ActionSpecifier: &ratelimit.Action_GenericKey_{
								GenericKey: &ratelimit.Action_GenericKey{
									DescriptorValue: "test-value",
									DescriptorKey:   "test-key",
								},
							},
						},
					},
				},
			}

			vhost := createVirtualHostWithRateLimits("non-limited-host", actions)
			createRateLimitProxy([]*gloov1.VirtualHost{vhost})

			// Expect the request not to be rate limited
			ConsistentlyWithOffset(1, func(g Gomega) {
				resp, err := makeRequest("non-limited-host")
				g.Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				g.Expect(resp).To(matchers.HaveOkResponse())
			}, "5s", "0.1s").Should(Succeed())

			// Verify descriptors were received
			Expect(rlServer.ReceivedDescriptors).NotTo(BeEmpty(), "Should have received descriptors")
		})
	})

	// Test scenario: Rate limiting based on specific descriptor values
	Context("descriptor-based rate limiting", func() {
		BeforeEach(func() {
			// Configure the service to limit specific descriptors
			rlServer.LimitDescriptors = map[string]string{
				"remote_address": "1.2.3.4",
				"path":           "/test",
				"user-id":        "test-user",
			}
			startRateLimitServer()
		})

		It("should rate limit based on remote address", func() {
			// Configure virtual host with remote address descriptor
			actions := []*ratelimit.RateLimitActions{
				{
					Actions: []*ratelimit.Action{
						{
							ActionSpecifier: &ratelimit.Action_RemoteAddress_{
								RemoteAddress: &ratelimit.Action_RemoteAddress{},
							},
						},
					},
				},
			}

			vhost := createVirtualHostWithRateLimits("remote-addr-host", actions)
			createRateLimitProxy([]*gloov1.VirtualHost{vhost})

			// Make a request and check that descriptors were sent
			resp, err := makeRequest("remote-addr-host")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			// Verify that a remote_address descriptor was sent to service
			found := false
			for _, desc := range rlServer.ReceivedDescriptors {
				for _, entry := range desc.Entries {
					if entry.Key == "remote_address" {
						found = true
						break
					}
				}
			}
			Expect(found).To(BeTrue(), "Should have received a remote_address descriptor")
		})

		It("should rate limit based on request header", func() {
			// Configure virtual host with header descriptor
			actions := []*ratelimit.RateLimitActions{
				{
					Actions: []*ratelimit.Action{
						{
							ActionSpecifier: &ratelimit.Action_RequestHeaders_{
								RequestHeaders: &ratelimit.Action_RequestHeaders{
									HeaderName:    "X-User-ID",
									DescriptorKey: "user-id",
								},
							},
						},
					},
				},
			}

			vhost := createVirtualHostWithRateLimits("header-host", actions)
			createRateLimitProxy([]*gloov1.VirtualHost{vhost})

			// Make a request with the header
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%d/test", envoyPort), nil)
			Expect(err).NotTo(HaveOccurred())
			req.Host = "header-host"
			req.Header.Set("X-User-ID", "test-user") // This matches our limited value

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			// Verify that a header descriptor was sent to service
			found := false
			for _, desc := range rlServer.ReceivedDescriptors {
				for _, entry := range desc.Entries {
					if entry.Key == "user-id" {
						found = true
						break
					}
				}
			}
			Expect(found).To(BeTrue(), "Should have received a user-id header descriptor")
		})
	})

	// Test scenario: FailOpen behavior
	Context("fail open behavior", func() {
		// Helper to create a virtual host with rate limit actions and fail open setting
		createVirtualHostWithFailOpen := func(hostname string, failOpen bool) *gloov1.VirtualHost {
			vhost := &gloov1.VirtualHost{
				Name:    "virt-" + hostname,
				Domains: []string{hostname},
				Routes: []*gloov1.Route{
					{
						Action: &gloov1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: testUpstream.Upstream.Metadata.Ref(),
										},
									},
								},
							},
						},
					},
				},
				Options: &gloov1.VirtualHostOptions{
					RateLimitConfigType: &gloov1.VirtualHostOptions_Ratelimit{
						Ratelimit: &ratelimit.RateLimitVhostExtension{
							RateLimits: []*ratelimit.RateLimitActions{
								{
									Actions: []*ratelimit.Action{
										{
											ActionSpecifier: &ratelimit.Action_GenericKey_{
												GenericKey: &ratelimit.Action_GenericKey{
													DescriptorValue: "test",
													DescriptorKey:   "test-key",
												},
											},
										},
									},
								},
							},
							RateLimitDomain: domain,
							FailOpen:        failOpen,
						},
					},
				},
			}
			return vhost
		}

		BeforeEach(func() {
			// Configure rate limit server to simulate failure
			rlServer.SimulateServiceFailure = true
			startRateLimitServer()
		})

		It("should allow requests when fail open is true and service fails", func() {
			// Create virtual host with fail open = true
			vhost := createVirtualHostWithFailOpen("fail-open-host", true)
			createRateLimitProxy([]*gloov1.VirtualHost{vhost})

			// Requests should be allowed because of fail open setting
			ConsistentlyWithOffset(1, func(g Gomega) {
				resp, err := makeRequest("fail-open-host")
				g.Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				g.Expect(resp).To(matchers.HaveOkResponse())
			}, "5s", "0.1s").Should(Succeed())
		})

		It("should reject requests when fail open is false and service fails", func() {
			// Create virtual host with fail open = false
			vhost := createVirtualHostWithFailOpen("fail-closed-host", false)
			createRateLimitProxy([]*gloov1.VirtualHost{vhost})

			// Requests should be rejected because of fail closed setting
			EventuallyWithOffset(1, func(g Gomega) {
				resp, err := makeRequest("fail-closed-host")
				g.Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				g.Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
			}, "5s", "0.1s").Should(Succeed())
		})
	})
})
