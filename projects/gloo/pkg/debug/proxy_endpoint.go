package debug

import (
	"context"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/debug"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"google.golang.org/grpc"
)

type ProxyEndpointServer interface {
	debug.ProxyEndpointServiceServer
	Register(grpcServer *grpc.Server)
	SetProxyClient(client v1.ProxyClient)
}
type proxyEndpointServer struct {
	proxyClient v1.ProxyClient
}

func NewProxyEndpointServer() *proxyEndpointServer {
	return &proxyEndpointServer{}
}
func (p *proxyEndpointServer) SetProxyClient(proxyClient v1.ProxyClient) {
	p.proxyClient = proxyClient
}

// GetProxies receives a request from outside the gloo pod and returns a filtered list of proxies in a format that mirrors the k8s client
func (p *proxyEndpointServer) GetProxies(ctx context.Context, req *debug.ProxyEndpointRequest) (*debug.ProxyEndpointResponse, error) {
	contextutils.LoggerFrom(ctx).Infof("received grpc request to read proxies")
	if req.GetName() == "" {
		proxies, err := p.proxyClient.List(req.GetNamespace(), clients.ListOpts{
			Ctx:      ctx,
			Selector: req.GetSelector(),
		})
		if err != nil {
			return nil, err
		}
		return &debug.ProxyEndpointResponse{Proxies: proxies}, nil
	} else {
		proxy, err := p.proxyClient.Read(req.GetNamespace(), req.GetName(), clients.ReadOpts{Ctx: ctx})
		if err != nil {
			return nil, err
		}
		return &debug.ProxyEndpointResponse{Proxies: v1.ProxyList{proxy}}, nil
	}
}

func (p *proxyEndpointServer) Register(grpcServer *grpc.Server) {
	debug.RegisterProxyEndpointServiceServer(grpcServer, p)
}
