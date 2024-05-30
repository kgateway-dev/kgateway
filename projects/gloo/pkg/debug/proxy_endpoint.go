package debug

import (
	"context"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/debug"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"google.golang.org/grpc"
)

// ProxyEndpointServer responds to requests for Proxies, and returns them
// The server relies on ProxyReaders being registered with the server
type ProxyEndpointServer interface {
	// ProxyEndpointServiceServer exposes the user-facing API to request proxies
	debug.ProxyEndpointServiceServer

	// Register is used by the ControlPlane to register this service with a particular grpc.Server
	Register(grpcServer *grpc.Server)

	// RegisterProxyReader registers a ProxyReader.
	// This is used by the ControlPlane to register the reader that will provide access to the proxies.
	RegisterProxyReader(client v1.ProxyReader)
}

type proxyEndpointServer struct {
	proxyReader v1.ProxyReader
}

// NewProxyEndpointServer returns an implementation of the ProxyEndpointServer
func NewProxyEndpointServer() ProxyEndpointServer {
	return &proxyEndpointServer{}
}

func (p *proxyEndpointServer) Register(grpcServer *grpc.Server) {
	debug.RegisterProxyEndpointServiceServer(grpcServer, p)
}

func (p *proxyEndpointServer) RegisterProxyReader(proxyReader v1.ProxyReader) {
	p.proxyReader = proxyReader
}

// GetProxies returns the list of Proxies that match the criteria of a given ProxyEndpointRequest
func (p *proxyEndpointServer) GetProxies(ctx context.Context, req *debug.ProxyEndpointRequest) (*debug.ProxyEndpointResponse, error) {
	contextutils.LoggerFrom(ctx).Debugf("received grpc request to read proxies")

	if req.GetName() != "" {
		proxy, err := p.getOne(ctx, req.GetNamespace(), req.GetName())
		return &debug.ProxyEndpointResponse{
			Proxies: v1.ProxyList{proxy},
		}, err
	}

	proxyList, err := p.getMany(ctx, req.GetNamespace(), req.GetSelector(), req.GetExpressionSelector())
	return &debug.ProxyEndpointResponse{
		Proxies: proxyList,
	}, err
}

func (p *proxyEndpointServer) getOne(ctx context.Context, namespace, name string) (*v1.Proxy, error) {
	if p.proxyReader == nil {
		return nil, eris.Errorf("a ProxyReader must be registered before calling the proxy endpoint")
	}

	proxy, err := p.proxyReader.Read(namespace, name, clients.ReadOpts{Ctx: ctx})
	if err != nil {
		return nil, err
	}
	return proxy, nil
}

func (p *proxyEndpointServer) getMany(ctx context.Context, namespace string, selector map[string]string, expressionSelector string) (v1.ProxyList, error) {
	if p.proxyReader == nil {
		return nil, eris.Errorf("a ProxyReader must be registered before calling the proxy endpoint")
	}

	listOpts := clients.ListOpts{
		Ctx: ctx,
	}
	if expressionSelector != "" {
		listOpts.ExpressionSelector = expressionSelector
	} else if len(selector) > 0 {
		listOpts.Selector = selector
	}
	proxyList, err := p.proxyReader.List(namespace, listOpts)
	if err != nil {
		return nil, err
	}

	return proxyList, nil
}
