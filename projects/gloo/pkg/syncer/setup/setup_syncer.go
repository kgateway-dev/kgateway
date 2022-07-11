package setup

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/solo-io/gloo/projects/gateway/pkg/utils/metrics"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"

	gloostatusutils "github.com/solo-io/gloo/pkg/utils/statusutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"

	"github.com/golang/protobuf/ptypes/duration"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	consulapi "github.com/hashicorp/consul/api"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/pkg/utils/channelutils"
	"github.com/solo-io/gloo/pkg/utils/setuputils"
	gateway "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	rlv1alpha1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	consulplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	sslutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	xdsserver "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/types"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	extauthExt "github.com/solo-io/gloo/projects/gloo/pkg/syncer/extauth"
	ratelimitExt "github.com/solo-io/gloo/projects/gloo/pkg/syncer/ratelimit"
)

type RunFunc func(opts bootstrap.Opts) error

func NewSetupFunc() setuputils.SetupFunc {
	return NewSetupFuncWithRunAndExtensions(RunGloo, nil)
}

// used outside of this repo
//noinspection GoUnusedExportedFunction
func NewSetupFuncWithExtensions(extensions Extensions) setuputils.SetupFunc {
	runWithExtensions := func(opts bootstrap.Opts) error {
		return RunGlooWithExtensions(opts, extensions, make(chan struct{}))
	}
	return NewSetupFuncWithRunAndExtensions(runWithExtensions, &extensions)
}

// for use by UDS, FDS, other v1.SetupSyncers
func NewSetupFuncWithRun(runFunc RunFunc) setuputils.SetupFunc {
	return NewSetupFuncWithRunAndExtensions(runFunc, nil)
}

func NewSetupFuncWithRunAndExtensions(runFunc RunFunc, extensions *Extensions) setuputils.SetupFunc {
	s := &setupSyncer{
		extensions: extensions,
		makeGrpcServer: func(ctx context.Context, options ...grpc.ServerOption) *grpc.Server {
			serverOpts := []grpc.ServerOption{
				grpc.StreamInterceptor(
					grpc_middleware.ChainStreamServer(
						grpc_ctxtags.StreamServerInterceptor(),
						grpc_zap.StreamServerInterceptor(zap.NewNop()),
						func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
							contextutils.LoggerFrom(ctx).Debugf("gRPC call: %v", info.FullMethod)
							return handler(srv, ss)
						},
					)),
			}
			serverOpts = append(serverOpts, options...)
			return grpc.NewServer(serverOpts...)
		},
		runFunc: runFunc,
	}
	return s.Setup
}

// grpcServer contains grpc server configuration fields we will need to persist after starting a server
// to later check if they changed and we need to trigger a server restart
type grpcServer struct {
	addr            string
	maxGrpcRecvSize int
	cancel          context.CancelFunc
}

type setupSyncer struct {
	extensions               *Extensions
	runFunc                  RunFunc
	makeGrpcServer           func(ctx context.Context, options ...grpc.ServerOption) *grpc.Server
	previousXdsServer        grpcServer
	previousValidationServer grpcServer
	controlPlane             bootstrap.ControlPlane
	validationServer         bootstrap.ValidationServer
	callbacks                xdsserver.Callbacks
}

func NewControlPlane(ctx context.Context, grpcServer *grpc.Server, bindAddr net.Addr, callbacks xdsserver.Callbacks, start bool) bootstrap.ControlPlane {
	hasher := &xds.ProxyKeyHasher{}
	settings := cache.CacheSettings{
		Ads:    true,
		Hash:   hasher,
		Logger: contextutils.LoggerFrom(ctx),
	}
	snapshotCache := cache.NewSnapshotCache(settings)
	xdsServer := server.NewServer(ctx, snapshotCache, callbacks)
	reflection.Register(grpcServer)

	return bootstrap.ControlPlane{
		GrpcService: &bootstrap.GrpcService{
			GrpcServer:      grpcServer,
			StartGrpcServer: start,
			BindAddr:        bindAddr,
			Ctx:             ctx,
		},
		SnapshotCache: snapshotCache,
		XDSServer:     xdsServer,
	}
}

