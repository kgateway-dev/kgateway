package debug

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/debug"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"google.golang.org/grpc"
)

type ProxyEndpointServer interface {
	debug.ProxyEndpointServiceServer
	Register(grpcServer *grpc.Server)
}
type proxyEndpointServer struct {
	proxyClient v1.ProxyClient
}

func NewProxyEndpointServer(proxyClient v1.ProxyClient) *proxyEndpointServer {
	return &proxyEndpointServer{
		proxyClient: proxyClient,
	}
}
func (p *proxyEndpointServer) GetProxies(ctx context.Context, req *debug.ProxyEndpointRequest) (*debug.ProxyEndpointResponse, error) {
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
