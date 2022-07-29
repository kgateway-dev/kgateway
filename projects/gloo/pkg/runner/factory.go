package runner

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/enterprise_warning"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"

	"github.com/solo-io/gloo/pkg/bootstrap"

	"github.com/solo-io/gloo/projects/gloo/pkg/debug"

	"github.com/solo-io/gloo/projects/gateway/pkg/services/k8sadmission"

	gwreconciler "github.com/solo-io/gloo/projects/gateway/pkg/reconciler"
	gwsyncer "github.com/solo-io/gloo/projects/gateway/pkg/syncer"
	gwvalidation "github.com/solo-io/gloo/projects/gateway/pkg/validation"

	"github.com/solo-io/gloo/projects/gateway/pkg/utils/metrics"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"

	gloostatusutils "github.com/solo-io/gloo/pkg/utils/statusutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"

	"github.com/golang/protobuf/ptypes/duration"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/pkg/utils/channelutils"
	gateway "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gwdefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gwtranslator "github.com/solo-io/gloo/projects/gateway/pkg/translator"
	rlv1alpha1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	gloobootstrap "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	consulplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	extauthExt "github.com/solo-io/gloo/projects/gloo/pkg/syncer/extauth"
	ratelimitExt "github.com/solo-io/gloo/projects/gloo/pkg/syncer/ratelimit"
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
)

// TODO: (copied from gateway) switch AcceptAllResourcesByDefault to false after validation has been tested in user environments
var AcceptAllResourcesByDefault = true

var AllowWarnings = true

type glooRunnerFactory struct {
	runnerFactory bootstrap.RunnerFactory

	resourceClientset ResourceClientset
	typedClientset    TypedClientset

	extensions               *RunExtensions
	runFunc                  RunWithOptions
	makeGrpcServer           func(ctx context.Context, options ...grpc.ServerOption) *grpc.Server
	previousXdsServer        grpcServer
	previousValidationServer grpcServer
	previousProxyDebugServer grpcServer
	controlPlane             ControlPlane
	validationServer         ValidationServer
	proxyDebugServer         ProxyDebugServer
	callbacks                xdsserver.Callbacks
}

func NewRunnerFactory() *glooRunnerFactory {
	return NewRunnerFactoryWithRunAndExtensions(RunGloo, nil)
}

// used outside of this repo
//noinspection GoUnusedExportedFunction
func NewRunnerFactoryWithExtensions(extensions RunExtensions) *glooRunnerFactory {
	runWithExtensions := func(opts RunOpts) error {
		return RunGlooWithExtensions(opts, extensions)
	}
	return NewRunnerFactoryWithRunAndExtensions(runWithExtensions, &extensions)
}

// for use by UDS, FDS, other v1.SetupSyncers
func NewRunnerFactoryWithRun(runFunc RunWithOptions) bootstrap.RunnerFactory {
	return NewRunnerFactoryWithRunAndExtensions(runFunc, nil).GetRunnerFactory()
}

