package services

import (
	"net"
	"time"

	gatewaysyncer "github.com/solo-io/gloo/projects/gateway/pkg/syncer"

	"context"
	"sync/atomic"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"google.golang.org/grpc"

	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"go.uber.org/zap"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"

	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	fds_syncer "github.com/solo-io/gloo/projects/discovery/pkg/fds/syncer"
	uds_syncer "github.com/solo-io/gloo/projects/discovery/pkg/uds/syncer"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"

	"k8s.io/client-go/kubernetes"
)

type TestClients struct {
	GatewayClient        gatewayv1.GatewayClient
	VirtualServiceClient gatewayv1.VirtualServiceClient
	ProxyClient          gloov1.ProxyClient
	UpstreamClient       gloov1.UpstreamClient
	SecretClient         gloov1.SecretClient
	GlooPort             int
}

var glooPortBase int32 = int32(30400)

func AllocateGlooPort() int32 {
	return atomic.AddInt32(&glooPortBase, 1) + int32(config.GinkgoConfig.ParallelNode*1000)
}

func RunGateway(ctx context.Context, justgloo bool) TestClients {
	ns := defaults.GlooSystem
	ro := &RunOptions{
		NsToWrite: ns,
		NsToWatch: []string{"default", ns},
		WhatToRun: What{
			DisableGateway: justgloo,
		},
	}
	return RunGlooGatewayUdsFds(ctx, ro)
}

type What struct {
	DisableGateway bool
	DisableUds     bool
	DisableFds     bool
}

type RunOptions struct {
	NsToWrite        string
	NsToWatch        []string
	WhatToRun        What
	GlooPort         int32
	ExtensionConfigs *gloov1.Extensions
	Extensions       syncer.Extensions
	Cache            memory.InMemoryResourceCache
	KubeClient       kubernetes.Interface
}

func RunGlooGatewayUdsFds(ctx context.Context, runOptions *RunOptions) TestClients {
	if runOptions.GlooPort == 0 {
		runOptions.GlooPort = AllocateGlooPort()
	}

	if runOptions.Cache == nil {
		runOptions.Cache = memory.NewInMemoryResourceCache()
	}

	glooOpts := DefaultGlooOpts(ctx, runOptions)
	glooOpts.BindAddr.(*net.TCPAddr).Port = int(runOptions.GlooPort)
	if !runOptions.WhatToRun.DisableGateway {
		opts := DefaultTestConstructOpts(ctx, runOptions)
		go gatewaysyncer.RunGateway(opts)
	}

	glooOpts.Settings = &gloov1.Settings{
		Extensions: runOptions.ExtensionConfigs,
	}
	glooOpts.ControlPlane.StartGrpcServer = true
	go syncer.RunGlooWithExtensions(glooOpts, runOptions.Extensions)
	if !runOptions.WhatToRun.DisableFds {
		go fds_syncer.RunFDS(glooOpts)
	}
	if !runOptions.WhatToRun.DisableUds {
		go uds_syncer.RunUDS(glooOpts)
	}

	testclients := GetTestClients(runOptions.Cache)
	testclients.GlooPort = int(runOptions.GlooPort)
	return testclients
}

func GetTestClients(cache memory.InMemoryResourceCache) TestClients {

	// construct our own resources:
	factory := &factory.MemoryResourceClientFactory{
		Cache: cache,
	}

	gatewayClient, err := gatewayv1.NewGatewayClient(factory)
	Expect(err).NotTo(HaveOccurred())
	virtualServiceClient, err := gatewayv1.NewVirtualServiceClient(factory)
	Expect(err).NotTo(HaveOccurred())
	upstreamClient, err := gloov1.NewUpstreamClient(factory)
	Expect(err).NotTo(HaveOccurred())
	secretClient, err := gloov1.NewSecretClient(factory)
	Expect(err).NotTo(HaveOccurred())
	proxyClient, err := gloov1.NewProxyClient(factory)
	Expect(err).NotTo(HaveOccurred())

	return TestClients{
		GatewayClient:        gatewayClient,
		VirtualServiceClient: virtualServiceClient,
		UpstreamClient:       upstreamClient,
		SecretClient:         secretClient,
		ProxyClient:          proxyClient,
	}
}

func DefaultTestConstructOpts(ctx context.Context, runOptions *RunOptions) gatewaysyncer.Opts {
	ctx = contextutils.WithLogger(ctx, "gateway")
	ctx = contextutils.SilenceLogger(ctx)
	f := &factory.MemoryResourceClientFactory{
		Cache: runOptions.Cache,
	}

	return gatewaysyncer.Opts{
		WriteNamespace:  runOptions.NsToWrite,
		WatchNamespaces: runOptions.NsToWatch,
		Gateways:        f,
		VirtualServices: f,
		Proxies:         f,
		WatchOpts: clients.WatchOpts{
			Ctx:         ctx,
			RefreshRate: time.Minute,
		},
		DevMode: false,
	}
}

func DefaultGlooOpts(ctx context.Context, runOptions *RunOptions) bootstrap.Opts {
	ctx = contextutils.WithLogger(ctx, "gloo")
	logger := contextutils.LoggerFrom(ctx)
	grpcServer := grpc.NewServer(grpc.StreamInterceptor(
		grpc_middleware.ChainStreamServer(
			grpc_ctxtags.StreamServerInterceptor(),
			grpc_zap.StreamServerInterceptor(zap.NewNop()),
			func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
				logger.Infof("gRPC call: %v", info.FullMethod)
				return handler(srv, ss)
			},
		)),
	)
	f := &factory.MemoryResourceClientFactory{
		Cache: runOptions.Cache,
	}
	return bootstrap.Opts{
		WriteNamespace:  runOptions.NsToWrite,
		Upstreams:       f,
		UpstreamGroups:  f,
		Proxies:         f,
		Secrets:         f,
		Artifacts:       f,
		WatchNamespaces: runOptions.NsToWatch,
		WatchOpts: clients.WatchOpts{
			Ctx:         ctx,
			RefreshRate: time.Second / 10,
		},
		ControlPlane: syncer.NewControlPlane(ctx, grpcServer, nil, true),
		BindAddr: &net.TCPAddr{
			IP:   net.ParseIP("0.0.0.0"),
			Port: 8081,
		},
		KubeClient: runOptions.KubeClient,
		DevMode:    true,
	}
}