func NewValidationServer(ctx context.Context, grpcServer *grpc.Server, bindAddr net.Addr, start bool) bootstrap.ValidationServer {
	return bootstrap.ValidationServer{
		GrpcService: &bootstrap.GrpcService{
			GrpcServer:      grpcServer,
			StartGrpcServer: start,
			BindAddr:        bindAddr,
			Ctx:             ctx,
		},
		Server: validation.NewValidationServer(),
	}
}

var (
	DefaultXdsBindAddr        = fmt.Sprintf("0.0.0.0:%v", defaults.GlooXdsPort)
	DefaultValidationBindAddr = fmt.Sprintf("0.0.0.0:%v", defaults.GlooValidationPort)
	DefaultRestXdsBindAddr    = fmt.Sprintf("0.0.0.0:%v", defaults.GlooRestXdsPort)
)

func getAddr(addr string) (*net.TCPAddr, error) {
	addrParts := strings.Split(addr, ":")
	if len(addrParts) != 2 {
		return nil, errors.Errorf("invalid bind addr: %v", addr)
	}
	ip := net.ParseIP(addrParts[0])

	port, err := strconv.Atoi(addrParts[1])
	if err != nil {
		return nil, errors.Wrapf(err, "invalid bind addr: %v", addr)
	}

	return &net.TCPAddr{IP: ip, Port: port}, nil
}

