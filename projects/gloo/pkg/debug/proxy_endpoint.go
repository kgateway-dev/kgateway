package debug

import (
	"context"
	"github.com/rotisserie/eris"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/debug"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"google.golang.org/grpc"
)

// ProxySource represents the type of translator that produced the Proxy resource
type ProxySource int

const (
	EdgeGatewayTranslation ProxySource = iota // Fault injection // First Filter Stage
	K8sGatewayTranslation                     // Cors stag
)

// Enum value maps for Gzip_CompressionLevel_Enum.
var (
	ProxySource_name = map[int32]string{
		0: "edge-gw",
		1: "k8s-gw",
	}
	ProxySource_name_value = map[string]int32{
		"edge-gw": 0,
		"k8s-gw":  1,
	}
)

type ProxyEndpointServer interface {
	debug.ProxyEndpointServiceServer

	Register(grpcServer *grpc.Server)
	RegisterProxyReader(source ProxySource, client ProxyReader)
}

type ProxyReader interface {
	Read(namespace, name string, opts clients.ReadOpts) (*v1.Proxy, error)
	List(namespace string, opts clients.ListOpts) (v1.ProxyList, error)
}

type proxyEndpointServer struct {
	proxyReader ProxyReader

	readersBySource map[ProxySource]ProxyReader
}

func NewProxyEndpointServer() *proxyEndpointServer {
	return &proxyEndpointServer{
		readersBySource: make(map[ProxySource]ProxyReader, 1),
	}
}

func (p *proxyEndpointServer) RegisterProxyReader(source ProxySource, proxyReader ProxyReader) {
	p.readersBySource[source] = proxyReader
}

func (p *proxyEndpointServer) SetProxyReader(proxyReader ProxyReader) {
	p.proxyReader = proxyReader
}

// GetProxies receives a request from outside the gloo pod and returns a filtered list of proxies in a format that mirrors the k8s client
func (p *proxyEndpointServer) GetProxies(ctx context.Context, req *debug.ProxyEndpointRequest) (*debug.ProxyEndpointResponse, error) {
	contextutils.LoggerFrom(ctx).Infof("received grpc request to read proxies")

	proxySource := K8sGatewayTranslation

	//proxyReader, ok := p.readersBySource[proxySource]
	if !true {
		return nil, eris.Errorf("Proxy Source (%s) does not have a reader registered", proxySource)
	}

	var (
		proxyList v1.ProxyList
		err       error
	)
	if req.GetName() != "" {
		proxyList, err = p.GetProxiesByRef(ctx, req.GetNamespace(), req.GetName())
	} else {
		proxyList, err = p.GetProxiesBySelector(ctx, req.GetNamespace(), req.GetSelector())
	}

	return &debug.ProxyEndpointResponse{
		Proxies: proxyList,
	}, err

}

func (p *proxyEndpointServer) GetProxiesByRef(ctx context.Context, namespace string, name string) (v1.ProxyList, error) {
	proxy, err := p.proxyReader.Read(namespace, name, clients.ReadOpts{Ctx: ctx})
	return v1.ProxyList{proxy}, err
}

func (p *proxyEndpointServer) GetProxiesBySelector(ctx context.Context, namespace string, selector map[string]string) (v1.ProxyList, error) {
	proxies, err := p.proxyReader.List(namespace, clients.ListOpts{
		Ctx:      ctx,
		Selector: selector,
	})
	return proxies, err
}

func (p *proxyEndpointServer) Register(grpcServer *grpc.Server) {
	debug.RegisterProxyEndpointServiceServer(grpcServer, p)
}