func NewRunnerFactoryWithRunAndExtensions(runFunc RunWithOptions, extensions *RunExtensions) *glooRunnerFactory {
	s := &glooRunnerFactory{
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
	s.runnerFactory = s.RunnerFactoryImpl
	return s
}

func (g *glooRunnerFactory) GetRunnerFactory() bootstrap.RunnerFactory {
	return g.runnerFactory
}

func (g *glooRunnerFactory) GetResourceClientset() ResourceClientset {
	return g.resourceClientset
}

func (g *glooRunnerFactory) GetTypedClientset() TypedClientset {
	return g.typedClientset
}

// grpcServer contains grpc server configuration fields we will need to persist after starting a server
// to later check if they changed and we need to trigger a server restart
type grpcServer struct {
	addr            string
	maxGrpcRecvSize int
	cancel          context.CancelFunc
}

func NewControlPlane(ctx context.Context, grpcServer *grpc.Server, bindAddr net.Addr, callbacks xdsserver.Callbacks, start bool) ControlPlane {
	snapshotCache := xds.NewAdsSnapshotCache(ctx)
	xdsServer := server.NewServer(ctx, snapshotCache, callbacks)
	reflection.Register(grpcServer)

	return ControlPlane{
		GrpcService: &GrpcService{
			GrpcServer:      grpcServer,
			StartGrpcServer: start,
			BindAddr:        bindAddr,
			Ctx:             ctx,
		},
		SnapshotCache: snapshotCache,
		XDSServer:     xdsServer,
	}
}

func NewValidationServer(ctx context.Context, grpcServer *grpc.Server, bindAddr net.Addr, start bool) ValidationServer {
	return ValidationServer{
		GrpcService: &GrpcService{
			GrpcServer:      grpcServer,
			StartGrpcServer: start,
			BindAddr:        bindAddr,
			Ctx:             ctx,
		},
		Server: validation.NewValidationServer(),
	}
}

func NewProxyDebugServer(ctx context.Context, grpcServer *grpc.Server, bindAddr net.Addr, start bool) ProxyDebugServer {
	return ProxyDebugServer{
		GrpcService: &GrpcService{
			Ctx:             ctx,
			BindAddr:        bindAddr,
			GrpcServer:      grpcServer,
			StartGrpcServer: start,
		},
		Server: debug.NewProxyEndpointServer(),
	}
}

var (
	DefaultXdsBindAddr        = fmt.Sprintf("0.0.0.0:%v", defaults.GlooXdsPort)
	DefaultValidationBindAddr = fmt.Sprintf("0.0.0.0:%v", defaults.GlooValidationPort)
	DefaultRestXdsBindAddr    = fmt.Sprintf("0.0.0.0:%v", defaults.GlooRestXdsPort)
	DefaultProxyDebugAddr     = fmt.Sprintf("0.0.0.0:%v", defaults.GlooProxyDebugPort)
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

func (s *glooRunnerFactory) RunnerFactoryImpl(ctx context.Context, kubeCache kube.SharedCache, memCache memory.InMemoryResourceCache, settings *v1.Settings) (bootstrap.RunFunc, error) {

	xdsAddr := settings.GetGloo().GetXdsBindAddr()
	if xdsAddr == "" {
		xdsAddr = DefaultXdsBindAddr
	}
	xdsTcpAddress, err := getAddr(xdsAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing xds addr")
	}

	validationAddr := settings.GetGloo().GetValidationBindAddr()
	if validationAddr == "" {
		validationAddr = DefaultValidationBindAddr
	}
	validationTcpAddress, err := getAddr(validationAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing validation addr")
	}

	proxyDebugAddr := settings.GetGloo().GetProxyDebugBindAddr()
	if proxyDebugAddr == "" {
		proxyDebugAddr = DefaultProxyDebugAddr
	}
	proxyDebugTcpAddress, err := getAddr(proxyDebugAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing proxy debug endpoint addr")
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

	maxGrpcRecvSize := -1
	// Use the same maxGrpcMsgSize for both validation server and proxy debug server as the message size is determined by the size of proxies.
	if maxGrpcMsgSize := settings.GetGateway().GetValidation().GetValidationServerGrpcMaxSizeBytes(); maxGrpcMsgSize != nil {
		if maxGrpcMsgSize.GetValue() < 0 {
			return nil, errors.Errorf("validationServerGrpcMaxSizeBytes in settings CRD must be non-negative, current value: %v", maxGrpcMsgSize.GetValue())
		}
		maxGrpcRecvSize = int(maxGrpcMsgSize.GetValue())
	}

	emptyControlPlane := ControlPlane{}
	emptyValidationServer := ValidationServer{}
	emptyProxyDebugServer := ProxyDebugServer{}

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

	// check if we need to restart the proxy debug server
	if proxyDebugAddr != s.previousProxyDebugServer.addr || maxGrpcRecvSize != s.previousProxyDebugServer.maxGrpcRecvSize {
		if s.previousProxyDebugServer.cancel != nil {
			s.previousProxyDebugServer.cancel()
			s.previousProxyDebugServer.cancel = nil
		}
		s.proxyDebugServer = emptyProxyDebugServer
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
		// if validationServerGrpcMaxSizeBytes was set this will be non-negative, otherwise use gRPC default
		if maxGrpcRecvSize >= 0 {
			validationGrpcServerOpts = append(validationGrpcServerOpts, grpc.MaxRecvMsgSize(maxGrpcRecvSize))
		}
		s.validationServer = NewValidationServer(ctx, s.makeGrpcServer(ctx, validationGrpcServerOpts...), validationTcpAddress, true)
		s.previousValidationServer.cancel = cancel
		s.previousValidationServer.addr = validationAddr
		s.previousValidationServer.maxGrpcRecvSize = maxGrpcRecvSize
	}
	// initialize the proxy debug server context in this block either on the first loop, or if bind addr changed
	if s.proxyDebugServer == emptyProxyDebugServer {
		// create new context as the grpc server might survive multiple iterations of this loop.
		ctx, cancel := context.WithCancel(context.Background())

		proxyGrpcServerOpts := []grpc.ServerOption{grpc.MaxRecvMsgSize(maxGrpcRecvSize)}
		s.proxyDebugServer = NewProxyDebugServer(ctx, s.makeGrpcServer(ctx, proxyGrpcServerOpts...), proxyDebugTcpAddress, true)
		s.previousProxyDebugServer.cancel = cancel
		s.previousProxyDebugServer.addr = proxyDebugAddr
		s.previousProxyDebugServer.maxGrpcRecvSize = maxGrpcRecvSize
	}

	// Generate the set of clients used to power Gloo Edge
	resourceClientset, typedClientset, err := GenerateGlooClientsets(ctx, settings, kubeCache, memCache)
	if err != nil {
		return nil, err
	}
	s.resourceClientset = resourceClientset
	s.typedClientset = typedClientset

	var gatewayControllerEnabled = true
	if settings.GetGateway().GetEnableGatewayController() != nil {
		gatewayControllerEnabled = settings.GetGateway().GetEnableGatewayController().GetValue()
	}

	opts := RunOpts{
		WriteNamespace:  writeNamespace,
		WatchNamespaces: watchNamespaces,
		WatchOpts: clients.WatchOpts{
			Ctx:         ctx,
			RefreshRate: refreshRate,
		},
		Settings: settings,

		ResourceClientset: resourceClientset,
		TypedClientset:    typedClientset,

		GatewayControllerEnabled: gatewayControllerEnabled,
	}

	// TODO (samheilbron) These should be built from the start options, not included in the start options
	opts.ControlPlane = s.controlPlane
	opts.ValidationServer = s.validationServer
	opts.ProxyDebugServer = s.proxyDebugServer

	return func() error {
		err = s.runFunc(opts)

		s.validationServer.StartGrpcServer = opts.ValidationServer.StartGrpcServer
		s.controlPlane.StartGrpcServer = opts.ControlPlane.StartGrpcServer

		return err
	}, nil
}

func GetPluginOpts(opts RunOpts) registry.PluginOpts {
	settings := opts.Settings
	dnsAddress := settings.GetConsul().GetDnsAddress()
	if len(dnsAddress) == 0 {
		dnsAddress = consulplugin.DefaultDnsAddress
	}

	dnsPollingInterval := consulplugin.DefaultDnsPollingInterval
	if pollingInterval := settings.GetConsul().GetDnsPollingInterval(); pollingInterval != nil {
		dnsPollingInterval = prototime.DurationFromProto(pollingInterval)
	}

	return registry.PluginOpts{
		SecretClient:  opts.ResourceClientset.Secrets,
		KubeClient:    opts.TypedClientset.KubeClient,
		KubeCoreCache: opts.TypedClientset.KubeCoreCache,
		Consul: registry.ConsulPluginOpts{
			ConsulWatcher:      opts.TypedClientset.ConsulWatcher,
			DnsServer:          dnsAddress,
			DnsPollingInterval: &dnsPollingInterval,
		},
	}
}

func GlooPluginRegistryFactory(_ context.Context, opts RunOpts) plugins.PluginRegistry {
	availablePlugins := registry.Plugins(GetPluginOpts(opts))

	// To improve the UX, load a plugin that warns users if they are attempting to use enterprise configuration
	availablePlugins = append(availablePlugins, enterprise_warning.NewPlugin())
	return registry.NewPluginRegistry(availablePlugins)
}

func RunGloo(opts RunOpts) error {
	glooExtensions := RunExtensions{
		PluginRegistryFactory: GlooPluginRegistryFactory,
		SyncerExtensions: []syncer.TranslatorSyncerExtensionFactory{
			ratelimitExt.NewTranslatorSyncerExtension,
			extauthExt.NewTranslatorSyncerExtension,
		},
		ApiEmitterChannel: make(chan struct{}),
		XdsCallbacks:      nil,
	}

	return RunGlooWithExtensions(opts, glooExtensions)
}

func RunGlooWithExtensions(opts RunOpts, extensions RunExtensions) error {
	// Validate RunExtensions
	if extensions.ApiEmitterChannel == nil {
		return errors.Errorf("RunExtensions.ApiEmitterChannel must be defined, found nil")
	}
	if extensions.PluginRegistryFactory == nil {
		return errors.Errorf("RunExtensions.PluginRegistryFactory must be defined, found nil")
	}
	if extensions.SyncerExtensions == nil {
		return errors.Errorf("RunExtensions.SyncerExtensions must be defined, found nil")
	}

	watchOpts := opts.WatchOpts.WithDefaults()
	opts.WatchOpts.Ctx = contextutils.WithLogger(opts.WatchOpts.Ctx, "gloo")

	watchOpts.Ctx = contextutils.WithLogger(watchOpts.Ctx, "setup")

	glooClientset := opts.ResourceClientset

	hybridUsClient, err := upstreams.NewHybridUpstreamClient(glooClientset.Upstreams, opts.TypedClientset.KubeServiceClient, opts.TypedClientset.ConsulWatcher)
	if err != nil {
		return err
	}

	// Delete proxies that may have been left from prior to an upgrade or from previously having set persistProxySpec
	// Ignore errors because gloo will still work with stray proxies.
	_ = setup.DoProxyCleanup(watchOpts.Ctx, opts.Settings, glooClientset.Proxies, opts.WriteNamespace)

	// Register grpc endpoints to the grpc server
	xds.SetupEnvoyXds(opts.ControlPlane.GrpcServer, opts.ControlPlane.XDSServer, opts.ControlPlane.SnapshotCache)

	pluginRegistry := extensions.PluginRegistryFactory(watchOpts.Ctx, opts)
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

	statusReporterNamespace := gloostatusutils.GetStatusReporterNamespaceOrDefault(opts.WriteNamespace)
	statusClient := gloostatusutils.GetStatusClientForNamespace(statusReporterNamespace)
	disc := discovery.NewEndpointDiscovery(opts.WatchNamespaces, opts.WriteNamespace, glooClientset.Endpoints, statusClient, discoveryPlugins)
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
		glooClientset.Artifacts,
		glooClientset.Endpoints,
		glooClientset.Proxies,
		glooClientset.UpstreamGroups,
		glooClientset.Secrets,
		hybridUsClient,
		glooClientset.AuthConfigs,
		glooClientset.RateLimitConfigs,
		glooClientset.VirtualServices,
		glooClientset.RouteTables,
		glooClientset.Gateways,
		glooClientset.VirtualHostOptions,
		glooClientset.RouteOptions,
		glooClientset.MatchableHttpGateways,
		glooClientset.GraphQLApis,
		extensions.ApiEmitterChannel,
	)

	rpt := reporter.NewReporter("gloo",
		statusClient,
		hybridUsClient.BaseClient(),
		glooClientset.Proxies.BaseClient(),
		glooClientset.UpstreamGroups.BaseClient(),
		glooClientset.AuthConfigs.BaseClient(),
		glooClientset.Gateways.BaseClient(),
		glooClientset.MatchableHttpGateways.BaseClient(),
		glooClientset.VirtualServices.BaseClient(),
		glooClientset.RouteTables.BaseClient(),
		glooClientset.VirtualHostOptions.BaseClient(),
		glooClientset.RouteOptions.BaseClient(),
		glooClientset.RateLimitReporter,
	)
	statusMetrics, err := metrics.NewConfigStatusMetrics(opts.Settings.GetObservabilityOptions().GetConfigStatusMetricLabels())
	if err != nil {
		return err
	}
	//The validation grpc server is available for custom controllers
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
	if opts.ProxyDebugServer.StartGrpcServer {
		proxyDebugServer := opts.ProxyDebugServer
		proxyDebugServer.Server.SetProxyClient(glooClientset.Proxies)
		proxyDebugServer.Server.Register(proxyDebugServer.GrpcServer)
		lis, err := net.Listen(opts.ProxyDebugServer.BindAddr.Network(), opts.ProxyDebugServer.BindAddr.String())
		if err != nil {
			return err
		}
		go func() {
			<-proxyDebugServer.GrpcService.Ctx.Done()
			proxyDebugServer.GrpcServer.Stop()
		}()

		go func() {
			if err := proxyDebugServer.GrpcServer.Serve(lis); err != nil {
				logger.Errorf("Proxy debug grpc server failed to start")
			}
		}()
		opts.ProxyDebugServer.StartGrpcServer = false
	}

	validationOptions, err := GenerateValidationStartOpts(opts.GatewayControllerEnabled, opts.Settings)
	if err != nil {
		return err
	}

	gwOpts := gwtranslator.Opts{
		WriteNamespace:                 opts.WriteNamespace,
		ReadGatewaysFromAllNamespaces:  opts.Settings.Gateway.GetReadGatewaysFromAllNamespaces(),
		Validation:                     validationOptions,
		IsolateVirtualHostsBySslConfig: opts.Settings.GetGateway().GetIsolateVirtualHostsBySslConfig().GetValue(),
	}
	var (
		ignoreProxyValidationFailure bool
		allowWarnings                bool
	)
	if validationOptions != nil && opts.GatewayControllerEnabled {
		ignoreProxyValidationFailure = gwOpts.Validation.IgnoreProxyValidationFailure
		allowWarnings = gwOpts.Validation.AllowWarnings
	}

	resourceHasher := translator.MustEnvoyCacheResourcesListToFnvHash

	t := translator.NewTranslatorWithHasher(sslutils.NewSslConfigTranslator(), opts.Settings, pluginRegistry, resourceHasher)

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

	var (
		gwTranslatorSyncer *gwsyncer.TranslatorSyncer
		gatewayTranslator  *gwtranslator.GwTranslator
	)
	if opts.GatewayControllerEnabled {
		logger.Debugf("Setting up gateway translator")
		gatewayTranslator = gwtranslator.NewDefaultTranslator(gwOpts)
		proxyReconciler := gwreconciler.NewProxyReconciler(validator.Validate, glooClientset.Proxies, statusClient)
		gwTranslatorSyncer = gwsyncer.NewTranslatorSyncer(opts.WatchOpts.Ctx, opts.WriteNamespace, glooClientset.Proxies, proxyReconciler, rpt, gatewayTranslator, statusClient, statusMetrics)
	} else {
		logger.Debugf("Gateway translation is disabled. Proxies are provided from another source")
	}
	gwValidationSyncer := gwvalidation.NewValidator(gwvalidation.NewValidatorConfig(
		gatewayTranslator,
		validator.Validate,
		ignoreProxyValidationFailure,
		allowWarnings,
	))

	// Set up the syncer extensions
	syncerExtensionParams := syncer.TranslatorSyncerExtensionParams{
		RateLimitServiceSettings: ratelimit.ServiceSettings{
			Descriptors:    opts.Settings.GetRatelimit().GetDescriptors(),
			SetDescriptors: opts.Settings.GetRatelimit().GetSetDescriptors(),
		},
		Hasher: resourceHasher,
	}
	var syncerExtensions []syncer.TranslatorSyncerExtension
	for _, syncerExtensionFactory := range extensions.SyncerExtensions {
		syncerExtension := syncerExtensionFactory(watchOpts.Ctx, syncerExtensionParams)
		syncerExtensions = append(syncerExtensions, syncerExtension)
	}

	translationSync := syncer.NewTranslatorSyncer(
		t,
		opts.ControlPlane.SnapshotCache,
		xdsSanitizer,
		rpt,
		opts.Settings.GetDevMode(),
		syncerExtensions,
		opts.Settings,
		statusMetrics,
		gwTranslatorSyncer,
		glooClientset.Proxies,
		opts.WriteNamespace)

	syncers := v1snap.ApiSyncers{
		validator,
		translationSync,
	}
	if opts.GatewayControllerEnabled {
		syncers = append(syncers, gwValidationSyncer)
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

	//Start the validation webhook
	glooNamespace := opts.WriteNamespace
	validationServerErr := make(chan error, 1)
	if gwOpts.Validation != nil {
		// make sure non-empty WatchNamespaces contains the gloo instance's own namespace if
		// ReadGatewaysFromAllNamespaces is false
		if !gwOpts.ReadGatewaysFromAllNamespaces && !utils.AllNamespaces(opts.WatchNamespaces) {
			foundSelf := false
			for _, namespace := range opts.WatchNamespaces {
				if glooNamespace == namespace {
					foundSelf = true
					break
				}
			}
			if !foundSelf {
				return errors.Errorf("The gateway configuration value readGatewaysFromAllNamespaces was set "+
					"to false, but the non-empty settings.watchNamespaces "+
					"list (%s) did not contain this gloo instance's own namespace: %s.",
					strings.Join(opts.WatchNamespaces, ", "), glooNamespace)
			}
		}

		validationWebhook, err := k8sadmission.NewGatewayValidatingWebhook(
			k8sadmission.NewWebhookConfig(
				watchOpts.Ctx,
				gwValidationSyncer,
				opts.WatchNamespaces,
				gwOpts.Validation.ValidatingWebhookPort,
				gwOpts.Validation.ValidatingWebhookCertPath,
				gwOpts.Validation.ValidatingWebhookKeyPath,
				gwOpts.Validation.AlwaysAcceptResources,
				gwOpts.ReadGatewaysFromAllNamespaces,
				glooNamespace,
			),
		)
		if err != nil {
			return errors.Wrapf(err, "creating validating webhook")
		}

		go func() {
			// close out validation server when context is cancelled
			<-watchOpts.Ctx.Done()
			validationWebhook.Close()
		}()
		go func() {
			contextutils.LoggerFrom(watchOpts.Ctx).Infow("starting gateway validation server",
				zap.Int("port", gwOpts.Validation.ValidatingWebhookPort),
				zap.String("cert", gwOpts.Validation.ValidatingWebhookCertPath),
				zap.String("key", gwOpts.Validation.ValidatingWebhookKeyPath),
			)
			if err := validationWebhook.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				select {
				case validationServerErr <- err:
				default:
					logger.DPanicw("failed to start validation webhook server", zap.Error(err))
				}
			}
		}()
	}

	// give the validation server 100ms to start
	select {
	case err := <-validationServerErr:
		return errors.Wrapf(err, "failed to start validation webhook server")
	case <-time.After(time.Millisecond * 100):
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

func startRestXdsServer(opts RunOpts) {
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

func GenerateGlooClientsets(ctx context.Context, settings *v1.Settings, kubeCache kube.SharedCache, memCache memory.InMemoryResourceCache) (ResourceClientset, TypedClientset, error) {
	var (
		cfg           *rest.Config
		kubeCoreCache corecache.KubeCoreCache
		kubeClient    kubernetes.Interface

		// Wrapper types for Gloo Edge
		typedClientset    TypedClientset
		resourceClientset ResourceClientset
	)

	failedToConstruct := func(err error) (ResourceClientset, TypedClientset, error) {
		return resourceClientset, typedClientset, err
	}

	consulClient, err := gloobootstrap.ConsulClientForSettings(ctx, settings)
	if err != nil {
		return failedToConstruct(err)
	}

	// if vault service discovery specified, initialize consul watcher
	var consulWatcher consul.ConsulWatcher
	if consulServiceDiscovery := settings.GetConsul().GetServiceDiscovery(); consulServiceDiscovery != nil {
		// Set up ConsulStartOpts client
		consulClientWrapper, err := consul.NewConsulWatcher(consulClient, consulServiceDiscovery.GetDataCenters())
		if err != nil {
			return failedToConstruct(err)
		}
		consulWatcher = consulClientWrapper
	}

	var vaultClient *vaultapi.Client
	if vaultSettings := settings.GetVaultSecretSource(); vaultSettings != nil {
		vaultClient, err = gloobootstrap.VaultClientForSettings(vaultSettings)
		if err != nil {
			return failedToConstruct(err)
		}
	}

	params := gloobootstrap.NewConfigFactoryParams(
		settings,
		memCache,
		kubeCache,
		&cfg,
		consulClient,
	)

	kubeServiceClient, err := gloobootstrap.KubeServiceClientForSettings(
		ctx,
		settings,
		memCache,
		&cfg,
		&kubeClient,
		&kubeCoreCache,
	)
	if err != nil {
		return failedToConstruct(err)
	}

	upstreamFactory, err := gloobootstrap.ConfigFactoryForSettings(params, v1.UpstreamCrd)
	if err != nil {
		return failedToConstruct(errors.Wrapf(err, "creating config source from settings"))
	}

	var proxyFactory factory.ResourceClientFactory
	if settings.GetGateway().GetPersistProxySpec().GetValue() {
		proxyFactory, err = gloobootstrap.ConfigFactoryForSettings(params, v1.ProxyCrd)
		if err != nil {
			return failedToConstruct(err)
		}
	} else {
		proxyFactory = &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}
	}

	secretFactory, err := gloobootstrap.SecretFactoryForSettings(
		ctx,
		settings,
		memCache,
		&cfg,
		&kubeClient,
		&kubeCoreCache,
		vaultClient,
		v1.SecretCrd.Plural,
	)
	if err != nil {
		return failedToConstruct(err)
	}

	upstreamGroupFactory, err := gloobootstrap.ConfigFactoryForSettings(params, v1.UpstreamGroupCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	artifactFactory, err := gloobootstrap.ArtifactFactoryForSettings(
		ctx,
		settings,
		memCache,
		&cfg,
		&kubeClient,
		&kubeCoreCache,
		consulClient,
		v1.ArtifactCrd.Plural,
	)
	if err != nil {
		return failedToConstruct(err)
	}

	authConfigFactory, err := gloobootstrap.ConfigFactoryForSettings(params, extauth.AuthConfigCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	rateLimitConfigFactory, err := gloobootstrap.ConfigFactoryForSettings(params, rlv1alpha1.RateLimitConfigCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	graphqlApiFactory, err := gloobootstrap.ConfigFactoryForSettings(params, v1beta1.GraphQLApiCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	virtualServiceFactory, err := gloobootstrap.ConfigFactoryForSettings(params, gateway.VirtualServiceCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	routeTableFactory, err := gloobootstrap.ConfigFactoryForSettings(params, gateway.RouteTableCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	virtualHostOptionFactory, err := gloobootstrap.ConfigFactoryForSettings(params, gateway.VirtualHostOptionCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	routeOptionFactory, err := gloobootstrap.ConfigFactoryForSettings(params, gateway.RouteOptionCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	gatewayFactory, err := gloobootstrap.ConfigFactoryForSettings(params, gateway.GatewayCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	matchableHttpGatewayFactory, err := gloobootstrap.ConfigFactoryForSettings(params, gateway.MatchableHttpGatewayCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	endpointsFactory := &factory.MemoryResourceClientFactory{
		Cache: memCache,
	}

	upstreamClient, err := v1.NewUpstreamClient(ctx, upstreamFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := upstreamClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	proxyClient, err := v1.NewProxyClient(ctx, proxyFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := proxyClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	upstreamGroupClient, err := v1.NewUpstreamGroupClient(ctx, upstreamGroupFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := upstreamGroupClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	endpointClient, err := v1.NewEndpointClient(ctx, endpointsFactory)
	if err != nil {
		return failedToConstruct(err)
	}

	secretClient, err := v1.NewSecretClient(ctx, secretFactory)
	if err != nil {
		return failedToConstruct(err)
	}

	artifactClient, err := v1.NewArtifactClient(ctx, artifactFactory)
	if err != nil {
		return failedToConstruct(err)
	}

	authConfigClient, err := extauth.NewAuthConfigClient(ctx, authConfigFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := authConfigClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	graphqlApiClient, err := v1beta1.NewGraphQLApiClient(ctx, graphqlApiFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := graphqlApiClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	rateLimitClient, rateLimitReporterClient, err := rlv1alpha1.NewRateLimitClients(ctx, rateLimitConfigFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := rateLimitClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	virtualServiceClient, err := gateway.NewVirtualServiceClient(ctx, virtualServiceFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := virtualServiceClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	routeTableClient, err := gateway.NewRouteTableClient(ctx, routeTableFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := routeTableClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	gatewayClient, err := gateway.NewGatewayClient(ctx, gatewayFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := gatewayClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	matchableHttpGatewayClient, err := gateway.NewMatchableHttpGatewayClient(ctx, matchableHttpGatewayFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := matchableHttpGatewayClient.Register(); err != nil {
		return failedToConstruct(err)
	}
	virtualHostOptionClient, err := gateway.NewVirtualHostOptionClient(ctx, virtualHostOptionFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := virtualHostOptionClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	routeOptionClient, err := gateway.NewRouteOptionClient(ctx, routeOptionFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := routeOptionClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	resourceClientset = ResourceClientset{
		// Gateway resources
		VirtualServices:       virtualServiceClient,
		RouteTables:           routeTableClient,
		Gateways:              gatewayClient,
		MatchableHttpGateways: matchableHttpGatewayClient,
		VirtualHostOptions:    virtualHostOptionClient,
		RouteOptions:          routeOptionClient,

		// Gloo resources
		Endpoints:      endpointClient,
		Upstreams:      upstreamClient,
		UpstreamGroups: upstreamGroupClient,
		Proxies:        proxyClient,
		Secrets:        secretClient,
		Artifacts:      artifactClient,

		// Gloo Enterprise resources
		AuthConfigs:       authConfigClient,
		RateLimitConfigs:  rateLimitClient,
		RateLimitReporter: rateLimitReporterClient,
		GraphQLApis:       graphqlApiClient,
	}

	typedClientset = TypedClientset{
		KubeClient:        kubeClient,
		KubeServiceClient: kubeServiceClient,
		KubeCoreCache:     kubeCoreCache,
		ConsulWatcher:     consulWatcher,
	}

	return resourceClientset, typedClientset, nil
}

func GenerateValidationStartOpts(gatewayMode bool, settings *v1.Settings) (*gwtranslator.ValidationOpts, error) {
	var validation *gwtranslator.ValidationOpts

	validationCfg := settings.GetGateway().GetValidation()

	if validationCfg != nil && gatewayMode {
		alwaysAcceptResources := AcceptAllResourcesByDefault

		if alwaysAccept := validationCfg.GetAlwaysAccept(); alwaysAccept != nil {
			alwaysAcceptResources = alwaysAccept.GetValue()
		}

		allowWarnings := AllowWarnings

		if allowWarning := validationCfg.GetAllowWarnings(); allowWarning != nil {
			allowWarnings = allowWarning.GetValue()
		}

		validation = &gwtranslator.ValidationOpts{
			ProxyValidationServerAddress: validationCfg.GetProxyValidationServerAddr(),
			ValidatingWebhookPort:        gwdefaults.ValidationWebhookBindPort,
			ValidatingWebhookCertPath:    validationCfg.GetValidationWebhookTlsCert(),
			ValidatingWebhookKeyPath:     validationCfg.GetValidationWebhookTlsKey(),
			IgnoreProxyValidationFailure: validationCfg.GetIgnoreGlooValidationFailure(),
			AlwaysAcceptResources:        alwaysAcceptResources,
			AllowWarnings:                allowWarnings,
			WarnOnRouteShortCircuiting:   validationCfg.GetWarnRouteShortCircuiting().GetValue(),
		}
		if validation.ProxyValidationServerAddress == "" {
			validation.ProxyValidationServerAddress = gwdefaults.GlooProxyValidationServerAddr
		}
		if overrideAddr := os.Getenv("PROXY_VALIDATION_ADDR"); overrideAddr != "" {
			validation.ProxyValidationServerAddress = overrideAddr
		}
		if validation.ValidatingWebhookCertPath == "" {
			validation.ValidatingWebhookCertPath = gwdefaults.ValidationWebhookTlsCertPath
		}
		if validation.ValidatingWebhookKeyPath == "" {
			validation.ValidatingWebhookKeyPath = gwdefaults.ValidationWebhookTlsKeyPath
		}
	} else {
		// This will stop users from setting failurePolicy=fail and then removing the webhook configuration
		if validationMustStart := os.Getenv("VALIDATION_MUST_START"); validationMustStart != "" && validationMustStart != "false" && gatewayMode {
			return validation, errors.Errorf("A validation webhook was configured, but no validation configuration was provided in the settings. "+
				"Ensure the v1.Settings %v contains the spec.gateway.validation config."+
				"If you have removed the webhook configuration from K8s since installing and want to disable validation, "+
				"set the environment variable VALIDATION_MUST_START=false",
				settings.GetMetadata().Ref())
		}
	}
	return validation, nil
}
