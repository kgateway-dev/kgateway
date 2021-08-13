package syncer

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/solo-io/gloo/projects/gateway/pkg/reconciler"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"

	"go.uber.org/zap"

	"github.com/solo-io/gloo/projects/gateway/pkg/services/k8sadmisssion"

	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gatewayvalidation "github.com/solo-io/gloo/projects/gateway/pkg/validation"

	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	gloodefaults "github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/pkg/errors"
	"k8s.io/client-go/rest"
)

// TODO: switch AcceptAllResourcesByDefault to false after validation has been tested in user environments
var AcceptAllResourcesByDefault = true

var AllowWarnings = true

func Setup(ctx context.Context, kubeCache kube.SharedCache, inMemoryCache memory.InMemoryResourceCache, settings *gloov1.Settings) error {
	var (
		cfg *rest.Config
	)

	consulClient, err := bootstrap.ConsulClientForSettings(ctx, settings)
	if err != nil {
		return err
	}

	params := bootstrap.NewConfigFactoryParams(
		settings,
		inMemoryCache,
		kubeCache,
		&cfg,
		consulClient,
	)

	proxyFactory, err := bootstrap.ConfigFactoryForSettings(params, gloov1.ProxyCrd)
	if err != nil {
		return err
	}

	virtualServiceFactory, err := bootstrap.ConfigFactoryForSettings(params, v1.VirtualServiceCrd)
	if err != nil {
		return err
	}

	routeTableFactory, err := bootstrap.ConfigFactoryForSettings(params, v1.RouteTableCrd)
	if err != nil {
		return err
	}

	virtualHostOptionFactory, err := bootstrap.ConfigFactoryForSettings(params, v1.VirtualHostOptionCrd)
	if err != nil {
		return err
	}

	routeOptionFactory, err := bootstrap.ConfigFactoryForSettings(params, v1.RouteOptionCrd)
	if err != nil {
		return err
	}

	gatewayFactory, err := bootstrap.ConfigFactoryForSettings(params, v1.GatewayCrd)
	if err != nil {
		return err
	}

	refreshRate := prototime.DurationFromProto(settings.GetRefreshRate())

	writeNamespace := settings.DiscoveryNamespace
	if writeNamespace == "" {
		writeNamespace = gloodefaults.GlooSystem
	}
	watchNamespaces := utils.ProcessWatchNamespaces(settings.GetWatchNamespaces(), writeNamespace)

	var validation *translator.ValidationOpts
	validationCfg := settings.GetGateway().GetValidation()
	if validationCfg != nil {
		alwaysAcceptResources := AcceptAllResourcesByDefault

		if alwaysAccept := validationCfg.AlwaysAccept; alwaysAccept != nil {
			alwaysAcceptResources = alwaysAccept.GetValue()
		}

		allowWarnings := AllowWarnings

		if allowWarning := validationCfg.AllowWarnings; allowWarning != nil {
			allowWarnings = allowWarning.GetValue()
		}

		validation = &translator.ValidationOpts{
			ProxyValidationServerAddress: validationCfg.GetProxyValidationServerAddr(),
			ValidatingWebhookPort:        defaults.ValidationWebhookBindPort,
			ValidatingWebhookCertPath:    validationCfg.GetValidationWebhookTlsCert(),
			ValidatingWebhookKeyPath:     validationCfg.GetValidationWebhookTlsKey(),
			IgnoreProxyValidationFailure: validationCfg.GetIgnoreGlooValidationFailure(),
			AlwaysAcceptResources:        alwaysAcceptResources,
			AllowWarnings:                allowWarnings,
			WarnOnRouteShortCircuiting:   validationCfg.GetWarnRouteShortCircuiting().GetValue(),
		}
		if validation.ProxyValidationServerAddress == "" {
			validation.ProxyValidationServerAddress = defaults.GlooProxyValidationServerAddr
		}
		if overrideAddr := os.Getenv("PROXY_VALIDATION_ADDR"); overrideAddr != "" {
			validation.ProxyValidationServerAddress = overrideAddr
		}
		if validation.ValidatingWebhookCertPath == "" {
			validation.ValidatingWebhookCertPath = defaults.ValidationWebhookTlsCertPath
		}
		if validation.ValidatingWebhookKeyPath == "" {
			validation.ValidatingWebhookKeyPath = defaults.ValidationWebhookTlsKeyPath
		}
	} else {
		if validationMustStart := os.Getenv("VALIDATION_MUST_START"); validationMustStart != "" && validationMustStart != "false" {
			return errors.Errorf("VALIDATION_MUST_START was set to true, but no validation configuration was provided in the settings. "+
				"Ensure the v1.Settings %v contains the spec.gateway.validation config", settings.GetMetadata().Ref())
		}
	}

	opts := translator.Opts{
		GlooNamespace:      settings.GetMetadata().GetNamespace(),
		WriteNamespace:     writeNamespace,
		WatchNamespaces:    watchNamespaces,
		Gateways:           gatewayFactory,
		VirtualServices:    virtualServiceFactory,
		RouteTables:        routeTableFactory,
		Proxies:            proxyFactory,
		VirtualHostOptions: virtualHostOptionFactory,
		RouteOptions:       routeOptionFactory,
		WatchOpts: clients.WatchOpts{
			Ctx:         ctx,
			RefreshRate: refreshRate,
		},
		DevMode:                       true,
		ReadGatewaysFromAllNamespaces: settings.GetGateway().GetReadGatewaysFromAllNamespaces(),
		Validation:                    validation,
	}

	return RunGateway(opts)
}

