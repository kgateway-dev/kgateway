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

	Context("GetProxies returns the appropriate value", func() {

		BeforeEach(func() {
			edgeGatewayProxies := v1.ProxyList{
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "edge-proxy-1",
						Namespace: "east",
					},
				},
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "edge-proxy-2",
						Namespace: "east",
					},
				},
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "edge-proxy-3",
						Namespace: "west",
					},
				},
			}

			k8sGatewayProxies := v1.ProxyList{
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "k8s-proxy-1",
						Namespace: "east",
					},
				},
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "k8s-proxy-2",
						Namespace: "east",
					},
				},
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "k8s-proxy-3",
						Namespace: "west",
					},
				},
			}

			for _, proxy := range edgeGatewayProxies {
				_, err := edgeGatewayProxyClient.Write(proxy, clients.WriteOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())
			}
			for _, proxy := range k8sGatewayProxies {
				_, err := k8sGatewayProxyClient.Write(proxy, clients.WriteOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("returns error when req.Source is invalid", func() {
			req := &debug_api.ProxyEndpointRequest{
				Source: "invalid-source",
			}
			_, err := proxyEndpointServer.GetProxies(ctx, req)
			Expect(err).To(MatchError(ContainSubstring("ProxyEndpointRequest.source (invalid-source) is not a valid option")))
		})

		It("returns proxy by name, with source", func() {
			edgeProxyResponse, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "edge-proxy-1",
				Namespace: "east",
				Source:    debug.EdgeGatewaySourceName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(edgeProxyResponse.GetProxies()).To(HaveLen(1), "There should be a single edge gateway proxy")
			Expect(edgeProxyResponse.GetProxies()[0].GetMetadata().GetName()).To(Equal("edge-proxy-1"))

			k8sProxyResponse, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "k8s-proxy-1",
				Namespace: "east",
				Source:    debug.K8sGatewaySourceName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sProxyResponse.GetProxies()).To(HaveLen(1), "There should be a single k8s gateway proxy")
			Expect(k8sProxyResponse.GetProxies()[0].GetMetadata().GetName()).To(Equal("k8s-proxy-1"))
		})

		It("returns error if name not found, with source", func() {
			_, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "edge-proxy-1-invalid",
				Namespace: "east",
				Source:    debug.EdgeGatewaySourceName,
			})
			Expect(err).To(MatchError(ContainSubstring("east.edge-proxy-1-invalid does not exist")))

			_, err = proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "k8s-proxy-1-invalid",
				Namespace: "east",
				Source:    debug.K8sGatewaySourceName,
			})
			Expect(err).To(MatchError(ContainSubstring("east.k8s-proxy-1-invalid does not exist")))
		})

		It("returns proxy by name, without source", func() {
			response, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "edge-proxy-1",
				Namespace: "east",
				Source:    "",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(response.GetProxies()).To(HaveLen(1), "There should be a single edge gateway proxy")
			Expect(response.GetProxies()[0].GetMetadata().GetName()).To(Equal("edge-proxy-1"))
		})

		It("returns all proxies from the provided namespace, with source", func() {
			edgeProxyResponse, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "", // name is empty so that we list all resources in the provided namespace
				Namespace: "east",
				Source:    debug.EdgeGatewaySourceName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(edgeProxyResponse.GetProxies()).To(HaveLen(2))

			k8sProxyResponse, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "", // name is empty so that we list all resources in the provided namespace
				Namespace: "west",
				Source:    debug.K8sGatewaySourceName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sProxyResponse.GetProxies()).To(HaveLen(1))
		})

		It("returns all proxies from the provided namespace, without source", func() {
			response, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "", // name is empty so that we list all resources in the provided namespace
				Namespace: "east",
				Source:    "",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(response.GetProxies()).To(HaveLen(4), "2 edge gateway proxies, 2 kubernetes gateway proxies")
		})

		It("returns all proxies from all namespaces, with source", func() {
			edgeProxyResponse, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "", // name is empty so that we list all resources
				Namespace: "",
				Source:    debug.EdgeGatewaySourceName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(edgeProxyResponse.GetProxies()).To(HaveLen(3))

			k8sProxyResponse, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "", // name is empty so that we list all resources
				Namespace: "",
				Source:    debug.K8sGatewaySourceName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sProxyResponse.GetProxies()).To(HaveLen(3))
		})

		It("returns all proxies from the provided namespace, without source", func() {
			response, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "", // name is empty so that we list all resources in the provided namespace
				Namespace: "",
				Source:    "",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(response.GetProxies()).To(HaveLen(6), "3 edge gateway proxies, 3 kubernetes gateway proxies")
		})

	})

})
