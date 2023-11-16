package server

import (
	"context"
	"fmt"
	"net"

	xdsutils "github.com/solo-io/gloo/v2/pkg/xds/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"

	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"

	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"

	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"

	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"

	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"

	envoyserver "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
)

// register xDS methods with GRPC server
func SetupEnvoyXds(grpcServer *grpc.Server, xdsServer envoyserver.Server, envoyCache envoycache.SnapshotCache) {

	// check if we need to register
	if _, ok := grpcServer.GetServiceInfo()["solo.io.xds.SoloDiscoveryService"]; ok {
		return
	}

	// The Gloo Server is an xDS server that accepts v2 Envoy ADS requests. The Envoy v2 API has been
	// deprecated but the ADS api has been preserved internally to support discovery of
	// ext-auth and rate-limit configurations.
	//	glooServer := NewGlooXdsServer(xdsServer)
	//	solo_xds.RegisterSoloDiscoveryServiceServer(grpcServer, glooServer)

	envoyServer := NewEnvoyServerV3(xdsServer)
	envoy_service_endpoint_v3.RegisterEndpointDiscoveryServiceServer(grpcServer, envoyServer)
	envoy_service_cluster_v3.RegisterClusterDiscoveryServiceServer(grpcServer, envoyServer)
	envoy_service_route_v3.RegisterRouteDiscoveryServiceServer(grpcServer, envoyServer)
	envoy_service_listener_v3.RegisterListenerDiscoveryServiceServer(grpcServer, envoyServer)
	envoy_service_discovery_v3.RegisterAggregatedDiscoveryServiceServer(grpcServer, envoyServer)

	// Seed the cache with a fallback snapshot
	envoyCache.SetSnapshot(xdsutils.FallbackNodeCacheKey, createFallbackSnapshot())
}

func NewServer(ctx context.Context, port uint16, xdsSyncer cache.SnapshotCache) manager.RunnableFunc {
	return func(ctx context.Context) error {
		grpcServer := grpc.NewServer()

		addr := fmt.Sprintf(":%d", port)
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}

		go func() {
			<-ctx.Done()
			grpcServer.GracefulStop()
		}()
		xdsServer := server.NewServer(ctx, xdsSyncer, nil)
		reflection.Register(grpcServer)

		SetupEnvoyXds(grpcServer, xdsServer, xdsSyncer)

		return grpcServer.Serve(lis)
	}
}