// the need for the namespace is limited to this function, whereas the opts struct's use is more widespread.
func RunGateway(opts translator.Opts) error {
	opts.WatchOpts = opts.WatchOpts.WithDefaults()
	opts.WatchOpts.Ctx = contextutils.WithLogger(opts.WatchOpts.Ctx, "gateway")
	ctx := opts.WatchOpts.Ctx

	gatewayClient, err := v1.NewGatewayClient(ctx, opts.Gateways)
	if err != nil {
		return err
	}
	if err := gatewayClient.Register(); err != nil {
		return err
	}

	virtualServiceClient, err := v1.NewVirtualServiceClient(ctx, opts.VirtualServices)
	if err != nil {
		return err
	}
	if err := virtualServiceClient.Register(); err != nil {
		return err
	}

	routeTableClient, err := v1.NewRouteTableClient(ctx, opts.RouteTables)
	if err != nil {
		return err
	}
	if err := routeTableClient.Register(); err != nil {
		return err
	}

	proxyClient, err := gloov1.NewProxyClient(ctx, opts.Proxies)
	if err != nil {
		return err
	}
	if err := proxyClient.Register(); err != nil {
		return err
	}

	virtualHostOptionClient, err := v1.NewVirtualHostOptionClient(ctx, opts.VirtualHostOptions)
	if err != nil {
		return err
	}
	if err := virtualHostOptionClient.Register(); err != nil {
		return err
	}

	routeOptionClient, err := v1.NewRouteOptionClient(ctx, opts.RouteOptions)
	if err != nil {
		return err
	}
	if err := routeOptionClient.Register(); err != nil {
		return err
	}

	rpt := reporter.NewReporter("gateway",
		gatewayClient.BaseClient(),
		virtualServiceClient.BaseClient(),
		routeTableClient.BaseClient(),
		virtualHostOptionClient.BaseClient(),
		routeOptionClient.BaseClient(),
	)
	writeErrs := make(chan error)

	txlator := translator.NewDefaultTranslator(opts)

	var (
		// this constructor should be called within a lock
		validationClient             validation.ProxyValidationServiceClient
		ignoreProxyValidationFailure bool
		allowWarnings                bool
	)

	// construct the channel that resyncs the API Translator loop
	// when the validation server sends a notification.
	// this tells Gateway that the validation snapshot has changed
	notifications := make(<-chan struct{})

	if opts.Validation != nil {
		validationClient, err = gatewayvalidation.NewConnectionRefreshingValidationClient(
			gatewayvalidation.RetryOnUnavailableClientConstructor(ctx, opts.Validation.ProxyValidationServerAddress),
		)
		if err != nil {
			return errors.Wrapf(err, "failed to initialize grpc connection to validation server.")
		}

		notifications, err = gatewayvalidation.MakeNotificationChannel(ctx, validationClient)
		if err != nil {
			return errors.Wrapf(err, "failed to read notifications from stream")
		}

		ignoreProxyValidationFailure = opts.Validation.IgnoreProxyValidationFailure
		allowWarnings = opts.Validation.AllowWarnings
	}

	emitter := v1.NewApiEmitterWithEmit(virtualServiceClient, routeTableClient, gatewayClient, virtualHostOptionClient, routeOptionClient, notifications)

	validationSyncer := gatewayvalidation.NewValidator(gatewayvalidation.NewValidatorConfig(
		txlator,
		validationClient,
		opts.WriteNamespace,
		ignoreProxyValidationFailure,
		allowWarnings,
	))

	proxyReconciler := reconciler.NewProxyReconciler(validationClient, proxyClient)

	translatorSyncer := NewTranslatorSyncer(
		ctx,
		opts.WriteNamespace,
		proxyClient,
		proxyReconciler,
		rpt,
		txlator)

	gatewaySyncers := v1.ApiSyncers{
		translatorSyncer,
		validationSyncer,
	}

	eventLoop := v1.NewApiEventLoop(emitter, gatewaySyncers)
	eventLoopErrs, err := eventLoop.Run(opts.WatchNamespaces, opts.WatchOpts)
	if err != nil {
		return err
	}
	go errutils.AggregateErrs(ctx, writeErrs, eventLoopErrs, "event_loop")

	logger := contextutils.LoggerFrom(ctx)

	go func() {
		for {
			select {
			case err := <-writeErrs:
				logger.Errorf("error: %v", err)
			case <-ctx.Done():
				return
			}
		}
	}()

	validationServerErr := make(chan error, 1)
	if opts.Validation != nil {
		// make sure non-empty WatchNamespaces contains the gloo instance's own namespace if
		// ReadGatewaysFromAllNamespaces is false
		if !opts.ReadGatewaysFromAllNamespaces && !utils.AllNamespaces(opts.WatchNamespaces) {
			foundSelf := false
			for _, namespace := range opts.WatchNamespaces {
				if opts.GlooNamespace == namespace {
					foundSelf = true
					break
				}
			}
			if !foundSelf {
				return errors.Errorf("The gateway configuration value readGatewaysFromAllNamespaces was set "+
					"to false, but the non-empty settings.watchNamespaces "+
					"list (%s) did not contain this gloo instance's own namespace: %s.",
					strings.Join(opts.WatchNamespaces, ", "), opts.GlooNamespace)
			}
		}

		validationWebhook, err := k8sadmisssion.NewGatewayValidatingWebhook(
			k8sadmisssion.NewWebhookConfig(
				ctx,
				validationSyncer,
				opts.WatchNamespaces,
				opts.Validation.ValidatingWebhookPort,
				opts.Validation.ValidatingWebhookCertPath,
				opts.Validation.ValidatingWebhookKeyPath,
				opts.Validation.AlwaysAcceptResources,
				opts.ReadGatewaysFromAllNamespaces,
				opts.GlooNamespace,
			),
		)
		if err != nil {
			return errors.Wrapf(err, "creating validating webhook")
		}

		go func() {
			// close out validation server when context is cancelled
			<-ctx.Done()
			validationWebhook.Close()
		}()
		go func() {
			contextutils.LoggerFrom(ctx).Infow("starting gateway validation server",
				zap.Int("port", opts.Validation.ValidatingWebhookPort),
				zap.String("cert", opts.Validation.ValidatingWebhookCertPath),
				zap.String("key", opts.Validation.ValidatingWebhookKeyPath),
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

	return nil
}
