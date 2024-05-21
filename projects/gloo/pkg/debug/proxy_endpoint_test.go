package debug_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	debug_api "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/debug"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/debug"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Proxy Debug Endpoint", func() {

	var (
		ctx context.Context

		proxyClient         v1.ProxyClient
		proxyEndpointServer debug.ProxyEndpointServer
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		proxyClient, err = v1.NewProxyClient(ctx, &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		})
		Expect(err).NotTo(HaveOccurred())

		proxyEndpointServer = debug.NewProxyEndpointServer()
		proxyEndpointServer.RegisterProxyReader(proxyClient)
	})

	Context("GetProxies returns the appropriate value", func() {

		BeforeEach(func() {
			edgeGatewayProxies := v1.ProxyList{
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "edge-proxy-1",
						Namespace: "east",
						Labels: map[string]string{
							utils.ProxyTypeKey: utils.GlooEdgeProxyValue,
							"another":          "label1",
						},
					},
				},
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "edge-proxy-2",
						Namespace: "east",
						Labels: map[string]string{
							utils.ProxyTypeKey: utils.GlooEdgeProxyValue,
							"another":          "label2",
						},
					},
				},
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "edge-proxy-3",
						Namespace: "west",
						Labels: map[string]string{
							utils.ProxyTypeKey: utils.GlooEdgeProxyValue,
							"another":          "label3",
						},
					},
				},
			}

			k8sGatewayProxies := v1.ProxyList{
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "k8s-proxy-1",
						Namespace: "east",
						Labels: map[string]string{
							utils.ProxyTypeKey: utils.GatewayApiProxyValue,
							"another":          "label1",
						},
					},
				},
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "k8s-proxy-2",
						Namespace: "east",
						Labels: map[string]string{
							utils.ProxyTypeKey: utils.GatewayApiProxyValue,
							"another":          "label2",
						},
					},
				},
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "k8s-proxy-3",
						Namespace: "west",
						Labels: map[string]string{
							utils.ProxyTypeKey: utils.GatewayApiProxyValue,
							"another":          "label3",
						},
					},
				},
			}

			otherProxies := v1.ProxyList{
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "knative-proxy-1",
						Namespace: "east",
						Labels: map[string]string{
							utils.ProxyTypeKey: utils.KnativeProxyValue,
							"another":          "label1",
						},
					},
				},
				&v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "ingress-proxy-2",
						Namespace: "west",
						Labels: map[string]string{
							utils.ProxyTypeKey: utils.IngressProxyValue,
							"another":          "label2",
						},
					},
				},
			}

			for _, proxy := range edgeGatewayProxies {
				_, err := proxyClient.Write(proxy, clients.WriteOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())
			}
			for _, proxy := range k8sGatewayProxies {
				_, err := proxyClient.Write(proxy, clients.WriteOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())
			}
			for _, proxy := range otherProxies {
				_, err := proxyClient.Write(proxy, clients.WriteOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("returns all proxies when no args are provided", func() {
			req := &debug_api.ProxyEndpointRequest{}
			resp, err := proxyEndpointServer.GetProxies(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			proxies := resp.GetProxies()
			Expect(proxies).To(HaveLen(8))
			// the proxy client returns the proxies sorted first by namespace, then by name
			expectedProxyNames := []string{
				// ns=east
				"edge-proxy-1", "edge-proxy-2", "k8s-proxy-1", "k8s-proxy-2", "knative-proxy-1",
				// ns=west
				"edge-proxy-3", "ingress-proxy-2", "k8s-proxy-3",
			}
			for i, name := range expectedProxyNames {
				Expect(proxies[i].GetMetadata().GetName()).To(Equal(name))
			}
		})

		It("returns proxy by name", func() {
			resp, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "edge-proxy-1",
				Namespace: "east",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.GetProxies()).To(HaveLen(1), "There should be a single edge gateway proxy")
			Expect(resp.GetProxies()[0].GetMetadata().GetName()).To(Equal("edge-proxy-1"))

			resp, err = proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "k8s-proxy-1",
				Namespace: "east",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.GetProxies()).To(HaveLen(1), "There should be a single k8s gateway proxy")
			Expect(resp.GetProxies()[0].GetMetadata().GetName()).To(Equal("k8s-proxy-1"))
		})

		It("returns error if name is provided with no namespace", func() {
			_, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name: "k8s-proxy-1",
			})
			Expect(err).To(MatchError(ContainSubstring("k8s-proxy-1 does not exist")))
		})

		It("returns error if name is not found", func() {
			_, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "edge-proxy-1-invalid",
				Namespace: "east",
			})
			Expect(err).To(MatchError(ContainSubstring("east.edge-proxy-1-invalid does not exist")))
		})

		It("when name is provided, ignores selectors", func() {
			resp, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:               "edge-proxy-1",
				Namespace:          "east",
				Selector:           map[string]string{"invalid": "label"},
				ExpressionSelector: "invalid expression",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.GetProxies()).To(HaveLen(1), "There should be a single edge gateway proxy")
			Expect(resp.GetProxies()[0].GetMetadata().GetName()).To(Equal("edge-proxy-1"))
		})

		It("returns all proxies from the provided namespace", func() {
			resp, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Name:      "", // name is empty so that we list all resources in the provided namespace
				Namespace: "east",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.GetProxies()).To(HaveLen(5))
			// the proxy client returns the proxies sorted by name
			expectedProxyNames := []string{
				"edge-proxy-1", "edge-proxy-2", "k8s-proxy-1", "k8s-proxy-2", "knative-proxy-1",
			}
			for i, name := range expectedProxyNames {
				Expect(resp.GetProxies()[i].GetMetadata().GetName()).To(Equal(name))
			}
		})

		It("returns all proxies matching the given selector, in all namespaces", func() {
			resp, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Selector: map[string]string{
					utils.ProxyTypeKey: utils.GlooEdgeProxyValue,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.GetProxies()).To(HaveLen(3))
			expectedProxyNames := []string{
				"edge-proxy-1", "edge-proxy-2", "edge-proxy-3",
			}
			for i, name := range expectedProxyNames {
				Expect(resp.GetProxies()[i].GetMetadata().GetName()).To(Equal(name))
			}
		})

		It("returns all proxies matching the given selector, in the given namespace", func() {
			resp, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Namespace: "west",
				Selector: map[string]string{
					utils.ProxyTypeKey: utils.GatewayApiProxyValue,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.GetProxies()).To(HaveLen(1))
			Expect(resp.GetProxies()[0].GetMetadata().GetName()).To(Equal("k8s-proxy-3"))
		})

		It("returns all proxies matching the given expression selector, in all namespaces", func() {
			resp, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				ExpressionSelector: utils.GetTranslatorSelectorExpression(utils.KnativeProxyValue, utils.GlooEdgeProxyValue),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.GetProxies()).To(HaveLen(4))
			expectedProxyNames := []string{
				// ns=east
				"edge-proxy-1", "edge-proxy-2", "knative-proxy-1",
				// ns=west
				"edge-proxy-3",
			}
			for i, name := range expectedProxyNames {
				Expect(resp.GetProxies()[i].GetMetadata().GetName()).To(Equal(name))
			}
		})

		It("returns all proxies matching the given expression selector, in the given namespace", func() {
			resp, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Namespace:          "west",
				ExpressionSelector: utils.GetTranslatorSelectorExpression(utils.KnativeProxyValue, utils.GlooEdgeProxyValue),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.GetProxies()).To(HaveLen(1))
			Expect(resp.GetProxies()[0].GetMetadata().GetName()).To(Equal("edge-proxy-3"))
		})

		It("ignores selector when expressionSelector is provided", func() {
			resp, err := proxyEndpointServer.GetProxies(ctx, &debug_api.ProxyEndpointRequest{
				Namespace:          "west",
				Selector:           map[string]string{"invalid": "label"},
				ExpressionSelector: utils.GetTranslatorSelectorExpression(utils.KnativeProxyValue, utils.GlooEdgeProxyValue),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.GetProxies()).To(HaveLen(1))
			Expect(resp.GetProxies()[0].GetMetadata().GetName()).To(Equal("edge-proxy-3"))
		})

	})

})
