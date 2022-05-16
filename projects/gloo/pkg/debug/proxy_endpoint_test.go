package debug

import (
	"context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/debug"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Proxy Debug Endpoint", func() {
	var (
		proxyClient         v1.ProxyClient
		ctx                 context.Context
		proxyEndpointServer ProxyEndpointServer
		ns                  string
	)
	BeforeEach(func() {
		ctx = context.Background()
		resourceClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}

		proxyClient, _ = v1.NewProxyClient(ctx, resourceClientFactory)
		proxyEndpointServer = NewProxyEndpointServer()
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

		req := &debug.ProxyEndpointRequest{
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
		req := &debug.ProxyEndpointRequest{
			Namespace: ns,
		}
		resp, err := proxyEndpointServer.GetProxies(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		proxyList := resp.GetProxies()
		Expect(len(proxyList)).To(Equal(2))
	})
})
