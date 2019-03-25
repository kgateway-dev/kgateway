package syncer

import (
	"context"
	"net"
	"strconv"
	"strings"

	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"

	"github.com/gogo/protobuf/types"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/pkg/utils/setuputils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	xdsserver "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	"github.com/solo-io/solo-kit/pkg/api/v1/reporter"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/utils/errutils"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	"go.uber.org/zap"

	envoyv2 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type RunFunc func(opts bootstrap.Opts) error

func NewSetupFunc() setuputils.SetupFunc {
	return NewSetupFuncWithRunAndExtensions(RunGloo, nil)
}

// used outside of this repo
func NewSetupFuncWithExtensions(extensions Extensions) setuputils.SetupFunc {
	runWithExtensions := func(opts bootstrap.Opts) error {
		return RunGlooWithExtensions(opts, extensions)
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
		grpcServer: func(ctx context.Context) *grpc.Server {
			return grpc.NewServer(grpc.StreamInterceptor(
				grpc_middleware.ChainStreamServer(
					grpc_ctxtags.StreamServerInterceptor(),
					grpc_zap.StreamServerInterceptor(zap.NewNop()),
					func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
						contextutils.LoggerFrom(ctx).Debugf("gRPC call: %v", info.FullMethod)
						return handler(srv, ss)
					},
				)),
			)
		},
		runFunc: runFunc,
	}
	return s.Setup
}

type setupSyncer struct {
	extensions         *Extensions
	runFunc            RunFunc
	grpcServer         func(ctx context.Context) *grpc.Server
	previousBindAddr   string
	controlPlane       bootstrap.ControlPlane
	cancelControlPlane context.CancelFunc
	callbacks          xdsserver.Callbacks
}

func NewControlPlane(ctx context.Context, grpcServer *grpc.Server, callbacks xdsserver.Callbacks, start bool) bootstrap.ControlPlane {
	var c bootstrap.ControlPlane
	c.GrpcServer = grpcServer
	hasher := &xds.ProxyKeyHasher{}
	snapshotCache := cache.NewSnapshotCache(true, hasher, contextutils.LoggerFrom(ctx))
	xdsServer := server.NewServer(snapshotCache, callbacks)
	envoyv2.RegisterAggregatedDiscoveryServiceServer(c.GrpcServer, xdsServer)
	c.SnapshotCache = snapshotCache
	c.XDSServer = xdsServer
	c.StartGrpcServer = start
	return c
}

func (s *setupSyncer) Setup(ctx context.Context, kubeCache kube.SharedCache, memCache memory.InMemoryResourceCache, settings *v1.Settings) error {

	ipPort := strings.Split(settings.BindAddr, ":")
	if len(ipPort) != 2 {
		return errors.Errorf("invalid bind addr: %v", settings.BindAddr)
	}
	port, err := strconv.Atoi(ipPort[1])
	if err != nil {
		return errors.Wrapf(err, "invalid bind addr: %v", settings.BindAddr)
	}
	refreshRate, err := types.DurationFromProto(settings.RefreshRate)
	if err != nil {
		return err
	}

	writeNamespace := settings.DiscoveryNamespace
	if writeNamespace == "" {
		writeNamespace = defaults.GlooSystem
	}
	watchNamespaces := utils.ProcessWatchNamespaces(settings.WatchNamespaces, writeNamespace)

	empty := bootstrap.ControlPlane{}

	if settings.BindAddr != s.previousBindAddr {
		if s.cancelControlPlane != nil {
			s.cancelControlPlane()
			s.cancelControlPlane = nil
		}
		s.controlPlane = empty
	}

	// enter this block either on the first loop, or if bind addr changed
	if s.controlPlane == empty {
		// create new context as the grpc server might survive multiple iterations of this loop.
		ctx, cancel := context.WithCancel(context.Background())
		var callbacks xdsserver.Callbacks
		if s.extensions != nil {
			callbacks = s.extensions.XdsCallbacks
		}
		s.controlPlane = NewControlPlane(ctx, s.grpcServer(ctx), callbacks, true)
		s.cancelControlPlane = cancel
	}

	var clientset kubernetes.Interface
	opts, err := BootstrapFactories(ctx, &clientset, kubeCache, memCache, settings)
	if err != nil {
		return err
	}
	opts.WriteNamespace = writeNamespace
	opts.WatchNamespaces = watchNamespaces
	opts.WatchOpts = clients.WatchOpts{
		Ctx:         ctx,
		RefreshRate: refreshRate,
	}
	opts.BindAddr = &net.TCPAddr{
		IP:   net.ParseIP(ipPort[0]),
		Port: port,
	}
	opts.ControlPlane = s.controlPlane
	// if nil, kube plugin disabled
	opts.KubeClient = clientset
	opts.DevMode = true
	opts.Extensions = settings.Extensions

	return s.runFunc(opts)
}

type Extensions struct {
	PluginExtensions []plugins.Plugin
	SyncerExtensions []TranslatorSyncerExtensionFactory
	XdsCallbacks     xdsserver.Callbacks
}

func RunGloo(opts bootstrap.Opts) error {
	return RunGlooWithExtensions(opts, Extensions{})
}

