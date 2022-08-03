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

	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"

	xdsserver "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

var _ bootstrap.Runner = new(glooRunner)

// TODO: (copied from gateway) switch AcceptAllResourcesByDefault to false after validation has been tested in user environments
var AcceptAllResourcesByDefault = true

var AllowWarnings = true

type glooRunner struct {
	extensions *RunExtensions

	resourceClientset ResourceClientset
	typedClientset    TypedClientset

	makeGrpcServer           func(ctx context.Context, options ...grpc.ServerOption) *grpc.Server
	previousXdsServer        grpcServer
	previousValidationServer grpcServer
	previousProxyDebugServer grpcServer
	controlPlane             ControlPlane
	validationServer         ValidationServer
	proxyDebugServer         ProxyDebugServer
	callbacks                xdsserver.Callbacks
}

func NewGlooRunner() *glooRunner {
	return NewGlooRunnerWithExtensions(DefaultRunExtensions())
}

func NewGlooRunnerWithExtensions(extensions *RunExtensions) *glooRunner {
	s := &glooRunner{
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
	}
	return s
}

func (g *glooRunner) GetResourceClientset() ResourceClientset {
	return g.resourceClientset
}

func (g *glooRunner) GetTypedClientset() TypedClientset {
	return g.typedClientset
}

// grpcServer contains grpc server configuration fields we will need to persist after starting a server
// to later check if they changed and we need to trigger a server restart
type grpcServer struct {
	addr            string
	maxGrpcRecvSize int
	cancel          context.CancelFunc
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

func (g *glooRunner) Run(ctx context.Context, kubeCache kube.SharedCache, memCache memory.InMemoryResourceCache, settings *v1.Settings) error {
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

	proxyDebugAddr := settings.GetGloo().GetProxyDebugBindAddr()
	if proxyDebugAddr == "" {
		proxyDebugAddr = DefaultProxyDebugAddr
	}
	proxyDebugTcpAddress, err := getAddr(proxyDebugAddr)
	if err != nil {
		return errors.Wrapf(err, "parsing proxy debug endpoint addr")
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
			return errors.Errorf("validationServerGrpcMaxSizeBytes in settings CRD must be non-negative, current value: %v", maxGrpcMsgSize.GetValue())
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
		return err
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

	// TODO (samheilbron) we should remove the whole concept of RunOpts
	opts.ControlPlane = g.controlPlane
	opts.ValidationServer = g.validationServer
	opts.ProxyDebugServer = g.proxyDebugServer

	err = RunGlooWithExtensions(opts, *g.extensions)

	g.validationServer.StartGrpcServer = opts.ValidationServer.StartGrpcServer
	g.controlPlane.StartGrpcServer = opts.ControlPlane.StartGrpcServer

	return err
}
