package controller

import (
	"context"
	"fmt"
	"net"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	gloot "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
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

type nodeNameNsHasher struct{}

func (h *nodeNameNsHasher) ID(node *envoy_config_core_v3.Node) string {
	if node.GetMetadata() != nil {
		gatewayValue := node.GetMetadata().GetFields()["gateway"].GetStructValue()
		if gatewayValue != nil {
			name := gatewayValue.GetFields()["name"]
			ns := gatewayValue.GetFields()["namespace"]
			if name != nil && ns != nil {
				return fmt.Sprintf("%v~%v", ns.GetStringValue(), name.GetStringValue())
			}
		}
	}

	return xds.FallbackNodeCacheKey
}

func newAdsSnapshotCache(ctx context.Context) cache.SnapshotCache {
	settings := cache.CacheSettings{
		Ads:    true,
		Hash:   &nodeNameNsHasher{},
		Logger: contextutils.LoggerFrom(ctx),
	}
	return cache.NewSnapshotCache(settings)
}

func newGlooTranslator(ctx context.Context) gloot.Translator {

	settings := &gloov1.Settings{}
	opts := bootstrap.Opts{}
	return gloot.NewDefaultTranslator(settings, registry.GetPluginRegistryFactory(opts)(ctx))

}