func RunGlooWithExtensions(opts bootstrap.Opts, extensions Extensions) error {
	watchOpts := opts.WatchOpts.WithDefaults()
	opts.WatchOpts.Ctx = contextutils.WithLogger(opts.WatchOpts.Ctx, "gloo")

	watchOpts.Ctx = contextutils.WithLogger(watchOpts.Ctx, "setup")
	endpointsFactory := &factory.MemoryResourceClientFactory{
		Cache: memory.NewInMemoryResourceCache(),
	}

	upstreamClient, err := v1.NewUpstreamClient(opts.Upstreams)
	if err != nil {
		return err
	}
	if err := upstreamClient.Register(); err != nil {
		return err
	}

	proxyClient, err := v1.NewProxyClient(opts.Proxies)
	if err != nil {
		return err
	}
	if err := proxyClient.Register(); err != nil {
		return err
	}

	upstreamGroupClient, err := v1.NewUpstreamGroupClient(opts.UpstreamGroups)
	if err != nil {
		return err
	}
	if err := upstreamGroupClient.Register(); err != nil {
		return err
	}

	endpointClient, err := v1.NewEndpointClient(endpointsFactory)
	if err != nil {
		return err
	}

	secretClient, err := v1.NewSecretClient(opts.Secrets)
	if err != nil {
		return err
	}

	artifactClient, err := v1.NewArtifactClient(opts.Artifacts)
	if err != nil {
		return err
	}

	cache := v1.NewApiEmitter(artifactClient, endpointClient, proxyClient, upstreamGroupClient, secretClient, upstreamClient)

	// Register grpc endpoints to the grpc server
	xdsHasher := xds.SetupEnvoyXds(opts.ControlPlane.GrpcServer, opts.ControlPlane.XDSServer, opts.ControlPlane.SnapshotCache)

	rpt := reporter.NewReporter("gloo", upstreamClient.BaseClient(), proxyClient.BaseClient())

	plugins := registry.Plugins(opts, extensions.PluginExtensions...)

	var discoveryPlugins []discovery.DiscoveryPlugin
	for _, plug := range plugins {
		disc, ok := plug.(discovery.DiscoveryPlugin)
		if ok {
			discoveryPlugins = append(discoveryPlugins, disc)
		}
	}
	logger := contextutils.LoggerFrom(watchOpts.Ctx)

	var syncerExtensions []TranslatorSyncerExtension
	params := TranslatorSyncerExtensionParams{
		SettingExtensions: opts.Extensions,
	}
	for _, syncerExtensionFactory := range extensions.SyncerExtensions {
		syncerExtension, err := syncerExtensionFactory(watchOpts.Ctx, params)
		if err != nil {
			logger.Errorw("Error initializing extension", "error", err)
			continue
		}
		syncerExtensions = append(syncerExtensions, syncerExtension)
	}

	sync := NewTranslatorSyncer(translator.NewTranslator(plugins, opts.Extensions), opts.ControlPlane.SnapshotCache, xdsHasher, rpt, opts.DevMode, syncerExtensions)
	eventLoop := v1.NewApiEventLoop(cache, sync)

	errs := make(chan error)

	eds := discovery.NewEndpointDiscovery(opts.WriteNamespace, endpointClient, discoveryPlugins)
	edsErrs, err := discovery.RunEds(upstreamClient, eds, opts.WriteNamespace, watchOpts)
	if err != nil {
		return err
	}
	go errutils.AggregateErrs(watchOpts.Ctx, errs, edsErrs, "eds.gloo")

	eventLoopErrs, err := eventLoop.Run(opts.WatchNamespaces, watchOpts)
	if err != nil {
		return err
	}
	go errutils.AggregateErrs(watchOpts.Ctx, errs, eventLoopErrs, "event_loop.gloo")

	go func() {
		for {
			select {
			case <-watchOpts.Ctx.Done():
				logger.Debugf("context cancelled")
				return
			}
		}
	}()

	if !opts.ControlPlane.StartGrpcServer {
		return nil
	}

	lis, err := net.Listen(opts.BindAddr.Network(), opts.BindAddr.String())
	if err != nil {
		return err
	}
	go func() {
		<-opts.WatchOpts.Ctx.Done()
		opts.ControlPlane.GrpcServer.Stop()
	}()

	go func() {
		if err := opts.ControlPlane.GrpcServer.Serve(lis); err != nil {
			logger.Errorf("grpc server failed to start")
		}
	}()
	return nil
}

func BootstrapFactories(ctx context.Context, clientset *kubernetes.Interface, kubeCache kube.SharedCache, memCache memory.InMemoryResourceCache, settings *v1.Settings) (bootstrap.Opts, error) {

	var (
		cfg           *rest.Config
		kubeCoreCache corecache.KubeCoreCache
	)

	upstreamFactory, err := bootstrap.ConfigFactoryForSettings(
		settings,
		memCache,
		kubeCache,
		v1.UpstreamCrd,
		&cfg,
	)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	proxyFactory, err := bootstrap.ConfigFactoryForSettings(
		settings,
		memCache,
		kubeCache,
		v1.ProxyCrd,
		&cfg,
	)
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
		v1.SecretCrd.Plural,
	)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	upstreamGroupFactory, err := bootstrap.ConfigFactoryForSettings(
		settings,
		memCache,
		kubeCache,
		v1.UpstreamGroupCrd,
		&cfg,
	)
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
		v1.ArtifactCrd.Plural,
	)
	if err != nil {
		return bootstrap.Opts{}, err
	}
	return bootstrap.Opts{
		Upstreams:      upstreamFactory,
		Proxies:        proxyFactory,
		UpstreamGroups: upstreamGroupFactory,
		Secrets:        secretFactory,
		Artifacts:      artifactFactory,
	}, nil
}
