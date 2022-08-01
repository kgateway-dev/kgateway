package runner

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/debug"

	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	xdsserver "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// TODO: (copied from gateway) switch AcceptAllResourcesByDefault to false after validation has been tested in user environments
var AcceptAllResourcesByDefault = true

var AllowWarnings = true

func NewRunnerFactory() bootstrap.RunnerFactory {
	return NewGlooRunnerFactory(RunGloo, nil).GetRunnerFactory()
}

// used outside of this repo
//noinspection GoUnusedExportedFunction
func NewRunnerFactoryWithExtensions(extensions RunExtensions) bootstrap.RunnerFactory {
	runWithExtensions := func(opts RunOpts) error {
		return RunGlooWithExtensions(opts, extensions)
	}
	return NewGlooRunnerFactory(runWithExtensions, &extensions).GetRunnerFactory()
}

// for use by UDS, FDS, other v1.SetupSyncers
func NewRunnerFactoryWithRun(runFunc RunWithOptions) bootstrap.RunnerFactory {
	return NewGlooRunnerFactory(runFunc, nil).GetRunnerFactory()
}

type glooRunnerFactory struct {
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

func NewGlooRunnerFactory(runFunc RunWithOptions, extensions *RunExtensions) *glooRunnerFactory {
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
	return s
}

func (g *glooRunnerFactory) GetRunnerFactory() bootstrap.RunnerFactory {
	return g.RunnerFactoryImpl
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

func (g *glooRunnerFactory) RunnerFactoryImpl(ctx context.Context, kubeCache kube.SharedCache, memCache memory.InMemoryResourceCache, settings *v1.Settings) (bootstrap.RunFunc, error) {
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
	if xdsAddr != g.previousXdsServer.addr {
		if g.previousXdsServer.cancel != nil {
			g.previousXdsServer.cancel()
			g.previousXdsServer.cancel = nil
		}
		g.controlPlane = emptyControlPlane
	}

	// check if we need to restart the validation server
	if validationAddr != g.previousValidationServer.addr || maxGrpcRecvSize != g.previousValidationServer.maxGrpcRecvSize {
		if g.previousValidationServer.cancel != nil {
			g.previousValidationServer.cancel()
			g.previousValidationServer.cancel = nil
		}
		g.validationServer = emptyValidationServer
	}

	// check if we need to restart the proxy debug server
	if proxyDebugAddr != g.previousProxyDebugServer.addr || maxGrpcRecvSize != g.previousProxyDebugServer.maxGrpcRecvSize {
		if g.previousProxyDebugServer.cancel != nil {
			g.previousProxyDebugServer.cancel()
			g.previousProxyDebugServer.cancel = nil
		}
		g.proxyDebugServer = emptyProxyDebugServer
	}

	// initialize the control plane context in this block either on the first loop, or if bind addr changed
	if g.controlPlane == emptyControlPlane {
		// create new context as the grpc server might survive multiple iterations of this loop.
		ctx, cancel := context.WithCancel(context.Background())
		var callbacks xdsserver.Callbacks
		if g.extensions != nil {
			callbacks = g.extensions.XdsCallbacks
		}
		g.controlPlane = NewControlPlane(ctx, g.makeGrpcServer(ctx), xdsTcpAddress, callbacks, true)
		g.previousXdsServer.cancel = cancel
		g.previousXdsServer.addr = xdsAddr
	}

	// initialize the validation server context in this block either on the first loop, or if bind addr changed
	if g.validationServer == emptyValidationServer {
		// create new context as the grpc server might survive multiple iterations of this loop.
		ctx, cancel := context.WithCancel(context.Background())
		var validationGrpcServerOpts []grpc.ServerOption
		// if validationServerGrpcMaxSizeBytes was set this will be non-negative, otherwise use gRPC default
		if maxGrpcRecvSize >= 0 {
			validationGrpcServerOpts = append(validationGrpcServerOpts, grpc.MaxRecvMsgSize(maxGrpcRecvSize))
		}
		g.validationServer = NewValidationServer(ctx, g.makeGrpcServer(ctx, validationGrpcServerOpts...), validationTcpAddress, true)
		g.previousValidationServer.cancel = cancel
		g.previousValidationServer.addr = validationAddr
		g.previousValidationServer.maxGrpcRecvSize = maxGrpcRecvSize
	}
	// initialize the proxy debug server context in this block either on the first loop, or if bind addr changed
	if g.proxyDebugServer == emptyProxyDebugServer {
		// create new context as the grpc server might survive multiple iterations of this loop.
		ctx, cancel := context.WithCancel(context.Background())

		proxyGrpcServerOpts := []grpc.ServerOption{grpc.MaxRecvMsgSize(maxGrpcRecvSize)}
		g.proxyDebugServer = NewProxyDebugServer(ctx, g.makeGrpcServer(ctx, proxyGrpcServerOpts...), proxyDebugTcpAddress, true)
		g.previousProxyDebugServer.cancel = cancel
		g.previousProxyDebugServer.addr = proxyDebugAddr
		g.previousProxyDebugServer.maxGrpcRecvSize = maxGrpcRecvSize
	}

	// Generate the set of clients used to power Gloo Edge
	resourceClientset, typedClientset, err := GenerateGlooClientsets(ctx, settings, kubeCache, memCache)
	if err != nil {
		return nil, err
	}
	g.resourceClientset = resourceClientset
	g.typedClientset = typedClientset

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
	opts.ControlPlane = g.controlPlane
	opts.ValidationServer = g.validationServer
	opts.ProxyDebugServer = g.proxyDebugServer

	return func() error {
		err = g.runFunc(opts)

		g.validationServer.StartGrpcServer = opts.ValidationServer.StartGrpcServer
		g.controlPlane.StartGrpcServer = opts.ControlPlane.StartGrpcServer

		return err
	}, nil
}
