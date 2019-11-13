package azure_test

import (
	"context"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/azure"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	azureplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/azure"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Plugin", func() {
	var (
		p      plugins.Plugin
		out    *envoyapi.Cluster
		params plugins.Params
	)

	BeforeEach(func() {
		var b bool
		p = azureplugin.NewPlugin(&b)
		p.Init(plugins.InitParams{Ctx: context.TODO()})
		out = &envoyapi.Cluster{}
		params = plugins.Params{}
	})

	Context("with valid upstream spec", func() {
		var (
			err      error
			upstream *v1.Upstream
		)

		BeforeEach(func() {
			upstream = &v1.Upstream{
				Metadata: core.Metadata{
					Name: "test",
					// TODO(yuval-k): namespace
					Namespace: "",
				},
				UpstreamType: &v1.Upstream_Azure{
					Azure: &azure.UpstreamSpec{
						FunctionAppName: "my-appwhos",
					},
				},
			}
		})
		Context("with secrets", func() {

			BeforeEach(func() {
				upstream.UpstreamType.(*v1.Upstream_Azure).Azure.SecretRef = core.ResourceRef{
					Namespace: "",
					Name:      "azure-secret1",
				}

				params.Snapshot = &v1.ApiSnapshot{
					Secrets: v1.SecretList{{
						Metadata: core.Metadata{
							Name: "azure-secret1",
							// TODO(yuval-k): namespace
							Namespace: "",
						},
						Kind: &v1.Secret_Azure{
							Azure: &v1.AzureSecret{
								ApiKeys: map[string]string{"_master": "key1", "foo": "key1", "bar": "key2"},
							},
						},
					}},
				}

				err = p.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			})
			It("should not error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("should have the correct output", func() {
				Expect(out.LoadAssignment.Endpoints).Should(HaveLen(1))
				// Expect(out.Hosts[0].GetSocketAddress().Address).To(Equal("my-appwhos.azurewebsites.net"))
				// Expect(out.Hosts[0].GetSocketAddress().PortSpecifier.(*envoycore.SocketAddress_PortValue).PortValue).To(BeEquivalentTo(443))
				Expect(out.TlsContext.Sni).To(Equal("my-appwhos.azurewebsites.net"))
				Expect(out.GetType()).To(Equal(envoyapi.Cluster_LOGICAL_DNS))
				Expect(out.DnsLookupFamily).To(Equal(envoyapi.Cluster_V4_ONLY))
			})

		})
		Context("without secrets", func() {
			BeforeEach(func() {
				err = p.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			})
			It("should not error", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("should have the correct output", func() {
				Expect(out.LoadAssignment.Endpoints).Should(HaveLen(1))
				// Expect(out.Hosts[0].GetSocketAddress().Address).To(Equal("my-appwhos.azurewebsites.net"))
				// Expect(out.Hosts[0].GetSocketAddress().PortSpecifier.(*envoycore.SocketAddress_PortValue).PortValue).To(BeEquivalentTo(443))
				Expect(out.TlsContext.Sni).To(Equal("my-appwhos.azurewebsites.net"))
				Expect(out.GetType()).To(Equal(envoyapi.Cluster_LOGICAL_DNS))
				Expect(out.DnsLookupFamily).To(Equal(envoyapi.Cluster_V4_ONLY))
			})
		})
	})
})
