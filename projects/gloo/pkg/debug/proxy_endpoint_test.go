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
		ctx context.Context

		edgeGatewayProxyClient v1.ProxyClient
		k8sGatewayProxyClient  v1.ProxyClient
		proxyEndpointServer    debug.ProxyEndpointServer
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		edgeGatewayProxyClient, err = v1.NewProxyClient(ctx, &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		})
		Expect(err).NotTo(HaveOccurred())
		k8sGatewayProxyClient, err = v1.NewProxyClient(ctx, &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		})
		Expect(err).NotTo(HaveOccurred())

		proxyEndpointServer = debug.NewProxyEndpointServer()
		proxyEndpointServer.RegisterProxyReader(debug.EdgeGatewayTranslation, edgeGatewayProxyClient)
		proxyEndpointServer.RegisterProxyReader(debug.K8sGatewayTranslation, k8sGatewayProxyClient)
	})

	Context("Request returns the appropriate error", func() {

		It("returns error when req.Source is invalid", func() {
			req := &debug_api.ProxyEndpointRequest{
				Source: "invalid-source",
			}
			_, err := proxyEndpointServer.GetProxies(ctx, req)
			Expect(err).To(MatchError(ContainSubstring("ProxyEndpointRequest.source (invalid-source) is not a valid option")))
		})

	})

	Context("Request returns the appropriate value", func() {

		BeforeEach(func() {
			proxies := v1.ProxyList{
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "proxy1",
						Namespace: "custom-namespace",
					},
				},
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "proxy2",
						Namespace: "custom-namespace",
					},
				},
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "proxy3",
						Namespace: "other-namespace",
					},
				},
			}

			for _, proxy := range proxies {
				_, err := edgeGatewayProxyClient.Write(proxy, clients.WriteOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("returns proxy by name", func() {
			edgeProxyResponse, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "proxy1",
				Namespace: "custom-namespace",
				Source:    "edge-gw",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(edgeProxyResponse.GetProxies()).To(HaveLen(1), "There should be a single edge gateway proxy")
			Expect(edgeProxyResponse.GetProxies()[0].GetMetadata().GetName()).To(Equal("proxy1"))

			_, err = proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "proxy1",
				Namespace: "custom-namespace",
				Source:    "k8s-gw",
			})
			Expect(err).To(MatchError(ContainSubstring("namespace.proxy1 does not exist")), "There should not be any k8s gateway proxies")
		})

		It("Returns all proxies from the provided namespace", func() {
			resp, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Namespace: "custom-namespace",
				// We do not include the source, to demonstrate that it will fallback to the edge gateway source
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.GetProxies()).To(HaveLen(2))
		})

		It("Returns all proxies from all namespaces", func() {
			resp, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Namespace: "",
				// We do not include the source, to demonstrate that it will fallback to the edge gateway source
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.GetProxies()).To(HaveLen(3))
		})

	})
})
