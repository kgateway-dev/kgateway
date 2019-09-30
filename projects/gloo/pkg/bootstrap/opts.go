package bootstrap

import (
	"context"
	"net"

	"github.com/solo-io/gloo/projects/gloo/pkg/validation"

	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
)

type Opts struct {
	WriteNamespace    string
	WatchNamespaces   []string
	Upstreams         factory.ResourceClientFactory
	KubeServiceClient skkube.ServiceClient
	UpstreamGroups    factory.ResourceClientFactory
	Proxies           factory.ResourceClientFactory
	Secrets           factory.ResourceClientFactory
	Artifacts         factory.ResourceClientFactory
	AuthConfigs       factory.ResourceClientFactory
	KubeClient        kubernetes.Interface
	ConsulWatcher     consul.ConsulWatcher
	WatchOpts         clients.WatchOpts
	DevMode           bool
	ControlPlane      ControlPlane
	ValidationServer  ValidationServer
	Settings          *v1.Settings
	KubeCoreCache     corecache.KubeCoreCache
}

type ControlPlane struct {
	*GrpcService
	SnapshotCache cache.SnapshotCache
	XDSServer     server.Server
}

type ValidationServer struct {
	*GrpcService
	Server validation.ValidationServer
}

type GrpcService struct {
	Ctx             context.Context
	BindAddr        net.Addr
	GrpcServer      *grpc.Server
	StartGrpcServer bool
}