func (s *setupSyncer) Setup(ctx context.Context, kubeCache kube.SharedCache, memCache memory.InMemoryResourceCache, settings *v1.Settings) error {

	xdsAddr := settings.GetGloo().GetXdsBindAddr()
	if xdsAddr == "" {
		xdsAddr = DefaultXdsBindAddr
	}
	xdsTcpAddress, err := getAddr(xdsAddr)
	if err != nil {
		return errors.Wrapf(err, "parsing xds addr")
	}

	validationAddr := settings.GetGloo().GetValidationBindAddr()
	if validationAddr == "" {
		validationAddr = DefaultValidationBindAddr
	}
	validationTcpAddress, err := getAddr(validationAddr)
	if err != nil {
		return errors.Wrapf(err, "parsing validation addr")
	}

	refreshRate := time.Minute
	if settings.GetRefreshRate() != nil {
		refreshRate = prototime.DurationFromProto(settings.GetRefreshRate())
	}

	writeNamespace := settings.GetDiscoveryNamespace()
	if writeNamespace == "" {
		writeNamespace = defaults.GlooSystem
	}
	watchNamespaces := utils.ProcessWatchNamespaces(settings.GetWatchNamespaces(), writeNamespace)

	// process grpcserver options to understand if any servers will need a restart

	var maxGrpcRecvSize int
	// Use the same maxGrpcMsgSize as validation as this is determined by the size of proxies.
	if maxGrpcMsgSize := settings.GetGateway().GetValidation().GetValidationServerGrpcMaxSizeBytes(); maxGrpcMsgSize != nil {
		if maxGrpcMsgSize.GetValue() < 0 {
			return errors.Errorf("validationServerGrpcMaxSizeBytes in settings CRD must be non-negative, current value: %v", maxGrpcMsgSize.GetValue())
		}
		maxGrpcRecvSize = int(maxGrpcMsgSize.GetValue())
	}

	emptyControlPlane := bootstrap.ControlPlane{}
	emptyValidationServer := bootstrap.ValidationServer{}

	// check if we need to restart the control plane
	if xdsAddr != s.previousXdsServer.addr {
		if s.previousXdsServer.cancel != nil {
			s.previousXdsServer.cancel()
			s.previousXdsServer.cancel = nil
		}
		s.controlPlane = emptyControlPlane
	}

	// check if we need to restart the validation server
	if validationAddr != s.previousValidationServer.addr || maxGrpcRecvSize != s.previousValidationServer.maxGrpcRecvSize {
		if s.previousValidationServer.cancel != nil {
			s.previousValidationServer.cancel()
			s.previousValidationServer.cancel = nil
		}
		s.validationServer = emptyValidationServer
	}

	// initialize the control plane context in this block either on the first loop, or if bind addr changed
	if s.controlPlane == emptyControlPlane {
		// create new context as the grpc server might survive multiple iterations of this loop.
		ctx, cancel := context.WithCancel(context.Background())
		var callbacks xdsserver.Callbacks
		if s.extensions != nil {
			callbacks = s.extensions.XdsCallbacks
		}
		s.controlPlane = NewControlPlane(ctx, s.makeGrpcServer(ctx), xdsTcpAddress, callbacks, true)
		s.previousXdsServer.cancel = cancel
		s.previousXdsServer.addr = xdsAddr
	}

	// initialize the validation server context in this block either on the first loop, or if bind addr changed
	if s.validationServer == emptyValidationServer {
		// create new context as the grpc server might survive multiple iterations of this loop.
		ctx, cancel := context.WithCancel(context.Background())
		var validationGrpcServerOpts []grpc.ServerOption
		if maxGrpcRecvSize > 0 {
			validationGrpcServerOpts = append(validationGrpcServerOpts, grpc.MaxRecvMsgSize(maxGrpcRecvSize))
		}
		s.validationServer = NewValidationServer(ctx, s.makeGrpcServer(ctx, validationGrpcServerOpts...), validationTcpAddress, true)
		s.previousValidationServer.cancel = cancel
		s.previousValidationServer.addr = validationAddr
		s.previousValidationServer.maxGrpcRecvSize = maxGrpcRecvSize
	}

	consulClient, err := bootstrap.ConsulClientForSettings(ctx, settings)
	if err != nil {
		return err
	}

	var vaultClient *vaultapi.Client
	if vaultSettings := settings.GetVaultSecretSource(); vaultSettings != nil {
		vaultClient, err = bootstrap.VaultClientForSettings(vaultSettings)
		if err != nil {
			return err
		}
	}

	var clientset kubernetes.Interface
	opts, err := constructOpts(ctx,
		&clientset,
		kubeCache,
		consulClient,
		vaultClient,
		memCache,
		settings,
	)
	if err != nil {
		return err
	}
	opts.WriteNamespace = writeNamespace
	opts.StatusReporterNamespace = gloostatusutils.GetStatusReporterNamespaceOrDefault(writeNamespace)
	opts.WatchNamespaces = watchNamespaces
	opts.WatchOpts = clients.WatchOpts{
		Ctx:         ctx,
		RefreshRate: refreshRate,
	}
	opts.ControlPlane = s.controlPlane
	opts.ValidationServer = s.validationServer
	// if nil, kube plugin disabled
	opts.KubeClient = clientset
	opts.DevMode = settings.GetDevMode()
	opts.Settings = settings

	opts.Consul.DnsServer = settings.GetConsul().GetDnsAddress()
	if len(opts.Consul.DnsServer) == 0 {
		opts.Consul.DnsServer = consulplugin.DefaultDnsAddress
	}
	if pollingInterval := settings.GetConsul().GetDnsPollingInterval(); pollingInterval != nil {
		dnsPollingInterval := prototime.DurationFromProto(pollingInterval)
		opts.Consul.DnsPollingInterval = &dnsPollingInterval
	}

	// if vault service discovery specified, initialize consul watcher
	if consulServiceDiscovery := settings.GetConsul().GetServiceDiscovery(); consulServiceDiscovery != nil {
		// Set up Consul client
		consulClientWrapper, err := consul.NewConsulWatcher(consulClient, consulServiceDiscovery.GetDataCenters())
		if err != nil {
			return err
		}
		opts.Consul.ConsulWatcher = consulClientWrapper
	}

	err = s.runFunc(opts)

	s.validationServer.StartGrpcServer = opts.ValidationServer.StartGrpcServer
	s.controlPlane.StartGrpcServer = opts.ControlPlane.StartGrpcServer

	return err
}

type Extensions struct {
	PluginRegistryFactory plugins.PluginRegistryFactory
	SyncerExtensions      []syncer.TranslatorSyncerExtensionFactory
	XdsCallbacks          xdsserver.Callbacks
}

func RunGloo(opts bootstrap.Opts) error {
	return RunGlooWithExtensions(
		opts,
		Extensions{
			SyncerExtensions: []syncer.TranslatorSyncerExtensionFactory{
				ratelimitExt.NewTranslatorSyncerExtension,
				extauthExt.NewTranslatorSyncerExtension,
			}},
		make(chan struct{}),
	)
}

