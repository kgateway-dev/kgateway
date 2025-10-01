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
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"istio.io/istio/pkg/security"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/xds"
)

func NewControlPlane(
	ctx context.Context,
	bindAddr net.Addr,
	callbacks xdsserver.Callbacks,
	authenticators []security.Authenticator,
	xdsAuth bool,
) (envoycache.SnapshotCache, error) {
	lis, err := net.Listen(bindAddr.Network(), bindAddr.String())
	if err != nil {
		return nil, err
	}
	snapshotCache, grpcServer := NewControlPlaneWithListener(ctx, lis, callbacks, authenticators, xdsAuth)
	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()
	return snapshotCache, err
}

func NewControlPlaneWithListener(ctx context.Context,
	lis net.Listener,
	callbacks xdsserver.Callbacks,
	authenticators []security.Authenticator,
	xdsAuth bool,
) (envoycache.SnapshotCache, *grpc.Server) {
	logger := contextutils.LoggerFrom(ctx).Desugar()
	grpcServer := grpc.NewServer(getGRPCServerOpts(authenticators, xdsAuth)...)

	snapshotCache := envoycache.NewSnapshotCache(true, xds.NewNodeRoleHasher(), logger.Sugar())

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

func getGRPCServerOpts(authenticators []security.Authenticator, xdsAuth bool) []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.MaxRecvMsgSize(math.MaxInt32),
		grpc.StreamInterceptor(
			grpc_middleware.ChainStreamServer(
				grpc_zap.StreamServerInterceptor(zap.NewNop()),
				func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
					slog.Debug("gRPC call", "method", info.FullMethod)
					if xdsAuth {
						am := authenticationManager{
							Authenticators: authenticators,
						}
						if u := am.authenticate(ss.Context()); u != nil {
							slog.Debug("xDS auth succeeded")
							return handler(srv, &grpc_middleware.WrappedServerStream{
								ServerStream:   ss,
								WrappedContext: context.WithValue(ss.Context(), xds.PeerCtxKey, u),
							})
						}
						slog.Error("xDS authentication failed", "reasons", am.authFailMsgs)
						return fmt.Errorf("authentication failed: %v", am.authFailMsgs)
					} else {
						slog.Warn("xDS authentication is disabled")
						return handler(srv, ss)
					}
				},
			)),
	}
}
