package azure_test

import (
	"context"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/azure"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	azureplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/azure"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Plugin", func() {
	var (
		p            plugins.Plugin
		namespace    string
		initParams   plugins.InitParams
		params       plugins.Params
		upstream     *v1.Upstream
		upstreamSpec *azure.UpstreamSpec
		out          *envoy_config_cluster_v3.Cluster
	)

	BeforeEach(func() {
		p = azureplugin.NewPlugin()

		namespace = ""
		initParams = plugins.InitParams{
			Ctx: context.TODO(),
		}
		params = plugins.Params{}

		upstreamSpec = &azure.UpstreamSpec{
			FunctionAppName: "app-name",
		}
		upstream = &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      "us",
				Namespace: namespace,
			},
			UpstreamType: &v1.Upstream_Azure{
				Azure: upstreamSpec,
			},
		}

		out = &envoy_config_cluster_v3.Cluster{}
	})

	JustBeforeEach(func() {
		p.Init(initParams)
	})

	Context("with valid upstream spec", func() {

		var err error

		JustBeforeEach(func() {
			err = p.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
		})

		Context("with secrets", func() {

			BeforeEach(func() {
				upstreamSpec.SecretRef = &core.ResourceRef{
					Namespace: namespace,
					Name:      "azure-secret1",
				}
				params.Snapshot = &v1snap.ApiSnapshot{
					Secrets: v1.SecretList{{
						Metadata: &core.Metadata{
							Name:      "azure-secret1",
							Namespace: namespace,
						},
						Kind: &v1.Secret_Azure{
							Azure: &v1.AzureSecret{
								ApiKeys: map[string]string{
									"_master": "key1",
									"foo":     "key1",
									"bar":     "key2",
								},
							},
						},
					}},
				}
			})

			It("should not error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("should have the correct output", func() {
				Expect(out.LoadAssignment.Endpoints).Should(HaveLen(1))

				tlsContext := getClusterTlsContext(out)
				Expect(tlsContext.Sni).To(Equal("app-name.azurewebsites.net"))
				Expect(out.GetType()).To(Equal(envoy_config_cluster_v3.Cluster_LOGICAL_DNS))
				Expect(out.DnsLookupFamily).To(Equal(envoy_config_cluster_v3.Cluster_V4_ONLY))
			})

		})

		Context("without secrets", func() {

			It("should not error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("should have the correct output", func() {
				Expect(out.LoadAssignment.Endpoints).Should(HaveLen(1))
				tlsContext := getClusterTlsContext(out)

				Expect(tlsContext.Sni).To(Equal("app-name.azurewebsites.net"))
				Expect(out.GetType()).To(Equal(envoy_config_cluster_v3.Cluster_LOGICAL_DNS))
				Expect(out.DnsLookupFamily).To(Equal(envoy_config_cluster_v3.Cluster_V4_ONLY))
			})
		})

		Context("with ssl", func() {

			Context("should allow configuring ssl without settings.UpstreamOptions", func() {

				BeforeEach(func() {
					initParams.Settings = &v1.Settings{}
				})

				It("should not error", func() {
					Expect(err).NotTo(HaveOccurred())
				})

				It("should configure CommonTlsContext without TlsParams", func() {
					commonTlsContext := getClusterTlsContext(out).GetCommonTlsContext()
					Expect(commonTlsContext).NotTo(BeNil())

					tlsParams := commonTlsContext.GetTlsParams()
					Expect(tlsParams).To(BeNil())
				})

			})

			Context("should allow configuring ssl with settings.UpstreamOptions", func() {

				BeforeEach(func() {
					initParams.Settings = &v1.Settings{
						UpstreamOptions: &v1.UpstreamOptions{
							SslParameters: &ssl.SslParameters{
								MinimumProtocolVersion: ssl.SslParameters_TLSv1_1,
								MaximumProtocolVersion: ssl.SslParameters_TLSv1_2,
								CipherSuites:           []string{"cipher-test"},
								EcdhCurves:             []string{"ec-dh-test"},
							},
						},
					}
				})

				It("should not error", func() {
					Expect(err).NotTo(HaveOccurred())
				})

				It("should configure CommonTlsContext", func() {
					commonTlsContext := getClusterTlsContext(out).GetCommonTlsContext()
					Expect(commonTlsContext).NotTo(BeNil())

					tlsParams := commonTlsContext.GetTlsParams()
					Expect(tlsParams).NotTo(BeNil())

					Expect(tlsParams.GetCipherSuites()).To(Equal([]string{"cipher-test"}))
					Expect(tlsParams.GetEcdhCurves()).To(Equal([]string{"ec-dh-test"}))
					Expect(tlsParams.GetTlsMinimumProtocolVersion()).To(Equal(envoyauth.TlsParameters_TLSv1_1))
					Expect(tlsParams.GetTlsMaximumProtocolVersion()).To(Equal(envoyauth.TlsParameters_TLSv1_2))
				})

			})

			Context("should error while configuring ssl with invalid tls versions in settings.UpstreamOptions", func() {

				var invalidProtocolVersion ssl.SslParameters_ProtocolVersion = 5 // INVALID

				BeforeEach(func() {
					initParams.Settings = &v1.Settings{
						UpstreamOptions: &v1.UpstreamOptions{
							SslParameters: &ssl.SslParameters{
								MinimumProtocolVersion: invalidProtocolVersion,
								MaximumProtocolVersion: ssl.SslParameters_TLSv1_2,
								CipherSuites:           []string{"cipher-test"},
								EcdhCurves:             []string{"ec-dh-test"},
							},
						},
					}
				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
				})

			})

		})
	})
})

func getClusterTlsContext(cluster *envoy_config_cluster_v3.Cluster) *envoyauth.UpstreamTlsContext {
	return utils.MustAnyToMessage(cluster.TransportSocket.GetTypedConfig()).(*envoyauth.UpstreamTlsContext)
}
