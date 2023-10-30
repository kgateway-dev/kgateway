package services

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector/singlereplica"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"

	"github.com/solo-io/gloo/pkg/utils/statusutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"

	"github.com/solo-io/gloo/projects/gateway/pkg/translator"

	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"

	"github.com/solo-io/solo-kit/test/helpers"

	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/service"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"

	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"

	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"

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
	fds_syncer "github.com/solo-io/gloo/projects/discovery/pkg/fds/syncer"
	uds_syncer "github.com/solo-io/gloo/projects/discovery/pkg/uds/syncer"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"k8s.io/client-go/kubernetes"
)

type TestClients struct {
	GatewayClient        gatewayv1.GatewayClient
	HttpGatewayClient    gatewayv1.MatchableHttpGatewayClient
	VirtualServiceClient gatewayv1.VirtualServiceClient
	ProxyClient          gloov1.ProxyClient
	UpstreamClient       gloov1.UpstreamClient
	SecretClient         gloov1.SecretClient
	ArtifactClient       gloov1.ArtifactClient
	ServiceClient        skkube.ServiceClient
	GlooPort             int
	RestXdsPort          int
}

// WriteSnapshot writes all resources in the ApiSnapshot to the cache
func (c TestClients) WriteSnapshot(ctx context.Context, snapshot *gloosnapshot.ApiSnapshot) error {
	// We intentionally create child resources first to avoid having the validation webhook reject
	// the parent resource

	writeOptions := clients.WriteOpts{
		Ctx:               ctx,
		OverwriteExisting: false,
	}
	for _, secret := range snapshot.Secrets {
		if _, writeErr := c.SecretClient.Write(secret, writeOptions); writeErr != nil {
			return writeErr
		}
	}
	for _, artifact := range snapshot.Artifacts {
		if _, writeErr := c.ArtifactClient.Write(artifact, writeOptions); writeErr != nil {
			return writeErr
		}
	}
	for _, us := range snapshot.Upstreams {
		if _, writeErr := c.UpstreamClient.Write(us, writeOptions); writeErr != nil {
			return writeErr
		}
	}
	for _, vs := range snapshot.VirtualServices {
		if _, writeErr := c.VirtualServiceClient.Write(vs, writeOptions); writeErr != nil {
			return writeErr
		}
	}
	for _, hgw := range snapshot.HttpGateways {
		if _, writeErr := c.HttpGatewayClient.Write(hgw, writeOptions); writeErr != nil {
			return writeErr
		}
	}
	for _, gw := range snapshot.Gateways {
		if _, writeErr := c.GatewayClient.Write(gw, writeOptions); writeErr != nil {
			return writeErr
		}
	}
	for _, proxy := range snapshot.Proxies {
		if _, writeErr := c.ProxyClient.Write(proxy, writeOptions); writeErr != nil {
			return writeErr
		}
	}

	return nil
}

