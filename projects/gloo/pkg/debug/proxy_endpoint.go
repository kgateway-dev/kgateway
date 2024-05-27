package debug

import (
	"context"

	"github.com/solo-io/solo-kit/pkg/errors"

	"github.com/rotisserie/eris"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/debug"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
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

	// RegisterProxyReader registers a given ProxyReader for a particular source
	// This is used by the ControlPlane to register the readers that will provide access to the proxies
	RegisterProxyReader(source ProxySource, client v1.ProxyReader)
}

type proxyEndpointServer struct {
	// readersBySource contains the set of ProxyReaders that have been registered for the server
	readersBySource map[ProxySource]v1.ProxyReader
}

// NewProxyEndpointServer returns an implementation of the ProxyEndpointServer
func NewProxyEndpointServer() ProxyEndpointServer {
	return &proxyEndpointServer{
		readersBySource: make(map[ProxySource]v1.ProxyReader, 1),
	}
}

func (p *proxyEndpointServer) Register(grpcServer *grpc.Server) {
	debug.RegisterProxyEndpointServiceServer(grpcServer, p)
}

func (p *proxyEndpointServer) RegisterProxyReader(source ProxySource, proxyReader v1.ProxyReader) {
	p.readersBySource[source] = proxyReader
}

// GetProxies returns the list of Proxies that match the criteria of a given ProxyEndpointRequest
func (p *proxyEndpointServer) GetProxies(ctx context.Context, req *debug.ProxyEndpointRequest) (*debug.ProxyEndpointResponse, error) {
	contextutils.LoggerFrom(ctx).Debugf("received grpc request to read proxies")

	if req.GetName() != "" {
		proxy, err := p.getOne(ctx, req.GetNamespace(), req.GetName(), req.GetSource())
		return &debug.ProxyEndpointResponse{
			Proxies: v1.ProxyList{proxy},
		}, err
	}

	proxyList, err := p.getMany(ctx, req.GetNamespace(), req.GetSelector(), req.GetSource())
	return &debug.ProxyEndpointResponse{
		Proxies: proxyList,
	}, err
}

func (p *proxyEndpointServer) getOne(ctx context.Context, namespace, name, source string) (*v1.Proxy, error) {
	proxyReaders, err := p.getProxyReadersForSource(source)
	if err != nil {
		return nil, err
	}

	for _, reader := range proxyReaders {
		proxy, readErr := reader.Read(namespace, name, clients.ReadOpts{Ctx: ctx})
		if readErr == nil {
			return proxy, nil
		}

		if errors.IsNotExist(readErr) {
			// continue, this just means that one of the readers didn't have a proxy with that name
		} else {
			// this was an unexpected error, return to the user
			return nil, readErr

		}
	}

	return nil, errors.NewNotExistErr(namespace, name)
}

func (p *proxyEndpointServer) getMany(ctx context.Context, namespace string, selector map[string]string, source string) (v1.ProxyList, error) {
	proxyReaders, err := p.getProxyReadersForSource(source)
	if err != nil {
		return nil, err
	}

	var proxyList v1.ProxyList
	for _, reader := range proxyReaders {
		readerList, listErr := reader.List(namespace, clients.ListOpts{
			Ctx:      ctx,
			Selector: selector,
		})
		if listErr == nil {
			proxyList = append(proxyList, readerList...)
		}
	}

	// we treat an getMany request that returned no proxies as valid (no error), just with no proxies
	return proxyList, nil
}

func (p *proxyEndpointServer) getProxyReadersForSource(source string) ([]v1.ProxyReader, error) {
	var proxyReaders []v1.ProxyReader

	if source == "" {
		// If the source is empty, try all ProxyReaders
		for _, reader := range p.readersBySource {
			proxyReaders = append(proxyReaders, reader)
		}
		return proxyReaders, nil
	}

	// If the source is provided, validate that it is an available one
	requestProxySource, ok := proxySourceByName[source]
	if !ok {
		return nil, eris.Errorf("ProxyEndpointRequest.source (%s) is not a valid option. Available options are: %v", source, proxySourceByName)
	}

	proxyReader, ok := p.readersBySource[requestProxySource]
	if !ok {
		// This should not really occur. If this does, it likely means that a developer forgot to write the code
		// to register a given proxySource with the ProxyEndpointServer
		return nil, eris.Errorf("ProxyEndpointRequest.source (%s) does not have a registered reader", source)
	}

	return []v1.ProxyReader{proxyReader}, nil
}
