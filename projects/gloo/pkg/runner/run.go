package runner

import (
	"context"
	"github.com/golang/protobuf/ptypes/duration"
	vaultapi "github.com/hashicorp/vault/api"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/pkg/utils/channelutils"
	gloostatusutils "github.com/solo-io/gloo/pkg/utils/statusutils"
	gwdefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gwreconciler "github.com/solo-io/gloo/projects/gateway/pkg/reconciler"
	"github.com/solo-io/gloo/projects/gateway/pkg/services/k8sadmission"
	gwsyncer "github.com/solo-io/gloo/projects/gateway/pkg/syncer"
	gwtranslator "github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils/metrics"
	gwvalidation "github.com/solo-io/gloo/projects/gateway/pkg/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	consulplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/enterprise_warning"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	extauthExt "github.com/solo-io/gloo/projects/gloo/pkg/syncer/extauth"
	ratelimitExt "github.com/solo-io/gloo/projects/gloo/pkg/syncer/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	sslutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/types"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	ratelimitv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1"
	"github.com/solo-io/gloo/projects/gloo/pkg/debug"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/validation"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
)

type RunWithOptions func(opts RunOpts) error

type RunOpts struct {
	WriteNamespace  string
	WatchNamespaces []string

	Settings  *gloov1.Settings
	WatchOpts clients.WatchOpts

	ResourceClientset ResourceClientset
	TypedClientset    TypedClientset

	GatewayControllerEnabled bool

	ControlPlane     ControlPlane
	ValidationServer ValidationServer
	ProxyDebugServer ProxyDebugServer
}

// A PluginRegistryFactory generates a PluginRegistry
// It is executed each translation loop, ensuring we have up to date configuration of all plugins
type PluginRegistryFactory func(ctx context.Context, opts RunOpts) plugins.PluginRegistry

type RunExtensions struct {
	PluginRegistryFactory PluginRegistryFactory
	SyncerExtensions      []syncer.TranslatorSyncerExtensionFactory
	XdsCallbacks          server.Callbacks
	ApiEmitterChannel     chan struct{}
}

type ResourceClientset struct {
	// Gateway resources
	VirtualServices       gatewayv1.VirtualServiceClient
	RouteTables           gatewayv1.RouteTableClient
	Gateways              gatewayv1.GatewayClient
	MatchableHttpGateways gatewayv1.MatchableHttpGatewayClient
	VirtualHostOptions    gatewayv1.VirtualHostOptionClient
	RouteOptions          gatewayv1.RouteOptionClient

	// Gloo resources
	Endpoints      gloov1.EndpointClient
	Upstreams      gloov1.UpstreamClient
	UpstreamGroups gloov1.UpstreamGroupClient
	Proxies        gloov1.ProxyClient
	Secrets        gloov1.SecretClient
	Artifacts      gloov1.ArtifactClient

	// Gloo Enterprise resources
	AuthConfigs       extauthv1.AuthConfigClient
	GraphQLApis       v1beta1.GraphQLApiClient
	RateLimitConfigs  ratelimitv1.RateLimitConfigClient
	RateLimitReporter reporter.ReporterResourceClient
}

type TypedClientset struct {
	// Kubernetes clients
	KubeClient        kubernetes.Interface
	KubeServiceClient skkube.ServiceClient
	KubeCoreCache     corecache.KubeCoreCache

	// Consul clients
	ConsulWatcher consul.ConsulWatcher
}

type ControlPlane struct {
	*GrpcService
	SnapshotCache cache.SnapshotCache
	XDSServer     server.Server
}

// ValidationServer validates proxies generated by controllers outside the gloo pod
type ValidationServer struct {
	*GrpcService
	Server validation.ValidationServer
}

