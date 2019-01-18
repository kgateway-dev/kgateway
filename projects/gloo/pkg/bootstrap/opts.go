package bootstrap

import (
	"net"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
)

type Opts struct {
	WriteNamespace  string
	WatchNamespaces []string
	Upstreams       factory.ResourceClientFactory
	Proxies         factory.ResourceClientFactory
	Secrets         factory.ResourceClientFactory
	Artifacts       factory.ResourceClientFactory
	BindAddr        net.Addr
	KubeClient      kubernetes.Interface
	WatchOpts       clients.WatchOpts
	DevMode         bool
	ControlPlane    ControlPlane
	Extensions      *v1.Extensions
}

type ControlPlane struct {
	GrpcServer      *grpc.Server
	StartGrpcServer bool
	SnapshotCache   cache.SnapshotCache
	XDSServer       server.Server
}