func RunGlooWithExtensions(opts bootstrap.Opts, extensions Extensions, apiEmitterChan chan struct{}) error {
	watchOpts := opts.WatchOpts.WithDefaults()
	opts.WatchOpts.Ctx = contextutils.WithLogger(opts.WatchOpts.Ctx, "gloo")

	watchOpts.Ctx = contextutils.WithLogger(watchOpts.Ctx, "setup")
	endpointsFactory := &factory.MemoryResourceClientFactory{
		Cache: memory.NewInMemoryResourceCache(),
	}

	upstreamClient, err := v1.NewUpstreamClient(watchOpts.Ctx, opts.Upstreams)
	if err != nil {
		return err
	}
	if err := upstreamClient.Register(); err != nil {
		return err
	}

	kubeServiceClient := opts.KubeServiceClient
	if opts.Settings.GetGloo().GetDisableKubernetesDestinations() {
		kubeServiceClient = nil
	}
	hybridUsClient, err := upstreams.NewHybridUpstreamClient(upstreamClient, kubeServiceClient, opts.Consul.ConsulWatcher)
	if err != nil {
		return err
	}

	proxyClient, err := v1.NewProxyClient(watchOpts.Ctx, opts.Proxies)
	if err != nil {
		return err
	}
	if err := proxyClient.Register(); err != nil {
		return err
	}

	upstreamGroupClient, err := v1.NewUpstreamGroupClient(watchOpts.Ctx, opts.UpstreamGroups)
	if err != nil {
		return err
	}
	if err := upstreamGroupClient.Register(); err != nil {
		return err
	}

	endpointClient, err := v1.NewEndpointClient(watchOpts.Ctx, endpointsFactory)
	if err != nil {
		return err
	}

	secretClient, err := v1.NewSecretClient(watchOpts.Ctx, opts.Secrets)
	if err != nil {
		return err
	}

	artifactClient, err := v1.NewArtifactClient(watchOpts.Ctx, opts.Artifacts)
	if err != nil {
		return err
	}

	authConfigClient, err := extauth.NewAuthConfigClient(watchOpts.Ctx, opts.AuthConfigs)
	if err != nil {
		return err
	}
	if err := authConfigClient.Register(); err != nil {
		return err
	}

	graphqlApiClient, err := v1beta1.NewGraphQLApiClient(watchOpts.Ctx, opts.GraphQLApis)
	if err != nil {
		return err
	}
	if err := graphqlApiClient.Register(); err != nil {
		return err
	}

	rlClient, rlReporterClient, err := rlv1alpha1.NewRateLimitClients(watchOpts.Ctx, opts.RateLimitConfigs)
	if err != nil {
		return err
	}
	if err := rlClient.Register(); err != nil {
		return err
	}

	virtualServiceClient, err := gateway.NewVirtualServiceClient(watchOpts.Ctx, opts.VirtualServices)
	if err != nil {
		return err
	}
	if err := virtualServiceClient.Register(); err != nil {
		return err
	}

	rtClient, err := gateway.NewRouteTableClient(watchOpts.Ctx, opts.RouteTables)
	if err != nil {
		return err
	}
	if err := rtClient.Register(); err != nil {
		return err
	}

	gatewayClient, err := gateway.NewGatewayClient(watchOpts.Ctx, opts.Gateways)
	if err != nil {
		return err
	}
	if err := gatewayClient.Register(); err != nil {
		return err
	}

	virtualHostOptionClient, err := gateway.NewVirtualHostOptionClient(watchOpts.Ctx, opts.VirtualHostOptions)
	if err != nil {
		return err
	}
	if err := virtualHostOptionClient.Register(); err != nil {
		return err
	}

	routeOptionClient, err := gateway.NewRouteOptionClient(watchOpts.Ctx, opts.RouteOptions)
	if err != nil {
		return err
	}
	if err := routeOptionClient.Register(); err != nil {
		return err
	}

	matchableHttpGatewayClient, err := gateway.NewMatchableHttpGatewayClient(watchOpts.Ctx, opts.MatchableHttpGateways)
	if err != nil {
		return err
	}
	if err := matchableHttpGatewayClient.Register(); err != nil {
		return err
	}

	// Register grpc endpoints to the grpc server
	xds.SetupEnvoyXds(opts.ControlPlane.GrpcServer, opts.ControlPlane.XDSServer, opts.ControlPlane.SnapshotCache)
	xdsHasher := xds.NewNodeHasher()

	pluginRegistryFactory := extensions.PluginRegistryFactory
	if pluginRegistryFactory == nil {
		pluginRegistryFactory = registry.GetPluginRegistryFactory(opts)
	}

	pluginRegistry := pluginRegistryFactory(watchOpts.Ctx)
	var discoveryPlugins []discovery.DiscoveryPlugin
	for _, plug := range pluginRegistry.GetPlugins() {
		disc, ok := plug.(discovery.DiscoveryPlugin)
		if ok {
			discoveryPlugins = append(discoveryPlugins, disc)
		}
	}
	logger := contextutils.LoggerFrom(watchOpts.Ctx)

	startRestXdsServer(opts)

	errs := make(chan error)

	statusClient := gloostatusutils.GetStatusClientForNamespace(opts.StatusReporterNamespace)

	disc := discovery.NewEndpointDiscovery(opts.WatchNamespaces, opts.WriteNamespace, endpointClient, statusClient, discoveryPlugins)
	edsSync := discovery.NewEdsSyncer(disc, discovery.Opts{}, watchOpts.RefreshRate)
	discoveryCache := v1.NewEdsEmitter(hybridUsClient)
	edsEventLoop := v1.NewEdsEventLoop(discoveryCache, edsSync)
	edsErrs, err := edsEventLoop.Run(opts.WatchNamespaces, watchOpts)
	if err != nil {
		return err
	}

	warmTimeout := opts.Settings.GetGloo().GetEndpointsWarmingTimeout()

	if warmTimeout == nil {
		warmTimeout = &duration.Duration{
			Seconds: 5 * 60,
		}
	}
	if warmTimeout.GetSeconds() != 0 || warmTimeout.GetNanos() != 0 {
		warmTimeoutDuration := prototime.DurationFromProto(warmTimeout)
		ctx := opts.WatchOpts.Ctx
		err = channelutils.WaitForReady(ctx, warmTimeoutDuration, edsEventLoop.Ready(), disc.Ready())
		if err != nil {
			// make sure that the reason we got here is not context cancellation
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Panicw("failed warming up endpoints - consider adjusting endpointsWarmingTimeout", "warmTimeoutDuration", warmTimeoutDuration)
		}
	}

	// We are ready!

	go errutils.AggregateErrs(watchOpts.Ctx, errs, edsErrs, "eds.gloo")

	apiCache := v1snap.NewApiEmitterWithEmit(
		artifactClient,
		endpointClient,
		proxyClient,
		upstreamGroupClient,
		secretClient,
		hybridUsClient,
		authConfigClient,
		rlClient,
		virtualServiceClient,
		rtClient,
		gatewayClient,
		virtualHostOptionClient,
		routeOptionClient,
		matchableHttpGatewayClient,
		graphqlApiClient,
		apiEmitterChan,
	)

	rpt := reporter.NewReporter("gloo",
		statusClient,
		hybridUsClient.BaseClient(),
		proxyClient.BaseClient(),
		upstreamGroupClient.BaseClient(),
		authConfigClient.BaseClient(),
		rlReporterClient,
	)

	t := translator.NewTranslator(sslutils.NewSslConfigTranslator(), opts.Settings, pluginRegistryFactory)

	routeReplacingSanitizer, err := sanitizer.NewRouteReplacingSanitizer(opts.Settings.GetGloo().GetInvalidConfigPolicy())
	if err != nil {
		return err
	}

	xdsSanitizer := sanitizer.XdsSanitizers{
		sanitizer.NewUpstreamRemovingSanitizer(),
		routeReplacingSanitizer,
	}

	validator := validation.NewValidator(watchOpts.Ctx, t, xdsSanitizer)
	if opts.ValidationServer.Server != nil {
		opts.ValidationServer.Server.SetValidator(validator)
	}

	params := syncer.TranslatorSyncerExtensionParams{
		RateLimitServiceSettings: ratelimit.ServiceSettings{
			Descriptors:    opts.Settings.GetRatelimit().GetDescriptors(),
			SetDescriptors: opts.Settings.GetRatelimit().GetSetDescriptors(),
		},
	}

	// Set up the syncer extension
	syncerExtensions := []syncer.TranslatorSyncerExtension{}

	upgradedExtensions := make(map[string]bool)
	for _, syncerExtensionFactory := range extensions.SyncerExtensions {
		syncerExtension, err := syncerExtensionFactory(watchOpts.Ctx, params)
		if err != nil {
			logger.Errorw("Error initializing extension", "error", err)
			continue
		}
		if extension, ok := syncerExtension.(syncer.UpgradeableTranslatorSyncerExtension); ok && extension.IsUpgrade() {
			upgradedExtensions[extension.ExtensionName()] = true
		}
		syncerExtensions = append(syncerExtensions, syncerExtension)
	}
	syncerExtensions = reconcileUpgradedTranslatorSyncerExtensions(syncerExtensions, upgradedExtensions)

	statusMetrics, err := metrics.NewConfigStatusMetrics(opts.Settings.GetObservabilityOptions().GetConfigStatusMetricLabels())
	if err != nil {
		return err
	}
	translationSync := syncer.NewTranslatorSyncer(t, opts.ControlPlane.SnapshotCache, xdsHasher, xdsSanitizer, rpt, opts.DevMode, syncerExtensions, opts.Settings, statusMetrics)

	syncers := v1snap.ApiSyncers{
		translationSync,
		validator,
	}

	apiEventLoop := v1snap.NewApiEventLoop(apiCache, syncers)
	apiEventLoopErrs, err := apiEventLoop.Run(opts.WatchNamespaces, watchOpts)
	if err != nil {
		return err
	}
	go errutils.AggregateErrs(watchOpts.Ctx, errs, apiEventLoopErrs, "event_loop.gloo")

	go func() {
		for {
			select {
			case <-watchOpts.Ctx.Done():
				logger.Debugf("context cancelled")
				return
			}
		}
	}()

	if opts.ControlPlane.StartGrpcServer {
		// copy for the go-routines
		controlPlane := opts.ControlPlane
		lis, err := net.Listen(opts.ControlPlane.BindAddr.Network(), opts.ControlPlane.BindAddr.String())
		if err != nil {
			return err
		}
		go func() {
			<-controlPlane.GrpcService.Ctx.Done()
			controlPlane.GrpcServer.Stop()
		}()

		go func() {
			if err := controlPlane.GrpcServer.Serve(lis); err != nil {
				logger.Errorf("xds grpc server failed to start")
			}
		}()
		opts.ControlPlane.StartGrpcServer = false
	}

	if opts.ValidationServer.StartGrpcServer {
		validationServer := opts.ValidationServer
		lis, err := net.Listen(validationServer.BindAddr.Network(), validationServer.BindAddr.String())
		if err != nil {
			return err
		}
		validationServer.Server.Register(validationServer.GrpcServer)

		go func() {
			<-validationServer.Ctx.Done()
			validationServer.GrpcServer.Stop()
		}()

		go func() {
			if err := validationServer.GrpcServer.Serve(lis); err != nil {
				logger.Errorf("validation grpc server failed to start")
			}
		}()
		opts.ValidationServer.StartGrpcServer = false
	}

	go func() {
		for {
			select {
			case err, ok := <-errs:
				if !ok {
					return
				}
				logger.Errorw("gloo main event loop", zap.Error(err))
			case <-opts.WatchOpts.Ctx.Done():
				// think about closing this channel
				// close(errs)
				return
			}
		}
	}()

	return nil
}

