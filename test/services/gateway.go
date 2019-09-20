package services

import (
	"net"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"

	"github.com/solo-io/solo-kit/test/helpers"

	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/service"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"

	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"

	gatewaysyncer "github.com/solo-io/gloo/projects/gateway/pkg/syncer"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"

	"context"
	"sync/atomic"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewayv2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"google.golang.org/grpc"

	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"go.uber.org/zap"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	fds_syncer "github.com/solo-io/gloo/projects/discovery/pkg/fds/syncer"
	uds_syncer "github.com/solo-io/gloo/projects/discovery/pkg/uds/syncer"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"

	"k8s.io/client-go/kubernetes"
)

type TestClients struct {
	GatewayClient        gatewayv2.GatewayClient
	VirtualServiceClient gatewayv1.VirtualServiceClient
	ProxyClient          gloov1.ProxyClient
	UpstreamClient       gloov1.UpstreamClient
	SecretClient         gloov1.SecretClient
	ServiceClient        skkube.ServiceClient
	GlooPort             int
}

var glooPortBase = int32(30400)

func AllocateGlooPort() int32 {
	return atomic.AddInt32(&glooPortBase, 1) + int32(config.GinkgoConfig.ParallelNode*1000)
}

func RunGateway(ctx context.Context, justGloo bool) TestClients {
	ns := defaults.GlooSystem
	ro := &RunOptions{
		NsToWrite: ns,
		NsToWatch: []string{"default", ns},
		WhatToRun: What{
			DisableGateway: justGloo,
		},
		KubeClient: helpers.MustKubeClient(),
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
	ValidationPort   int32
	ExtensionConfigs *gloov1.Extensions
	Extensions       syncer.Extensions
	Cache            memory.InMemoryResourceCache
	KubeClient       kubernetes.Interface
	ConsulClient     consul.ConsulWatcher
}

//noinspection GoUnhandledErrorResult
func RunGlooGatewayUdsFds(ctx context.Context, runOptions *RunOptions) TestClients {
	if runOptions.GlooPort == 0 {
		runOptions.GlooPort = AllocateGlooPort()
	}
	if runOptions.ValidationPort == 0 {
		runOptions.ValidationPort = AllocateGlooPort()
	}

	if runOptions.Cache == nil {
		runOptions.Cache = memory.NewInMemoryResourceCache()
	}

	settings := &gloov1.Settings{
		WatchNamespaces:    runOptions.NsToWatch,
		DiscoveryNamespace: runOptions.NsToWrite,
	}
	ctx = settingsutil.WithSettings(ctx, settings)

	glooOpts := defaultGlooOpts(ctx, runOptions)

	glooOpts.ControlPlane.BindAddr.(*net.TCPAddr).Port = int(runOptions.GlooPort)
	glooOpts.ValidationServer.BindAddr.(*net.TCPAddr).Port = int(runOptions.ValidationPort)
	if !runOptions.WhatToRun.DisableGateway {
		opts := defaultTestConstructOpts(ctx, runOptions)
		go gatewaysyncer.RunGateway(opts)
	}

	glooOpts.Settings = &gloov1.Settings{
		Extensions: runOptions.ExtensionConfigs,
	}
	glooOpts.ControlPlane.StartGrpcServer = true
	glooOpts.ValidationServer.StartGrpcServer = true
	go syncer.RunGlooWithExtensions(glooOpts, runOptions.Extensions)
	if !runOptions.WhatToRun.DisableFds {
		go func() {
			defer GinkgoRecover()
			fds_syncer.RunFDS(glooOpts)
		}()
	}
	if !runOptions.WhatToRun.DisableUds {
		go func() {
			defer GinkgoRecover()
			uds_syncer.RunUDS(glooOpts)
		}()
	}

	testClients := getTestClients(runOptions.Cache, glooOpts.KubeServiceClient)
	testClients.GlooPort = int(runOptions.GlooPort)
	return testClients
}

func getTestClients(cache memory.InMemoryResourceCache, serviceClient skkube.ServiceClient) TestClients {

	// construct our own resources:
	memFactory := &factory.MemoryResourceClientFactory{
		Cache: cache,
	}

	gatewayClient, err := gatewayv2.NewGatewayClient(memFactory)
	Expect(err).NotTo(HaveOccurred())
	virtualServiceClient, err := gatewayv1.NewVirtualServiceClient(memFactory)
	Expect(err).NotTo(HaveOccurred())
	upstreamClient, err := gloov1.NewUpstreamClient(memFactory)
	Expect(err).NotTo(HaveOccurred())
	secretClient, err := gloov1.NewSecretClient(memFactory)
	Expect(err).NotTo(HaveOccurred())
	proxyClient, err := gloov1.NewProxyClient(memFactory)
	Expect(err).NotTo(HaveOccurred())

	return TestClients{
		GatewayClient:        gatewayClient,
		VirtualServiceClient: virtualServiceClient,
		UpstreamClient:       upstreamClient,
		SecretClient:         secretClient,
		ProxyClient:          proxyClient,
		ServiceClient:        serviceClient,
	}
}

func defaultTestConstructOpts(ctx context.Context, runOptions *RunOptions) gatewaysyncer.Opts {
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
		RouteTables:     f,
		Proxies:         f,
		WatchOpts: clients.WatchOpts{
			Ctx:         ctx,
			RefreshRate: time.Minute,
		},
		DevMode: false,
	}
}

