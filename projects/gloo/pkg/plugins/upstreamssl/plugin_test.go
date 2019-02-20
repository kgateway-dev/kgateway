package upstreamssl_test

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/upstreamssl"
)

var _ = Describe("Plugin", func() {
	It("should process an upstream with tls config", func() {
		p := NewPlugin()
		params := plugins.Params{
			Snapshot: &v1.ApiSnapshot{
				Secrets: v1.SecretsByNamespace{
					"namespace": v1.SecretList{{
						Metadata: core.Metadata{
							Name:      "name",
							Namespace: "namespace",
						},
						Kind: &v1.Secret_Tls{
							Tls: &v1.TlsSecret{},
						},
					}},
				},
			},
		}
		ref := params.Snapshot.Secrets["namespace"][0].Metadata.Ref()

		upstream := &v1.Upstream{
			UpstreamSpec: &v1.UpstreamSpec{
				SslConfig: &v1.UpstreamSslConfig{
					SslSecrets: &v1.UpstreamSslConfig_SecretRef{
						SecretRef: &ref,
					},
				},
			},
		}
		out := new(envoyapi.Cluster)

		err := p.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.TlsContext).ToNot(BeNil())
	})
})