// removes any redundant syncers, if we have added an upgraded version to replace them
func reconcileUpgradedTranslatorSyncerExtensions(syncerList []syncer.TranslatorSyncerExtension, upgradedSyncers map[string]bool) []syncer.TranslatorSyncerExtension {
	var syncersToDrop []int
	for i, syncerExtension := range syncerList {
		extension, upgradable := syncerExtension.(syncer.UpgradeableTranslatorSyncerExtension)
		if upgradable {
			_, inMap := upgradedSyncers[extension.ExtensionName()]
			if inMap && !extension.IsUpgrade() {
				// An upgraded version of this syncer exists,
				// mark this one for removal
				syncersToDrop = append(syncersToDrop, i)
			}
		}
	}

	// Walk back through the syncerList and remove the redundant syncers
	for i := len(syncersToDrop) - 1; i >= 0; i-- {
		badIndex := syncersToDrop[i]
		syncerList = append(syncerList[:badIndex], syncerList[badIndex+1:]...)
	}
	return syncerList
}

func startRestXdsServer(opts bootstrap.Opts) {
	restClient := server.NewHTTPGateway(
		contextutils.LoggerFrom(opts.WatchOpts.Ctx),
		opts.ControlPlane.XDSServer,
		map[string]string{
			types.FetchEndpointsV3: types.EndpointTypeV3,
		},
	)
	restXdsAddr := opts.Settings.GetGloo().GetRestXdsBindAddr()
	if restXdsAddr == "" {
		restXdsAddr = DefaultRestXdsBindAddr
	}
	srv := &http.Server{
		Addr:    restXdsAddr,
		Handler: restClient,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// TODO: Add metrics for rest xds server
			contextutils.LoggerFrom(opts.WatchOpts.Ctx).Warnf("error while running REST xDS server", zap.Error(err))
		}
	}()
	go func() {
		<-opts.WatchOpts.Ctx.Done()
		if err := srv.Close(); err != nil {
			contextutils.LoggerFrom(opts.WatchOpts.Ctx).Warnf("error while shutting down REST xDS server", zap.Error(err))
		}
	}()
}