func defaultGlooOpts(ctx context.Context, runOptions *RunOptions) bootstrap.Opts {
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
	grpcServerValidation := grpc.NewServer(grpc.StreamInterceptor(
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
	var kubeCoreCache corecache.KubeCoreCache
	if runOptions.KubeClient != nil {
		var err error
		kubeCoreCache, err = cache.NewKubeCoreCacheWithOptions(ctx, runOptions.KubeClient, time.Hour, runOptions.NsToWatch)
		Expect(err).NotTo(HaveOccurred())
	}

	return bootstrap.Opts{
		WriteNamespace:    runOptions.NsToWrite,
		Upstreams:         f,
		UpstreamGroups:    f,
		Proxies:           f,
		Secrets:           f,
		Artifacts:         f,
		AuthConfigs:       f,
		KubeServiceClient: newServiceClient(ctx, f, runOptions),
		WatchNamespaces:   runOptions.NsToWatch,
		WatchOpts: clients.WatchOpts{
			Ctx:         ctx,
			RefreshRate: time.Second / 10,
		},
		ControlPlane: syncer.NewControlPlane(ctx, grpcServer, &net.TCPAddr{
			IP:   net.ParseIP("0.0.0.0"),
			Port: 8081,
		}, nil, true),
		ValidationServer: syncer.NewValidationServer(ctx, grpcServerValidation, &net.TCPAddr{
			IP:   net.ParseIP("0.0.0.0"),
			Port: 8081,
		}, true),

		KubeClient:    runOptions.KubeClient,
		KubeCoreCache: kubeCoreCache,
		DevMode:       true,
		ConsulWatcher: runOptions.ConsulClient,
	}
}

func newServiceClient(ctx context.Context, memFactory *factory.MemoryResourceClientFactory, runOpts *RunOptions) skkube.ServiceClient {

	// If the KubeClient option is set, the kubernetes discovery plugin will be activated and we must provide a
	// kubernetes service client in order for service-derived upstreams to be included in the snapshot
	if kube := runOpts.KubeClient; kube != nil {
		kubeCache, err := cache.NewKubeCoreCache(ctx, kube)
		if err != nil {
			panic(err)
		}
		return service.NewServiceClient(kube, kubeCache)
	}

	// Else return in-memory client
	client, err := skkube.NewServiceClient(memFactory)
	if err != nil {
		panic(err)
	}
	return client
}
