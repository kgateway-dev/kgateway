package setup

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net"

	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoylog "github.com/envoyproxy/go-control-plane/pkg/log"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/xds"
)

// slogAdapterForEnvoy adapts *slog.Logger to envoylog.Logger interface
type slogAdapterForEnvoy struct {
	logger *slog.Logger
}

// Ensure it implements the interface
var _ envoylog.Logger = (*slogAdapterForEnvoy)(nil)

func (s *slogAdapterForEnvoy) Debugf(format string, args ...interface{}) {
	if s.logger.Enabled(context.Background(), slog.LevelDebug) {
		s.logger.Debug(fmt.Sprintf(format, args...)) //nolint:sloglint // ignore formatting
	}
}

func (s *slogAdapterForEnvoy) Infof(format string, args ...interface{}) {
	if s.logger.Enabled(context.Background(), slog.LevelInfo) {
		s.logger.Info(fmt.Sprintf(format, args...)) //nolint:sloglint // ignore formatting
	}
}

func (s *slogAdapterForEnvoy) Warnf(format string, args ...interface{}) {
	if s.logger.Enabled(context.Background(), slog.LevelWarn) {
		s.logger.Warn(fmt.Sprintf(format, args...)) //nolint:sloglint // ignore formatting
	}
}

func (s *slogAdapterForEnvoy) Errorf(format string, args ...interface{}) {
	if s.logger.Enabled(context.Background(), slog.LevelError) {
		s.logger.Error(fmt.Sprintf(format, args...)) //nolint:sloglint // ignore formatting
	}
}

func NewControlPlane(
	ctx context.Context,
	bindAddr net.Addr,
	callbacks xdsserver.Callbacks,
) (envoycache.SnapshotCache, error) {
	lis, err := net.Listen(bindAddr.Network(), bindAddr.String())
	if err != nil {
		return nil, err
	}
	snapshotCache, grpcServer := NewControlPlaneWithListener(ctx, lis, callbacks)
	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()
	return snapshotCache, err
}

func NewControlPlaneWithListener(ctx context.Context,
	lis net.Listener,
	callbacks xdsserver.Callbacks,
) (envoycache.SnapshotCache, *grpc.Server) {
	baseLogger := slog.Default().With("component", "envoy-controlplane")
	envoyLoggerAdapter := &slogAdapterForEnvoy{logger: baseLogger}

	serverOpts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(math.MaxInt32),
		grpc.StreamInterceptor(
			grpc_middleware.ChainStreamServer(
				//				grpc_ctxtags.StreamServerInterceptor(),
				grpc_zap.StreamServerInterceptor(zap.NewNop()),
				func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
					baseLogger.Debug("gRPC call", "method", info.FullMethod)
					return handler(srv, ss)
				},
			)),
	}
	grpcServer := grpc.NewServer(serverOpts...)

	snapshotCache := envoycache.NewSnapshotCache(true, xds.NewNodeRoleHasher(), envoyLoggerAdapter)

	xdsServer := xdsserver.NewServer(ctx, snapshotCache, callbacks)
	reflection.Register(grpcServer)

	envoy_service_endpoint_v3.RegisterEndpointDiscoveryServiceServer(grpcServer, xdsServer)
	envoy_service_cluster_v3.RegisterClusterDiscoveryServiceServer(grpcServer, xdsServer)
	envoy_service_route_v3.RegisterRouteDiscoveryServiceServer(grpcServer, xdsServer)
	envoy_service_listener_v3.RegisterListenerDiscoveryServiceServer(grpcServer, xdsServer)
	envoy_service_discovery_v3.RegisterAggregatedDiscoveryServiceServer(grpcServer, xdsServer)

	go grpcServer.Serve(lis)

	return snapshotCache, grpcServer
}
