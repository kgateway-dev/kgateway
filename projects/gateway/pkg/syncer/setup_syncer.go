package syncer

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gatewayvalidation "github.com/solo-io/gloo/projects/gateway/pkg/validation"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gateway/pkg/propagator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	gloodefaults "github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/reporter"
	"github.com/solo-io/solo-kit/pkg/errors"
	"k8s.io/client-go/rest"
)

const DefaultValidationServerAddress = "gloo:9988"

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

	validationServerAddress := settings.GetGateway().GetValidationServerAddr()
	if validationServerAddress == "" {
		validationServerAddress = DefaultValidationServerAddress
	}

	opts := Opts{
		WriteNamespace:          writeNamespace,
		WatchNamespaces:         watchNamespaces,
		Gateways:                gatewayFactory,
		VirtualServices:         virtualServiceFactory,
		RouteTables:             routeTableFactory,
		Proxies:                 proxyFactory,
		ValidationServerAddress: validationServerAddress,
		WatchOpts: clients.WatchOpts{
			Ctx:         ctx,
			RefreshRate: refreshRate,
		},
		DevMode: true,
	}

	return RunGateway(opts)
}

func RunGateway(opts Opts) error {
	opts.WatchOpts = opts.WatchOpts.WithDefaults()
	opts.WatchOpts.Ctx = contextutils.WithLogger(opts.WatchOpts.Ctx, "gateway")

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

	for _, gw := range []*v2.Gateway{defaults.DefaultGateway(opts.WriteNamespace), defaults.DefaultSslGateway(opts.WriteNamespace)} {
		if _, err := gatewayClient.Write(gw, clients.WriteOpts{
			Ctx: opts.WatchOpts.Ctx,
		}); err != nil && !errors.IsExist(err) {
			return err
		}
	}

	emitter := v2.NewApiEmitter(virtualServiceClient, routeTableClient, gatewayClient)

	rpt := reporter.NewReporter("gateway", gatewayClient.BaseClient(), virtualServiceClient.BaseClient())
	writeErrs := make(chan error)

	prop := propagator.NewPropagator("gateway", gatewayClient, virtualServiceClient, proxyClient, writeErrs)

	var validationClient validation.ProxyValidationServiceClient
	cc, err := grpc.DialContext(opts.WatchOpts.Ctx, opts.ValidationServerAddress, grpc.WithBlock())
	if err == nil {
		validationClient = validation.NewProxyValidationServiceClient(cc)
	} else {
		contextutils.LoggerFrom(opts.WatchOpts.Ctx).Errorw("failed to initialize grpc connection to validation server. validation will not be enabled", zap.Error(err))
	}

	t := translator.NewDefaultTranslator()

	translatorSyncer := NewTranslatorSyncer(
		opts.WriteNamespace,
		proxyClient,
		gatewayClient,
		virtualServiceClient,
		rpt,
		prop,
		t)

	validationSyncer := gatewayvalidation.NewValidator(t, validationClient, opts.WriteNamespace)

	gatewaySyncers := v2.ApiSyncers{
		translatorSyncer,
		validationSyncer,
	}

	eventLoop := v2.NewApiEventLoop(emitter, gatewaySyncers)
	eventLoopErrs, err := eventLoop.Run(opts.WatchNamespaces, opts.WatchOpts)
	if err != nil {
		return err
	}
	go errutils.AggregateErrs(opts.WatchOpts.Ctx, writeErrs, eventLoopErrs, "event_loop")

	logger := contextutils.LoggerFrom(opts.WatchOpts.Ctx)

	go func() {
		for {
			select {
			case err := <-writeErrs:
				logger.Errorf("error: %v", err)
			case <-opts.WatchOpts.Ctx.Done():
				return
			}
		}
	}()
	return nil
}
