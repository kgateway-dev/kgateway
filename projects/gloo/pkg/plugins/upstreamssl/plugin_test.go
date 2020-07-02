package upstreamssl_test

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/upstreamssl"
)

var _ = Describe("Plugin", func() {

	var (
		params   plugins.Params
		plugin   *Plugin
		upstream *v1.Upstream
		tlsConf  *v1.TlsSecret
		out      *envoyapi.Cluster
	)
	BeforeEach(func() {
		out = new(envoyapi.Cluster)

		tlsConf = &v1.TlsSecret{}
		params = plugins.Params{
			Snapshot: &v1.ApiSnapshot{
				Secrets: v1.SecretList{{
					Metadata: core.Metadata{
						Name:      "name",
						Namespace: "namespace",
					},
					Kind: &v1.Secret_Tls{
						Tls: tlsConf,
					},
				}},
			},
		}
		ref := params.Snapshot.Secrets[0].Metadata.Ref()

		upstream = &v1.Upstream{
			SslConfig: &v1.UpstreamSslConfig{
				SslSecrets: &v1.UpstreamSslConfig_SecretRef{
					SecretRef: &ref,
				},
			},
		}
		plugin = NewPlugin()
	})

	tlsContext := func() *envoyauth.UpstreamTlsContext {
		return utils.MustAnyToMessage(out.TransportSocket.GetTypedConfig()).(*envoyauth.UpstreamTlsContext)
	}
	It("should process an upstream with tls config", func() {
		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(tlsContext()).ToNot(BeNil())
	})

	It("should process an upstream with tls config", func() {

		tlsConf.PrivateKey = "private"
		tlsConf.CertChain = "certchain"

		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(tlsContext()).ToNot(BeNil())
		Expect(tlsContext().CommonTlsContext.TlsCertificates[0].PrivateKey.GetInlineString()).To(Equal("private"))
		Expect(tlsContext().CommonTlsContext.TlsCertificates[0].CertificateChain.GetInlineString()).To(Equal("certchain"))
	})

	It("should process an upstream with rootca", func() {
		tlsConf.RootCa = "rootca"

		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(tlsContext()).ToNot(BeNil())
		Expect(tlsContext().CommonTlsContext.GetValidationContext().TrustedCa.GetInlineString()).To(Equal("rootca"))
	})

	Context("failure", func() {

		It("should fail with only private key", func() {

			tlsConf.PrivateKey = "private"

			err := plugin.ProcessUpstream(params, upstream, out)
			Expect(err).To(HaveOccurred())
		})
		It("should fail with only cert chain", func() {

			tlsConf.CertChain = "certchain"

			err := plugin.ProcessUpstream(params, upstream, out)
			Expect(err).To(HaveOccurred())
		})
	})
})
