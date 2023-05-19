package debug_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	debug_api "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/debug"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/debug"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Proxy Debug Endpoint", func() {
	var (
		proxyClient         v1.ProxyClient
		ctx                 context.Context
		proxyEndpointServer debug.ProxyEndpointServer
		ns                  string
	)
	BeforeEach(func() {
		ctx = context.Background()
		resourceClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}

		proxyClient, _ = v1.NewProxyClient(ctx, resourceClientFactory)
		proxyEndpointServer = debug.NewProxyEndpointServer()
		proxyEndpointServer.SetProxyClient(proxyClient)
		ns = "some-namespace"
		proxy1 := &v1.Proxy{
			Metadata: &core.Metadata{
				Namespace: ns,
				Name:      "proxy1",
			},
		}
		proxy2 := &v1.Proxy{
			Metadata: &core.Metadata{
				Namespace: ns,
				Name:      "proxy2",
			},
		}
		proxyClient.Write(proxy1, clients.WriteOpts{Ctx: ctx})
		proxyClient.Write(proxy2, clients.WriteOpts{Ctx: ctx})
	})
	It("Returns proxies by name", func() {

		req := &debug_api.ProxyEndpointRequest{
			Name:      "proxy1",
			Namespace: ns,
		}
		resp, err := proxyEndpointServer.GetProxies(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		proxyList := resp.GetProxies()
		Expect(len(proxyList)).To(Equal(1))
		Expect(proxyList[0].GetMetadata().GetName()).To(Equal("proxy1"))
	})
	It("Returns all proxies from the provided namespace", func() {
		ns2 := "other namespace"
		additionalProxy := &v1.Proxy{
			Metadata: &core.Metadata{
				Namespace: ns2,
				Name:      "proxy3",
			},
		}
		proxyClient.Write(additionalProxy, clients.WriteOpts{Ctx: ctx})
		req := &debug_api.ProxyEndpointRequest{
			Namespace: ns,
		}
		resp, err := proxyEndpointServer.GetProxies(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		proxyList := resp.GetProxies()
		Expect(len(proxyList)).To(Equal(2))
	})
	It("Can return proxies from all namespaces", func() {
		ns2 := "other namespace"
		additionalProxy := &v1.Proxy{
			Metadata: &core.Metadata{
				Namespace: ns2,
				Name:      "proxy3",
			},
		}
		proxyClient.Write(additionalProxy, clients.WriteOpts{Ctx: ctx})
		req := &debug_api.ProxyEndpointRequest{}
		resp, err := proxyEndpointServer.GetProxies(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		proxyList := resp.GetProxies()
		Expect(len(proxyList)).To(Equal(3))
	})
})
