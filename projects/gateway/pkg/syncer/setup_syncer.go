package syncer

import (
	"context"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/solo-io/gloo/projects/gateway/pkg/services/k8sadmisssion"

	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gatewayvalidation "github.com/solo-io/gloo/projects/gateway/pkg/validation"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gateway/pkg/propagator"
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

// TODO: expose AllowMissingLinks as a setting
var AllowMissingLinks = true

func Setup(ctx context.Context, kubeCache kube.SharedCache, inMemoryCache memory.InMemoryResourceCache, settings *gloov1.Settings) error {
	var (
		cfg *rest.Config
	)

	consulClient, err := bootstrap.ConsulClientForSettings(settings)
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

	gatewayFactory, err := bootstrap.ConfigFactoryForSettings(params, v2.GatewayCrd)
	if err != nil {
		return err
	}

	refreshRate, err := types.DurationFromProto(settings.RefreshRate)
	if err != nil {
		return err
	}

	writeNamespace := settings.DiscoveryNamespace
	if writeNamespace == "" {
		writeNamespace = gloodefaults.GlooSystem
	}
	watchNamespaces := utils.ProcessWatchNamespaces(settings.WatchNamespaces, writeNamespace)

	var validation *ValidationOpts
	validationCfg := settings.GetGateway().GetValidation()
	if validationCfg != nil {
		alwaysAcceptResources := AcceptAllResourcesByDefault

		if alwaysAccept := validationCfg.AlwaysAccept; alwaysAccept != nil {
			alwaysAcceptResources = alwaysAccept.GetValue()
		}

		allowMissingLinks := AllowMissingLinks

		validation = &ValidationOpts{
			ProxyValidationServerAddress: validationCfg.GetProxyValidationServerAddr(),
			ValidatingWebhookPort:        defaults.ValidationWebhookBindPort,
			ValidatingWebhookCertPath:    validationCfg.GetValidationWebhookTlsCert(),
			ValidatingWebhookKeyPath:     validationCfg.GetValidationWebhookTlsKey(),
			IgnoreProxyValidationFailure: validationCfg.GetIgnoreGlooValidationFailure(),
			AlwaysAcceptResources:        alwaysAcceptResources,
			AllowMissingLinks:            allowMissingLinks,
		}
		if validation.ProxyValidationServerAddress == "" {
			validation.ProxyValidationServerAddress = defaults.GlooProxyValidationServerAddr
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

	opts := Opts{
		WriteNamespace:  writeNamespace,
		WatchNamespaces: watchNamespaces,
		Gateways:        gatewayFactory,
		VirtualServices: virtualServiceFactory,
		RouteTables:     routeTableFactory,
		Proxies:         proxyFactory,
		WatchOpts: clients.WatchOpts{
			Ctx:         ctx,
			RefreshRate: refreshRate,
		},
		DevMode:                true,
		DisableAutoGenGateways: settings.GetGateway().GetDisableAutoGenGateways(),
		Validation:             validation,
	}

	return RunGateway(opts)
}

func RunGateway(opts Opts) error {
	opts.WatchOpts = opts.WatchOpts.WithDefaults()
	opts.WatchOpts.Ctx = contextutils.WithLogger(opts.WatchOpts.Ctx, "gateway")
	ctx := opts.WatchOpts.Ctx

	gatewayClient, err := v2.NewGatewayClient(opts.Gateways)
	if err != nil {
		return err
	}
	if err := gatewayClient.Register(); err != nil {
		return err
	}

	virtualServiceClient, err := v1.NewVirtualServiceClient(opts.VirtualServices)
	if err != nil {
		return err
	}
	if err := virtualServiceClient.Register(); err != nil {
		return err
	}

	routeTableClient, err := v1.NewRouteTableClient(opts.RouteTables)
	if err != nil {
		return err
	}
	if err := routeTableClient.Register(); err != nil {
		return err
	}

	proxyClient, err := gloov1.NewProxyClient(opts.Proxies)
	if err != nil {
		return err
	}
	if err := proxyClient.Register(); err != nil {
		return err
	}

	// The helm install should have created these, but go ahead and try again just in case
	// installing through helm lets these be configurable.
	// Added new setting to disable these gateways from ever being generated
	if !opts.DisableAutoGenGateways {
		for _, gw := range []*v2.Gateway{defaults.DefaultGateway(opts.WriteNamespace), defaults.DefaultSslGateway(opts.WriteNamespace)} {
			if _, err := gatewayClient.Write(gw, clients.WriteOpts{
				Ctx: ctx,
			}); err != nil && !errors.IsExist(err) {
				return err
			}
		}
	}

	emitter := v2.NewApiEmitter(virtualServiceClient, routeTableClient, gatewayClient)

	rpt := reporter.NewReporter("gateway", gatewayClient.BaseClient(), virtualServiceClient.BaseClient(), routeTableClient.BaseClient())
	writeErrs := make(chan error)

	prop := propagator.NewPropagator("gateway", gatewayClient, virtualServiceClient, proxyClient, writeErrs)

	txlator := translator.NewDefaultTranslator()

	translatorSyncer := NewTranslatorSyncer(
		opts.WriteNamespace,
		proxyClient,
		gatewayClient,
		virtualServiceClient,
		rpt,
		prop,
		txlator)

	var (
		// this constructor should be called within a lock
		validationClient             validation.ProxyValidationServiceClient
		ignoreProxyValidationFailure bool
		allowMissingLinks            bool
	)
	if opts.Validation != nil {
		validationClient, err = gatewayvalidation.NewConnectionRefreshingValidationClient(
			gatewayvalidation.RetryOnUnavailableClientConstructor(ctx, opts.Validation.ProxyValidationServerAddress),
		)
		if err != nil {
			return errors.Wrapf(err, "failed to initialize grpc connection to validation server.")
		}

		ignoreProxyValidationFailure = opts.Validation.IgnoreProxyValidationFailure
		allowMissingLinks = opts.Validation.AllowMissingLinks
	}

	validationSyncer := gatewayvalidation.NewValidator(gatewayvalidation.NewValidatorConfig(
		txlator,
		validationClient,
		opts.WriteNamespace,
		ignoreProxyValidationFailure,
		allowMissingLinks,
	))

	gatewaySyncers := v2.ApiSyncers{
		translatorSyncer,
		validationSyncer,
	}

	eventLoop := v2.NewApiEventLoop(emitter, gatewaySyncers)
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
		validationWebhook, err := k8sadmisssion.NewGatewayValidatingWebhook(
			k8sadmisssion.NewWebhookConfig(
				ctx,
				validationSyncer,
				opts.WatchNamespaces,
				opts.Validation.ValidatingWebhookPort,
				opts.Validation.ValidatingWebhookCertPath,
				opts.Validation.ValidatingWebhookKeyPath,
				opts.Validation.AlwaysAcceptResources,
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