func constructOpts(ctx context.Context, clientset *kubernetes.Interface, kubeCache kube.SharedCache, consulClient *consulapi.Client, vaultClient *vaultapi.Client, memCache memory.InMemoryResourceCache, settings *v1.Settings) (bootstrap.Opts, error) {

	var (
		cfg           *rest.Config
		kubeCoreCache corecache.KubeCoreCache
	)

	params := bootstrap.NewConfigFactoryParams(
		settings,
		memCache,
		kubeCache,
		&cfg,
		consulClient,
	)

	upstreamFactory, err := bootstrap.ConfigFactoryForSettings(params, v1.UpstreamCrd)
	if err != nil {
		return bootstrap.Opts{}, errors.Wrapf(err, "creating config source from settings")
	}

	kubeServiceClient, err := bootstrap.KubeServiceClientForSettings(
		ctx,
		settings,
		memCache,
		&cfg,
		clientset,
		&kubeCoreCache,
	)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	proxyFactory, err := bootstrap.ConfigFactoryForSettings(params, v1.ProxyCrd)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	secretFactory, err := bootstrap.SecretFactoryForSettings(
		ctx,
		settings,
		memCache,
		&cfg,
		clientset,
		&kubeCoreCache,
		vaultClient,
		v1.SecretCrd.Plural,
	)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	upstreamGroupFactory, err := bootstrap.ConfigFactoryForSettings(params, v1.UpstreamGroupCrd)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	artifactFactory, err := bootstrap.ArtifactFactoryForSettings(
		ctx,
		settings,
		memCache,
		&cfg,
		clientset,
		&kubeCoreCache,
		consulClient,
		v1.ArtifactCrd.Plural,
	)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	authConfigFactory, err := bootstrap.ConfigFactoryForSettings(params, extauth.AuthConfigCrd)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	rateLimitConfigFactory, err := bootstrap.ConfigFactoryForSettings(params, rlv1alpha1.RateLimitConfigCrd)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	graphqlApiFactory, err := bootstrap.ConfigFactoryForSettings(params, v1beta1.GraphQLApiCrd)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	virtualServiceFactory, err := bootstrap.ConfigFactoryForSettings(params, gateway.VirtualServiceCrd)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	routeTableFactory, err := bootstrap.ConfigFactoryForSettings(params, gateway.RouteTableCrd)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	virtualHostOptionFactory, err := bootstrap.ConfigFactoryForSettings(params, gateway.VirtualHostOptionCrd)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	routeOptionFactory, err := bootstrap.ConfigFactoryForSettings(params, gateway.RouteOptionCrd)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	gatewayFactory, err := bootstrap.ConfigFactoryForSettings(params, gateway.GatewayCrd)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	matchableHttpGatewayFactory, err := bootstrap.ConfigFactoryForSettings(params, gateway.MatchableHttpGatewayCrd)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	return bootstrap.Opts{
		Upstreams:             upstreamFactory,
		KubeServiceClient:     kubeServiceClient,
		Proxies:               proxyFactory,
		UpstreamGroups:        upstreamGroupFactory,
		Secrets:               secretFactory,
		Artifacts:             artifactFactory,
		AuthConfigs:           authConfigFactory,
		RateLimitConfigs:      rateLimitConfigFactory,
		GraphQLApis:           graphqlApiFactory,
		VirtualServices:       virtualServiceFactory,
		RouteTables:           routeTableFactory,
		VirtualHostOptions:    virtualHostOptionFactory,
		RouteOptions:          routeOptionFactory,
		Gateways:              gatewayFactory,
		MatchableHttpGateways: matchableHttpGatewayFactory,
		KubeCoreCache:         kubeCoreCache,
	}, nil
}
