package runner

import (
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/duration"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/pkg/utils/channelutils"
	gloostatusutils "github.com/solo-io/gloo/pkg/utils/statusutils"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gwdefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gwreconciler "github.com/solo-io/gloo/projects/gateway/pkg/reconciler"
	"github.com/solo-io/gloo/projects/gateway/pkg/services/k8sadmission"
	gwsyncer "github.com/solo-io/gloo/projects/gateway/pkg/syncer"
	gwtranslator "github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils/metrics"
	gwvalidation "github.com/solo-io/gloo/projects/gateway/pkg/validation"
	ratelimitv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	consulplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	sslutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/types"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	"go.uber.org/zap"
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

	pluginRegistry := extensions.PluginRegistryFactory(watchOpts.Ctx, GetPluginOpts(opts))
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

	validationOptions, err := generateValidationStartOpts(opts.GatewayControllerEnabled, opts.Settings)
	if err != nil {
		return err
	}

	gwOpts := gwtranslator.Opts{
		WriteNamespace:                 opts.WriteNamespace,
		ReadGatewaysFromAllNamespaces:  opts.Settings.GetGateway().GetReadGatewaysFromAllNamespaces(),
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

func generateValidationStartOpts(gatewayMode bool, settings *gloov1.Settings) (*gwtranslator.ValidationOpts, error) {
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
