package services

import (
	"fmt"
	"net"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	extauthExt "github.com/solo-io/gloo/projects/gloo/pkg/syncer/extauth"
	ratelimitExt "github.com/solo-io/gloo/projects/gloo/pkg/syncer/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"

	gatewaysyncer "github.com/solo-io/gloo/projects/gateway/pkg/syncer"

	"github.com/solo-io/gloo/projects/gateway/pkg/translator"

	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"

	"github.com/solo-io/solo-kit/test/helpers"

	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/service"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"

	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"

	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"

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

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	fds_syncer "github.com/solo-io/gloo/projects/discovery/pkg/fds/syncer"
	uds_syncer "github.com/solo-io/gloo/projects/discovery/pkg/uds/syncer"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"k8s.io/client-go/kubernetes"
)

type TestClients struct {
	GatewayClient        gatewayv1.GatewayClient
	VirtualServiceClient gatewayv1.VirtualServiceClient
	ProxyClient          gloov1.ProxyClient
	UpstreamClient       gloov1.UpstreamClient
	SecretClient         gloov1.SecretClient
	ServiceClient        skkube.ServiceClient
	GlooPort             int
	RestXdsPort          int
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
	RestXdsPort      int32
	Settings         *gloov1.Settings
	Extensions       setup.Extensions
	Cache            memory.InMemoryResourceCache
	KubeClient       kubernetes.Interface
	ConsulClient     consul.ConsulWatcher
	ConsulDnsAddress string
}

//noinspection GoUnhandledErrorResult
func RunGlooGatewayUdsFds(ctx context.Context, runOptions *RunOptions) TestClients {
	if runOptions.GlooPort == 0 {
		runOptions.GlooPort = AllocateGlooPort()
	}
	if runOptions.ValidationPort == 0 {
		runOptions.ValidationPort = AllocateGlooPort()
	}
	if runOptions.RestXdsPort == 0 {
		runOptions.RestXdsPort = AllocateGlooPort()
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

	glooOpts.Settings = runOptions.Settings
	if glooOpts.Settings == nil {
		glooOpts.Settings = &gloov1.Settings{}
	}
	if glooOpts.Settings.GetGloo() == nil {
		glooOpts.Settings.Gloo = &gloov1.GlooOptions{}
	}
	if glooOpts.Settings.GetGloo().GetRestXdsBindAddr() == "" {
		glooOpts.Settings.GetGloo().RestXdsBindAddr = fmt.Sprintf("0.0.0.0:%v", int(runOptions.RestXdsPort))
	}

	runOptions.Extensions.SyncerExtensions = []syncer.TranslatorSyncerExtensionFactory{
		ratelimitExt.NewTranslatorSyncerExtension,
		extauthExt.NewTranslatorSyncerExtension,
	}

	glooOpts.ControlPlane.StartGrpcServer = true
	glooOpts.ValidationServer.StartGrpcServer = true
	go setup.RunGlooWithExtensions(glooOpts, runOptions.Extensions, make(chan struct{}))

	// gloo is dependency of gateway, needs to run second if we want to test validation
	if !runOptions.WhatToRun.DisableGateway {
		opts := defaultTestConstructOpts(ctx, runOptions)
		go gatewaysyncer.RunGateway(opts)
	}

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

	testClients := getTestClients(ctx, runOptions.Cache, glooOpts.KubeServiceClient)
	testClients.GlooPort = int(runOptions.GlooPort)
	testClients.RestXdsPort = int(runOptions.RestXdsPort)
	return testClients
}

func getTestClients(ctx context.Context, cache memory.InMemoryResourceCache, serviceClient skkube.ServiceClient) TestClients {

	// construct our own resources:
	memFactory := &factory.MemoryResourceClientFactory{
		Cache: cache,
	}

	gatewayClient, err := gatewayv1.NewGatewayClient(ctx, memFactory)
	Expect(err).NotTo(HaveOccurred())
	virtualServiceClient, err := gatewayv1.NewVirtualServiceClient(ctx, memFactory)
	Expect(err).NotTo(HaveOccurred())
	upstreamClient, err := gloov1.NewUpstreamClient(ctx, memFactory)
	Expect(err).NotTo(HaveOccurred())
	secretClient, err := gloov1.NewSecretClient(ctx, memFactory)
	Expect(err).NotTo(HaveOccurred())
	proxyClient, err := gloov1.NewProxyClient(ctx, memFactory)
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

func defaultTestConstructOpts(ctx context.Context, runOptions *RunOptions) translator.Opts {
	ctx = contextutils.WithLogger(ctx, "gateway")
	f := &factory.MemoryResourceClientFactory{
		Cache: runOptions.Cache,
	}

	meta := runOptions.Settings.GetMetadata()

	var validation *translator.ValidationOpts
	if runOptions.Settings != nil && runOptions.Settings.GetGateway() != nil && runOptions.Settings.GetGateway().GetValidation() != nil {
		validation = &translator.ValidationOpts{}
		if runOptions.Settings.GetGateway().GetValidation().GetProxyValidationServerAddr() != "" {
			validation.ProxyValidationServerAddress = runOptions.Settings.GetGateway().GetValidation().ProxyValidationServerAddr
		}
		if runOptions.Settings.GetGateway().GetValidation().GetAllowWarnings() != nil {
			validation.AllowWarnings = runOptions.Settings.GetGateway().GetValidation().GetAllowWarnings().Value

		}
		if runOptions.Settings.GetGateway().GetValidation().GetAlwaysAccept() != nil {
			validation.AlwaysAcceptResources = runOptions.Settings.GetGateway().GetValidation().GetAlwaysAccept().Value
		}

	}

	return translator.Opts{
		GlooNamespace:      meta.GetNamespace(),
		WriteNamespace:     runOptions.NsToWrite,
		WatchNamespaces:    runOptions.NsToWatch,
		Gateways:           f,
		VirtualServices:    f,
		RouteTables:        f,
		VirtualHostOptions: f,
		RouteOptions:       f,
		Proxies:            f,
		WatchOpts: clients.WatchOpts{
			Ctx:         ctx,
			RefreshRate: time.Minute,
		},
		Validation: validation,
		DevMode:    false,
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
		Settings:          runOptions.Settings,
		WriteNamespace:    runOptions.NsToWrite,
		Upstreams:         f,
		UpstreamGroups:    f,
		Proxies:           f,
		Secrets:           f,
		Artifacts:         f,
		AuthConfigs:       f,
		RateLimitConfigs:  f,
		KubeServiceClient: newServiceClient(ctx, f, runOptions),
		WatchNamespaces:   runOptions.NsToWatch,
		WatchOpts: clients.WatchOpts{
			Ctx:         ctx,
			RefreshRate: time.Second / 10,
		},
		ControlPlane: setup.NewControlPlane(ctx, grpcServer, &net.TCPAddr{
			IP:   net.ParseIP("0.0.0.0"),
			Port: 8081,
		}, nil, true),
		ValidationServer: setup.NewValidationServer(ctx, grpcServerValidation, &net.TCPAddr{
			IP:   net.ParseIP("0.0.0.0"),
			Port: 8081,
		}, true),

		KubeClient:    runOptions.KubeClient,
		KubeCoreCache: kubeCoreCache,
		DevMode:       true,
		Consul: bootstrap.Consul{
			ConsulWatcher: runOptions.ConsulClient,
			DnsServer:     runOptions.ConsulDnsAddress,
		},
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
	client, err := skkube.NewServiceClient(ctx, memFactory)
	if err != nil {
		panic(err)
	}
	return client
}
