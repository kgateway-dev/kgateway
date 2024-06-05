package istio_automtls_test

import (
	"context"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/istio_automtls"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Istio Automtls Plugin", func() {

	When("automtls enabled, Istio integration is enabled", func() {

		var (
			plugin   plugins.Plugin
			upstream *gloov1.Upstream
		)

		BeforeEach(func() {
			plugin = istio_automtls.NewPlugin(false)
			plugin.Init(plugins.InitParams{
				Ctx: context.TODO(),
				Settings: &gloov1.Settings{
					Gloo: &gloov1.GlooOptions{
						IstioOptions: &gloov1.GlooOptions_IstioOptions{
							EnableAutoMtls:    &wrappers.BoolValue{Value: true},
							EnableIntegration: &wrappers.BoolValue{Value: true},
						},
					},
				},
			})

			upstream = &gloov1.Upstream{
				Metadata: &core.Metadata{
					Name:      "us",
					Namespace: "namespace",
				},
				UpstreamType: &gloov1.Upstream_Static{
					Static: &static.UpstreamSpec{
						Hosts: []*static.Host{
							{
								Addr: "localhost",
								Port: 12345,
							},
						},
					},
				},
			}
		})

		It("correctly translated auto mtls transport socket matches", func() {
			out := &envoy_config_cluster_v3.Cluster{}
			err := plugin.(plugins.UpstreamPlugin).ProcessUpstream(
				plugins.Params{},
				upstream,
				out)

			Expect(err).NotTo(HaveOccurred())
			Expect(out.TransportSocketMatches).To(HaveLen(2))
		})

		It("does not translate automtls transport socket matches if disabled in sslConfig", func() {
			upstream.DisableIstioAutoMtls = &wrappers.BoolValue{Value: true}

			out := &envoy_config_cluster_v3.Cluster{}
			err := plugin.(plugins.UpstreamPlugin).ProcessUpstream(
				plugins.Params{},
				upstream,
				out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TransportSocketMatches).To(BeNil())
		})

		It("does not translate automtls transport socket matches if sslConfig is provided", func() {
			upstream.SslConfig = &ssl.UpstreamSslConfig{
				AlpnProtocols: []string{"gloo"},
				SslSecrets: &ssl.UpstreamSslConfig_Sds{
					Sds: &ssl.SDSConfig{
						TargetUri: "fake.svc:8080",
					},
				},
			}

			out := &envoy_config_cluster_v3.Cluster{}
			err := plugin.(plugins.UpstreamPlugin).ProcessUpstream(
				plugins.Params{},
				upstream,
				out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TransportSocketMatches).To(BeNil())
		})
	})

	It("automtls enabled, istio integration is disabled, no transport socket matches are translated by plugin", func() {
		plugin := istio_automtls.NewPlugin(false)
		plugin.Init(plugins.InitParams{
			Ctx: context.TODO(),
			Settings: &gloov1.Settings{
				Gloo: &gloov1.GlooOptions{
					IstioOptions: &gloov1.GlooOptions_IstioOptions{
						EnableAutoMtls:    &wrappers.BoolValue{Value: true},
						EnableIntegration: &wrappers.BoolValue{Value: false},
					},
				},
			},
		})

		out := &envoy_config_cluster_v3.Cluster{}
		err := plugin.ProcessUpstream(
			plugins.Params{},
			&gloov1.Upstream{
				Metadata: &core.Metadata{
					Name:      "us",
					Namespace: "namespace",
				},
				UpstreamType: &gloov1.Upstream_Static{
					Static: &static.UpstreamSpec{
						Hosts: []*static.Host{
							{
								Addr: "localhost",
								Port: 12345,
							},
						},
					},
				},
			},
			out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.TransportSocketMatches).To(BeNil())
	})

	It("automtls disabled, istio integration is enabled, no transport socket matches are translated by plugin", func() {
		plugin := istio_automtls.NewPlugin(false)
		plugin.Init(plugins.InitParams{
			Ctx: context.TODO(),
			Settings: &gloov1.Settings{
				Gloo: &gloov1.GlooOptions{
					IstioOptions: &gloov1.GlooOptions_IstioOptions{
						EnableAutoMtls:    &wrappers.BoolValue{Value: false},
						EnableIntegration: &wrappers.BoolValue{Value: true},
					},
				},
			},
		})

		out := &envoy_config_cluster_v3.Cluster{}
		err := plugin.ProcessUpstream(
			plugins.Params{},
			&gloov1.Upstream{
				Metadata: &core.Metadata{
					Name:      "us",
					Namespace: "namespace",
				},
				UpstreamType: &gloov1.Upstream_Static{
					Static: &static.UpstreamSpec{
						Hosts: []*static.Host{
							{
								Addr: "localhost",
								Port: 12345,
							},
						},
					},
				},
			},
			out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.TransportSocketMatches).To(BeNil())
	})

	It("automtls disabled, istio integration is disabled, no transport socket matches are translated by plugin", func() {
		plugin := istio_automtls.NewPlugin(false)
		plugin.Init(plugins.InitParams{
			Ctx: context.TODO(),
			Settings: &gloov1.Settings{
				Gloo: &gloov1.GlooOptions{
					IstioOptions: &gloov1.GlooOptions_IstioOptions{
						EnableAutoMtls:    &wrappers.BoolValue{Value: false},
						EnableIntegration: &wrappers.BoolValue{Value: true},
					},
				},
			},
		})

		out := &envoy_config_cluster_v3.Cluster{}
		err := plugin.ProcessUpstream(
			plugins.Params{},
			&gloov1.Upstream{
				Metadata: &core.Metadata{
					Name:      "us",
					Namespace: "namespace",
				},
				UpstreamType: &gloov1.Upstream_Static{
					Static: &static.UpstreamSpec{
						Hosts: []*static.Host{
							{
								Addr: "localhost",
								Port: 12345,
							},
						},
					},
				},
			},
			out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.TransportSocketMatches).To(BeNil())
	})
})
