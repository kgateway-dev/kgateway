package setup

import (
	"context"
	"net"

	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/xds"
)

func NewControlPlane(
	ctx context.Context,
	bindAddr net.Addr,
	callbacks xdsserver.Callbacks) (envoycache.SnapshotCache, error) {
	lis, err := net.Listen(bindAddr.Network(), bindAddr.String())
	if err != nil {
		return nil, err
	}
	return NewControlPlaneWithListener(ctx, lis, callbacks)
}

func NewControlPlaneWithListener(ctx context.Context,
	lis net.Listener,
	callbacks xdsserver.Callbacks) (envoycache.SnapshotCache, error) {
	logger := contextutils.LoggerFrom(ctx).Desugar()
	serverOpts := []grpc.ServerOption{
		grpc.StreamInterceptor(
			grpc_middleware.ChainStreamServer(
				//				grpc_ctxtags.StreamServerInterceptor(),
				grpc_zap.StreamServerInterceptor(zap.NewNop()),
				func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
					logger.Debug("gRPC call", zap.String("method", info.FullMethod))
					return handler(srv, ss)
				},
			)),
	}
	grpcServer := grpc.NewServer(serverOpts...)

	snapshotCache := envoycache.NewSnapshotCache(true, xds.NewNodeRoleHasher(), logger.Sugar())

	xdsServer := xdsserver.NewServer(ctx, snapshotCache, callbacks)
	reflection.Register(grpcServer)

	envoy_service_endpoint_v3.RegisterEndpointDiscoveryServiceServer(grpcServer, xdsServer)
	envoy_service_cluster_v3.RegisterClusterDiscoveryServiceServer(grpcServer, xdsServer)
	envoy_service_route_v3.RegisterRouteDiscoveryServiceServer(grpcServer, xdsServer)
	envoy_service_listener_v3.RegisterListenerDiscoveryServiceServer(grpcServer, xdsServer)
	envoy_service_discovery_v3.RegisterAggregatedDiscoveryServiceServer(grpcServer, xdsServer)

	go grpcServer.Serve(lis)
	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	return snapshotCache, nil
}
