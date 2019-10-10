package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	pb "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/ratelimit"
	gloov1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	ratelimit2 "github.com/solo-io/gloo/projects/gloo/pkg/plugins/ratelimit"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type acceptOrDenyRateLimitServer struct {
	acceptAll bool
}

func (this *acceptOrDenyRateLimitServer) ShouldRateLimit(ctx context.Context, req *pb.RateLimitRequest) (*pb.RateLimitResponse, error) {
	// users could implement their own logic in custom rate limit servers here
	// the request descriptors are present in the rate limit request object, e.g.
	Expect(req.Descriptors[0].Entries[0].Key).To(Equal("generic_key"))
	Expect(req.Descriptors[0].Entries[0].Value).To(Equal("test"))
	if this.acceptAll {
		return &pb.RateLimitResponse{
			OverallCode: pb.RateLimitResponse_OK,
		}, nil
	} else {
		return &pb.RateLimitResponse{
			OverallCode: pb.RateLimitResponse_OVER_LIMIT,
		}, nil
	}
}

var _ = Describe("Rate Limit", func() {

	var (
		ctx            context.Context
		testClients    services.TestClients
		glooExtensions map[string]*types.Struct
		cache          memory.InMemoryResourceCache
	)

	const (
		rlport = uint32(18081)
	)

	Context("with envoy", func() {

		var (
			envoyInstance *services.EnvoyInstance
			testUpstream  *v1helpers.TestUpstream
			envoyPort     = uint32(8081)
			srv           *grpc.Server
		)

		BeforeEach(func() {
			var err error
			envoyInstance, err = envoyFactory.NewEnvoyInstance()
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			// add the rl service as a static upstream
			rlserver := &gloov1.Upstream{
				Metadata: core.Metadata{
					Name:      "rl-server",
					Namespace: "default",
				},
				UpstreamSpec: &gloov1.UpstreamSpec{
					UseHttp2: true,
					UpstreamType: &gloov1.UpstreamSpec_Static{
						Static: &gloov1static.UpstreamSpec{
							Hosts: []*gloov1static.Host{{
								Addr: envoyInstance.GlooAddr,
								Port: rlport,
							}},
						},
					},
				},
			}
			ref := rlserver.Metadata.Ref()
			rlSettings := &ratelimit.Settings{
				RatelimitServerRef: &ref,
			}
			settingsStruct, err := envoyutil.MessageToStruct(rlSettings)
			Expect(err).NotTo(HaveOccurred())

			glooExtensions = map[string]*types.Struct{
				ratelimit2.ExtensionName: settingsStruct,
			}

			extensions := &gloov1.Extensions{
				Configs: glooExtensions,
			}
			ctx, _ = context.WithCancel(context.Background())
			cache = memory.NewInMemoryResourceCache()
			ro := &services.RunOptions{
				NsToWrite: defaults.GlooSystem,
				NsToWatch: []string{"default", defaults.GlooSystem},
				WhatToRun: services.What{
					DisableGateway: true,
					DisableUds:     true,
					DisableFds:     true,
				},
				ExtensionConfigs: extensions,
				Cache:            cache,
			}
			testClients = services.RunGlooGatewayUdsFds(ctx, ro)
			_, err = testClients.UpstreamClient.Write(rlserver, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			err = envoyInstance.Run(testClients.GlooPort)
			Expect(err).NotTo(HaveOccurred())

			testUpstream = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
			// drain channel as we dont care about it
			go func() {
				for range testUpstream.C {
				}
			}()
			var opts clients.WriteOpts
			up := testUpstream.Upstream
			_, err = testClients.UpstreamClient.Write(up, opts)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if envoyInstance != nil {
				envoyInstance.Clean()
			}
			srv.GracefulStop()
		})

		It("should rate limit", func() {
			srv = startSimpleRateLimitServer(false, rlport)

			hosts := map[string]bool{"host1": true}
			proxy := getProxy(envoyPort, testUpstream.Upstream.Metadata.Ref(), hosts)
			_, err := testClients.ProxyClient.Write(proxy, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			EventuallyRateLimited("host1", envoyPort)
		})

		It("shouldn't rate limit", func() {
			srv = startSimpleRateLimitServer(true, rlport)

			hosts := map[string]bool{"host1": true}
			proxy := getProxy(envoyPort, testUpstream.Upstream.Metadata.Ref(), hosts)
			_, err := testClients.ProxyClient.Write(proxy, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			// waiting for envoy to start, so that consistently works
			EventuallyOk("host1", envoyPort)

			ConsistentlyNotRateLimited("host1", envoyPort)
		})
	})
})

func startSimpleRateLimitServer(acceptAll bool, rlport uint32) *grpc.Server {
	service := acceptOrDenyRateLimitServer{acceptAll: acceptAll}
	srv := grpc.NewServer()
	pb.RegisterRateLimitServiceServer(srv, &service)
	reflection.Register(srv)
	addr := fmt.Sprintf(":%d", rlport)
	lis, err := net.Listen("tcp", addr)
	Expect(err).To(BeNil())
	go func() {
		defer GinkgoRecover()
		err := srv.Serve(lis)
		Expect(err).ToNot(HaveOccurred())
	}()
	Expect(err).To(BeNil())
	return srv
}

func EventuallyOk(hostname string, port uint32) {
	EventuallyWithOffset(1, func() error {
		res, err := get(hostname, port)
		if err != nil {
			return err
		}
		if res.StatusCode != http.StatusOK {
			return errors.New(fmt.Sprintf("%v is not OK", res.StatusCode))
		}
		return nil
	}, "5s", ".1s").Should(BeNil())
}

func ConsistentlyNotRateLimited(hostname string, port uint32) {
	ConsistentlyWithOffset(1, func() error {
		res, err := get(hostname, port)
		if err != nil {
			return err
		}
		if res.StatusCode != http.StatusOK {
			return errors.New(fmt.Sprintf("%v is not OK", res.StatusCode))
		}
		return nil
	}, "5s", ".1s").Should(BeNil())
}

func EventuallyRateLimited(hostname string, port uint32) {
	EventuallyWithOffset(1, func() error {
		res, err := get(hostname, port)
		if err != nil {
			return err
		}
		if res.StatusCode != http.StatusTooManyRequests {
			return errors.New(fmt.Sprintf("%v is not TooManyRequests", res.StatusCode))
		}
		return nil
	}, "5s", ".1s").Should(BeNil())
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

func getProxy(envoyPort uint32, upstream core.ResourceRef, hostsToRateLimits map[string]bool) *gloov1.Proxy {
	rlVhostExt := &ratelimit.RateLimitVhostExtension{
		RateLimits: []*ratelimit.RateLimitActions{
			{
				Actions: []*ratelimit.Action{{
					ActionSpecifier: &ratelimit.Action_GenericKey_{
						GenericKey: &ratelimit.Action_GenericKey{DescriptorValue: "test"},
					},
				}},
			},
		},
	}
	rlb := RlProxyBuilder{
		envoyPort:         envoyPort,
		upstream:          upstream,
		hostsToRateLimits: hostsToRateLimits,
		customRateLimit:   rlVhostExt,
	}
	return rlb.getProxy()
}

type RlProxyBuilder struct {
	customRateLimit   *ratelimit.RateLimitVhostExtension
	upstream          core.ResourceRef
	hostsToRateLimits map[string]bool
	envoyPort         uint32
}

func (b *RlProxyBuilder) getProxy() *gloov1.Proxy {
	var extensions *gloov1.Extensions

	rateLimitStruct, err := envoyutil.MessageToStruct(b.customRateLimit)
	Expect(err).NotTo(HaveOccurred())
	protos := map[string]*types.Struct{
		ratelimit2.EnvoyExtensionName: rateLimitStruct,
	}

	extensions = &gloov1.Extensions{
		Configs: protos,
	}

	var vhosts []*gloov1.VirtualHost

	for hostname, enableRateLimits := range b.hostsToRateLimits {
		vhost := &gloov1.VirtualHost{
			Name:    "gloo-system.virt" + hostname,
			Domains: []string{hostname},
			Routes: []*gloov1.Route{
				{
					Matcher: &gloov1.Matcher{
						PathSpecifier: &gloov1.Matcher_Prefix{
							Prefix: "/",
						},
					},
					Action: &gloov1.Route_RouteAction{
						RouteAction: &gloov1.RouteAction{
							Destination: &gloov1.RouteAction_Single{
								Single: &gloov1.Destination{
									DestinationType: &gloov1.Destination_Upstream{
										Upstream: utils.ResourceRefPtr(b.upstream),
									},
								},
							},
						},
					},
				},
			},
		}

		if enableRateLimits {
			vhost.VirtualHostPlugins = &gloov1.VirtualHostPlugins{
				Extensions: extensions,
			}
		}
		vhosts = append(vhosts, vhost)
	}

	p := &gloov1.Proxy{
		Metadata: core.Metadata{
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