// DeleteSnapshot deletes all resources in the ApiSnapshot from the cache
func (c TestClients) DeleteSnapshot(ctx context.Context, snapshot *gloosnapshot.ApiSnapshot) error {
	// We intentionally delete resources in the reverse order that we create resources
	// If we delete child resources first, the validation webhook may reject the change

	deleteOptions := clients.DeleteOpts{
		Ctx:            ctx,
		IgnoreNotExist: true,
	}

	for _, gw := range snapshot.Gateways {
		gwNamespace, gwName := gw.GetMetadata().Ref().Strings()
		if deleteErr := c.GatewayClient.Delete(gwNamespace, gwName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}
	for _, hgw := range snapshot.HttpGateways {
		hgwNamespace, hgwName := hgw.GetMetadata().Ref().Strings()
		if deleteErr := c.HttpGatewayClient.Delete(hgwNamespace, hgwName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}
	for _, vs := range snapshot.VirtualServices {
		vsNamespace, vsName := vs.GetMetadata().Ref().Strings()
		if deleteErr := c.VirtualServiceClient.Delete(vsNamespace, vsName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}
	for _, us := range snapshot.Upstreams {
		usNamespace, usName := us.GetMetadata().Ref().Strings()
		if deleteErr := c.UpstreamClient.Delete(usNamespace, usName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}
	for _, artifact := range snapshot.Artifacts {
		artifactNamespace, artifactName := artifact.GetMetadata().Ref().Strings()
		if deleteErr := c.ArtifactClient.Delete(artifactNamespace, artifactName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}
	for _, secret := range snapshot.Secrets {
		secretNamespace, secretName := secret.GetMetadata().Ref().Strings()
		if deleteErr := c.SecretClient.Delete(secretNamespace, secretName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}

	// Proxies are auto generated by Gateway resources
	// Therefore we delete Proxies after we have deleted the resources that may regenerate a Proxy
	for _, proxy := range snapshot.Proxies {
		proxyNamespace, proxyName := proxy.GetMetadata().Ref().Strings()
		if deleteErr := c.ProxyClient.Delete(proxyNamespace, proxyName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}

	return nil
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
	Cache            memory.InMemoryResourceCache
	KubeClient       kubernetes.Interface
	ConsulClient     consul.ConsulWatcher
	ConsulDnsAddress string
}

// noinspection GoUnhandledErrorResult
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

	if glooOpts.Settings == nil {
		glooOpts.Settings = &gloov1.Settings{}
	}
	if glooOpts.Settings.GetGloo() == nil {
		glooOpts.Settings.Gloo = &gloov1.GlooOptions{}
	}
	if glooOpts.Settings.GetGloo().GetRestXdsBindAddr() == "" {
		glooOpts.Settings.GetGloo().RestXdsBindAddr = fmt.Sprintf("%s:%d", net.IPv4zero.String(), runOptions.RestXdsPort)
	}
	glooOpts.ControlPlane.StartGrpcServer = true
	glooOpts.ValidationServer.StartGrpcServer = true
	glooOpts.GatewayControllerEnabled = !runOptions.WhatToRun.DisableGateway

	go setup.RunGloo(glooOpts)

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
	httpGatewayClient, err := gatewayv1.NewMatchableHttpGatewayClient(ctx, memFactory)
	Expect(err).NotTo(HaveOccurred())
	virtualServiceClient, err := gatewayv1.NewVirtualServiceClient(ctx, memFactory)
	Expect(err).NotTo(HaveOccurred())
	upstreamClient, err := gloov1.NewUpstreamClient(ctx, memFactory)
	Expect(err).NotTo(HaveOccurred())
	secretClient, err := gloov1.NewSecretClient(ctx, memFactory)
	Expect(err).NotTo(HaveOccurred())
	artifactClient, err := gloov1.NewArtifactClient(ctx, memFactory)
	Expect(err).NotTo(HaveOccurred())
	proxyClient, err := gloov1.NewProxyClient(ctx, memFactory)
	Expect(err).NotTo(HaveOccurred())

	return TestClients{
		GatewayClient:        gatewayClient,
		HttpGatewayClient:    httpGatewayClient,
		VirtualServiceClient: virtualServiceClient,
		UpstreamClient:       upstreamClient,
		SecretClient:         secretClient,
		ArtifactClient:       artifactClient,
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
	if runOptions.Settings.GetGateway().GetValidation().GetProxyValidationServerAddr() != "" {
		if validation == nil {
			validation = &translator.ValidationOpts{}
		}
		validation.ProxyValidationServerAddress = runOptions.Settings.GetGateway().GetValidation().GetProxyValidationServerAddr()
	}
	if runOptions.Settings.GetGateway().GetValidation().GetAllowWarnings() != nil {
		if validation == nil {
			validation = &translator.ValidationOpts{}
		}
		validation.AllowWarnings = runOptions.Settings.GetGateway().GetValidation().GetAllowWarnings().GetValue()
	}
	if runOptions.Settings.GetGateway().GetValidation().GetAlwaysAccept() != nil {
		if validation == nil {
			validation = &translator.ValidationOpts{}
		}
		validation.AlwaysAcceptResources = runOptions.Settings.GetGateway().GetValidation().GetAlwaysAccept().GetValue()
	}
	return translator.Opts{
		GlooNamespace:           meta.GetNamespace(),
		WriteNamespace:          runOptions.NsToWrite,
		WatchNamespaces:         runOptions.NsToWatch,
		StatusReporterNamespace: statusutils.GetStatusReporterNamespaceOrDefault(defaults.GlooSystem),
		Gateways:                f,
		MatchableHttpGateways:   f,
		VirtualServices:         f,
		RouteTables:             f,
		VirtualHostOptions:      f,
		RouteOptions:            f,
		Proxies:                 f,
		WatchOpts: clients.WatchOpts{
			Ctx:         ctx,
			RefreshRate: time.Minute,
		},
		Validation:             validation,
		DevMode:                false,
		ConfigStatusMetricOpts: runOptions.Settings.GetObservabilityOptions().GetConfigStatusMetricLabels(),
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
	var validationOpts *translator.ValidationOpts
	if runOptions.Settings.GetGateway().GetValidation().GetProxyValidationServerAddr() != "" {
		if validationOpts == nil {
			validationOpts = &translator.ValidationOpts{}
		}
		validationOpts.ProxyValidationServerAddress = runOptions.Settings.GetGateway().GetValidation().GetProxyValidationServerAddr()
	}
	if runOptions.Settings.GetGateway().GetValidation().GetAllowWarnings() != nil {
		if validationOpts == nil {
			validationOpts = &translator.ValidationOpts{}
		}
		validationOpts.AllowWarnings = runOptions.Settings.GetGateway().GetValidation().GetAllowWarnings().GetValue()
	}
	if runOptions.Settings.GetGateway().GetValidation().GetAlwaysAccept() != nil {
		if validationOpts == nil {
			validationOpts = &translator.ValidationOpts{}
		}
		validationOpts.AlwaysAcceptResources = runOptions.Settings.GetGateway().GetValidation().GetAlwaysAccept().GetValue()
	}
	return bootstrap.Opts{
		Settings:                runOptions.Settings,
		WriteNamespace:          runOptions.NsToWrite,
		StatusReporterNamespace: statusutils.GetStatusReporterNamespaceOrDefault(defaults.GlooSystem),
		Upstreams:               f,
		UpstreamGroups:          f,
		Proxies:                 f,
		Secrets:                 f,
		Artifacts:               f,
		AuthConfigs:             f,
		RateLimitConfigs:        f,
		GraphQLApis:             f,
		Gateways:                f,
		MatchableHttpGateways:   f,
		VirtualServices:         f,
		RouteTables:             f,
		RouteOptions:            f,
		VirtualHostOptions:      f,
		KubeServiceClient:       newServiceClient(ctx, f, runOptions),
		WatchNamespaces:         runOptions.NsToWatch,
		WatchOpts: clients.WatchOpts{
			Ctx:         ctx,
			RefreshRate: time.Second / 10,
		},
		ControlPlane: setup.NewControlPlane(ctx, grpcServer, &net.TCPAddr{
			IP:   net.IPv4zero,
			Port: 8081,
		}, nil, true),
		ValidationServer: setup.NewValidationServer(ctx, grpcServerValidation, &net.TCPAddr{
			IP:   net.IPv4zero,
			Port: 8081,
		}, true),
		ProxyDebugServer: setup.NewProxyDebugServer(ctx, grpcServer, &net.TCPAddr{
			IP:   net.IPv4zero,
			Port: 8001,
		}, false),
		KubeClient:    runOptions.KubeClient,
		KubeCoreCache: kubeCoreCache,
		DevMode:       true,
		Consul: bootstrap.Consul{
			ConsulWatcher: runOptions.ConsulClient,
			DnsServer:     runOptions.ConsulDnsAddress,
		},
		GatewayControllerEnabled: true,
		ValidationOpts:           validationOpts,
		Identity:                 singlereplica.Identity(),
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