// ProxyDebugServer returns proxies to callers outside the gloo pod - this is only necessary for UI/debugging purposes.
type ProxyDebugServer struct {
	*GrpcService
	Server debug.ProxyEndpointServer
}
type GrpcService struct {
	Ctx             context.Context
	BindAddr        net.Addr
	GrpcServer      *grpc.Server
	StartGrpcServer bool
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
	discoveryCache := gloov1.NewEdsEmitter(hybridUsClient)
	edsEventLoop := gloov1.NewEdsEventLoop(discoveryCache, edsSync)
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

func GenerateGlooClientsets(ctx context.Context, settings *gloov1.Settings, kubeCache kube.SharedCache, memCache memory.InMemoryResourceCache) (ResourceClientset, TypedClientset, error) {
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

	consulClient, err := bootstrap.ConsulClientForSettings(ctx, settings)
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
		vaultClient, err = bootstrap.VaultClientForSettings(vaultSettings)
		if err != nil {
			return failedToConstruct(err)
		}
	}

	params := bootstrap.NewConfigFactoryParams(
		settings,
		memCache,
		kubeCache,
		&cfg,
		consulClient,
	)

	kubeServiceClient, err := bootstrap.KubeServiceClientForSettings(
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

	upstreamFactory, err := bootstrap.ConfigFactoryForSettings(params, gloov1.UpstreamCrd)
	if err != nil {
		return failedToConstruct(errors.Wrapf(err, "creating config source from settings"))
	}

	var proxyFactory factory.ResourceClientFactory
	if settings.GetGateway().GetPersistProxySpec().GetValue() {
		proxyFactory, err = bootstrap.ConfigFactoryForSettings(params, gloov1.ProxyCrd)
		if err != nil {
			return failedToConstruct(err)
		}
	} else {
		proxyFactory = &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}
	}

	secretFactory, err := bootstrap.SecretFactoryForSettings(
		ctx,
		settings,
		memCache,
		&cfg,
		&kubeClient,
		&kubeCoreCache,
		vaultClient,
		gloov1.SecretCrd.Plural,
	)
	if err != nil {
		return failedToConstruct(err)
	}

	upstreamGroupFactory, err := bootstrap.ConfigFactoryForSettings(params, gloov1.UpstreamGroupCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	artifactFactory, err := bootstrap.ArtifactFactoryForSettings(
		ctx,
		settings,
		memCache,
		&cfg,
		&kubeClient,
		&kubeCoreCache,
		consulClient,
		gloov1.ArtifactCrd.Plural,
	)
	if err != nil {
		return failedToConstruct(err)
	}

	authConfigFactory, err := bootstrap.ConfigFactoryForSettings(params, extauthv1.AuthConfigCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	rateLimitConfigFactory, err := bootstrap.ConfigFactoryForSettings(params, ratelimitv1.RateLimitConfigCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	graphqlApiFactory, err := bootstrap.ConfigFactoryForSettings(params, v1beta1.GraphQLApiCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	virtualServiceFactory, err := bootstrap.ConfigFactoryForSettings(params, gatewayv1.VirtualServiceCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	routeTableFactory, err := bootstrap.ConfigFactoryForSettings(params, gatewayv1.RouteTableCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	virtualHostOptionFactory, err := bootstrap.ConfigFactoryForSettings(params, gatewayv1.VirtualHostOptionCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	routeOptionFactory, err := bootstrap.ConfigFactoryForSettings(params, gatewayv1.RouteOptionCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	gatewayFactory, err := bootstrap.ConfigFactoryForSettings(params, gatewayv1.GatewayCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	matchableHttpGatewayFactory, err := bootstrap.ConfigFactoryForSettings(params, gatewayv1.MatchableHttpGatewayCrd)
	if err != nil {
		return failedToConstruct(err)
	}

	endpointsFactory := &factory.MemoryResourceClientFactory{
		Cache: memCache,
	}

	upstreamClient, err := gloov1.NewUpstreamClient(ctx, upstreamFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := upstreamClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	proxyClient, err := gloov1.NewProxyClient(ctx, proxyFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := proxyClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	upstreamGroupClient, err := gloov1.NewUpstreamGroupClient(ctx, upstreamGroupFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := upstreamGroupClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	endpointClient, err := gloov1.NewEndpointClient(ctx, endpointsFactory)
	if err != nil {
		return failedToConstruct(err)
	}

	secretClient, err := gloov1.NewSecretClient(ctx, secretFactory)
	if err != nil {
		return failedToConstruct(err)
	}

	artifactClient, err := gloov1.NewArtifactClient(ctx, artifactFactory)
	if err != nil {
		return failedToConstruct(err)
	}

	authConfigClient, err := extauthv1.NewAuthConfigClient(ctx, authConfigFactory)
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

	rateLimitClient, rateLimitReporterClient, err := ratelimitv1.NewRateLimitClients(ctx, rateLimitConfigFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := rateLimitClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	virtualServiceClient, err := gatewayv1.NewVirtualServiceClient(ctx, virtualServiceFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := virtualServiceClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	routeTableClient, err := gatewayv1.NewRouteTableClient(ctx, routeTableFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := routeTableClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	gatewayClient, err := gatewayv1.NewGatewayClient(ctx, gatewayFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := gatewayClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	matchableHttpGatewayClient, err := gatewayv1.NewMatchableHttpGatewayClient(ctx, matchableHttpGatewayFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := matchableHttpGatewayClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	virtualHostOptionClient, err := gatewayv1.NewVirtualHostOptionClient(ctx, virtualHostOptionFactory)
	if err != nil {
		return failedToConstruct(err)
	}
	if err := virtualHostOptionClient.Register(); err != nil {
		return failedToConstruct(err)
	}

	routeOptionClient, err := gatewayv1.NewRouteOptionClient(ctx, routeOptionFactory)
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

func GenerateValidationStartOpts(gatewayMode bool, settings *gloov1.Settings) (*gwtranslator.ValidationOpts, error) {
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
