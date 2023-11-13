package controller

import (
	"context"
	"fmt"
	"net"

	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

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

		xds.SetupEnvoyXds(grpcServer, xdsServer, xdsSyncer)

		return grpcServer.Serve(lis)
	}
}
