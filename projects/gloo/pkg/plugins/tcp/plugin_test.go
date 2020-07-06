package tcp_test

import (
	"time"

	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tcp"
	mock_utils "github.com/solo-io/gloo/projects/gloo/pkg/utils/mocks"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/tcp"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Plugin", func() {
	var (
		in *v1.Listener

		ctrl          *gomock.Controller
		sslTranslator *mock_utils.MockSslConfigTranslator
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())

		sslTranslator = mock_utils.NewMockSslConfigTranslator(ctrl)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Context("listener filter chain plugin", func() {
		var (
			tcpListener *v1.TcpListener
			snap        *v1.ApiSnapshot
			tcps        *tcp.TcpProxySettings

			ns = "one"
			wd = []*v1.WeightedDestination{
				{
					Weight: 5,
					Destination: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: &core.ResourceRef{
								Name:      "one",
								Namespace: ns,
							},
						},
					},
				},
				{
					Weight: 1,
					Destination: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: &core.ResourceRef{
								Name:      "two",
								Namespace: ns,
							},
						},
					},
				},
			}
		)

		BeforeEach(func() {
			pd := func(t time.Duration) *time.Duration { return &t }
			snap = &v1.ApiSnapshot{
				Upstreams: v1.UpstreamList{
					{
						Metadata: core.Metadata{
							Name:      "one",
							Namespace: ns,
						},
					},
					{
						Metadata: core.Metadata{
							Name:      "two",
							Namespace: ns,
						},
					},
					{
						Metadata: core.Metadata{
							Name:      "three",
							Namespace: ns,
						},
					},
				},
			}
			tcps = &tcp.TcpProxySettings{
				MaxConnectAttempts: &types.UInt32Value{
					Value: 5,
				},
				IdleTimeout: pd(5 * time.Second),
			}
			tcpListener = &v1.TcpListener{
				TcpHosts: []*v1.TcpHost{},
				Options: &v1.TcpListenerOptions{
					TcpProxySettings: tcps,
				},
			}
			in = &v1.Listener{
				ListenerType: &v1.Listener_TcpListener{
					TcpListener: tcpListener,
				},
			}
		})

		It("can copy over tcp plugin settings", func() {
			tcpListener.TcpHosts = append(tcpListener.TcpHosts, &v1.TcpHost{
				Name: "one",
				Destination: &v1.TcpHost_TcpAction{
					Destination: &v1.TcpHost_TcpAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: &core.ResourceRef{
									Name:      "one",
									Namespace: ns,
								},
							},
						},
					},
				},
			})

			p := NewPlugin(sslTranslator)
			filterChains, err := p.ProcessListenerFilterChain(plugins.Params{Snapshot: snap}, in)
			Expect(err).NotTo(HaveOccurred())
			Expect(filterChains).To(HaveLen(1))

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseTypedConfig(filterChains[0].Filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.IdleTimeout).To(Equal(gogoutils.DurationStdToProto(tcps.IdleTimeout)))
			Expect(cfg.MaxConnectAttempts).To(Equal(gogoutils.UInt32GogoToProto(tcps.MaxConnectAttempts)))
		})

		It("can transform a single destination", func() {
			tcpListener.TcpHosts = append(tcpListener.TcpHosts, &v1.TcpHost{
				Name: "one",
				Destination: &v1.TcpHost_TcpAction{
					Destination: &v1.TcpHost_TcpAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: &core.ResourceRef{
									Name:      "one",
									Namespace: ns,
								},
							},
						},
					},
				},
			})
			p := NewPlugin(sslTranslator)
			filterChains, err := p.ProcessListenerFilterChain(plugins.Params{Snapshot: snap}, in)
			Expect(err).NotTo(HaveOccurred())
			Expect(filterChains).To(HaveLen(1))

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseTypedConfig(filterChains[0].Filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())
			cluster := cfg.GetCluster()
			Expect(cluster).To(Equal(translatorutil.UpstreamToClusterName(core.ResourceRef{Namespace: ns, Name: "one"})))
		})
		It("can transform a multi destination", func() {
			tcpListener.TcpHosts = append(tcpListener.TcpHosts, &v1.TcpHost{
				Name: "one",
				Destination: &v1.TcpHost_TcpAction{
					Destination: &v1.TcpHost_TcpAction_Multi{
						Multi: &v1.MultiDestination{
							Destinations: wd,
						},
					},
				},
			})
			p := NewPlugin(sslTranslator)
			filterChains, err := p.ProcessListenerFilterChain(plugins.Params{Snapshot: snap}, in)
			Expect(err).NotTo(HaveOccurred())
			Expect(filterChains).To(HaveLen(1))

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseTypedConfig(filterChains[0].Filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())
			clusters := cfg.GetWeightedClusters()
			Expect(clusters.Clusters).To(HaveLen(2))
			Expect(clusters.Clusters[0].Name).To(Equal(translatorutil.UpstreamToClusterName(core.ResourceRef{Namespace: ns, Name: "one"})))
			Expect(clusters.Clusters[0].Weight).To(Equal(uint32(5)))
			Expect(clusters.Clusters[1].Name).To(Equal(translatorutil.UpstreamToClusterName(core.ResourceRef{Namespace: ns, Name: "two"})))
			Expect(clusters.Clusters[1].Weight).To(Equal(uint32(1)))
		})
		It("can transform an upstream group", func() {
			snap.UpstreamGroups = append(snap.UpstreamGroups, &v1.UpstreamGroup{
				Destinations: wd,
				Metadata: core.Metadata{
					Name:      "one",
					Namespace: ns,
				},
			})
			tcpListener.TcpHosts = append(tcpListener.TcpHosts, &v1.TcpHost{
				Name: "one",
				Destination: &v1.TcpHost_TcpAction{
					Destination: &v1.TcpHost_TcpAction_UpstreamGroup{
						UpstreamGroup: &core.ResourceRef{
							Namespace: ns,
							Name:      "one",
						},
					},
				},
			})
			p := NewPlugin(sslTranslator)
			filterChains, err := p.ProcessListenerFilterChain(plugins.Params{Snapshot: snap}, in)
			Expect(err).NotTo(HaveOccurred())
			Expect(filterChains).To(HaveLen(1))

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseTypedConfig(filterChains[0].Filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())
			clusters := cfg.GetWeightedClusters()
			Expect(clusters.Clusters).To(HaveLen(2))
			Expect(clusters.Clusters[0].Name).To(Equal(translatorutil.UpstreamToClusterName(core.ResourceRef{Namespace: ns, Name: "one"})))
			Expect(clusters.Clusters[0].Weight).To(Equal(uint32(5)))
			Expect(clusters.Clusters[1].Name).To(Equal(translatorutil.UpstreamToClusterName(core.ResourceRef{Namespace: ns, Name: "two"})))
			Expect(clusters.Clusters[1].Weight).To(Equal(uint32(1)))
		})

		It("will error when forward sni is specified with no SslConfig", func() {
			tcpListener.TcpHosts = append(tcpListener.TcpHosts, &v1.TcpHost{
				Name: "one",
				Destination: &v1.TcpHost_TcpAction{
					Destination: &v1.TcpHost_TcpAction_ForwardSniClusterName{
						ForwardSniClusterName: &types.Empty{},
					},
				},
			})
			p := NewPlugin(sslTranslator)
			_, err := p.ProcessListenerFilterChain(plugins.Params{Snapshot: snap}, in)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(NoSslConfigFoundError.Error()))
		})

		It("can add the forward sni cluster name filter", func() {
			sslConfig := &v1.SslConfig{
				SslSecrets: &v1.SslConfig_SecretRef{
					SecretRef: &core.ResourceRef{
						Name:      "name",
						Namespace: "namespace",
					},
				},
				SniDomains: []string{"hello.world"},
			}
			tcpListener.TcpHosts = append(tcpListener.TcpHosts, &v1.TcpHost{
				Name: "one",
				Destination: &v1.TcpHost_TcpAction{
					Destination: &v1.TcpHost_TcpAction_ForwardSniClusterName{
						ForwardSniClusterName: &types.Empty{},
					},
				},
				SslConfig: sslConfig,
			})

			sslTranslator.EXPECT().
				ResolveDownstreamSslConfig(snap.Secrets, sslConfig).
				Return(&envoy_api_v2_auth.DownstreamTlsContext{}, nil)

			p := NewPlugin(sslTranslator)
			filterChains, err := p.ProcessListenerFilterChain(plugins.Params{Snapshot: snap}, in)
			Expect(err).NotTo(HaveOccurred())
			Expect(filterChains).To(HaveLen(1))
			Expect(filterChains[0].Filters).To(HaveLen(2))
			Expect(filterChains[0].Filters[0].Name).To(Equal(SniFilter))
			Expect(filterChains[0].Filters[0].GetConfig()).To(BeNil())
			Expect(filterChains[0].Filters[0].GetTypedConfig()).To(BeNil())

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseTypedConfig(filterChains[0].Filters[1], &cfg)
			Expect(err).NotTo(HaveOccurred())
			cluster, ok := cfg.GetClusterSpecifier().(*envoytcp.TcpProxy_Cluster)
			Expect(ok).To(BeTrue(), "must be a single cluster type")
			Expect(cluster.Cluster).To(Equal(""))
		})

		It("will the forward sni cluster name filter, and tls inspector when no sni domains defined", func() {
			sslConfig := &v1.SslConfig{
				SslSecrets: &v1.SslConfig_SecretRef{
					SecretRef: &core.ResourceRef{
						Name:      "name",
						Namespace: "namespace",
					},
				},
			}
			tcpListener.TcpHosts = append(tcpListener.TcpHosts, &v1.TcpHost{
				Name:      "one",
				SslConfig: sslConfig,
				Destination: &v1.TcpHost_TcpAction{
					Destination: &v1.TcpHost_TcpAction_ForwardSniClusterName{
						ForwardSniClusterName: &types.Empty{},
					},
				},
			})

			sslTranslator.EXPECT().
				ResolveDownstreamSslConfig(snap.Secrets, sslConfig).
				Return(&envoy_api_v2_auth.DownstreamTlsContext{}, nil)

			p := NewPlugin(sslTranslator)
			filterChains, err := p.ProcessListenerFilterChain(plugins.Params{Snapshot: snap}, in)
			Expect(err).NotTo(HaveOccurred())
			Expect(filterChains).To(HaveLen(1))
			Expect(filterChains[0].Filters).To(HaveLen(3))
			Expect(filterChains[0].Filters[0].Name).To(Equal(wellknown.TlsInspector))
			Expect(filterChains[0].Filters[0].GetConfig()).To(BeNil())
			Expect(filterChains[0].Filters[0].GetTypedConfig()).To(BeNil())
			Expect(filterChains[0].Filters[1].Name).To(Equal(SniFilter))
			Expect(filterChains[0].Filters[1].GetConfig()).To(BeNil())
			Expect(filterChains[0].Filters[1].GetTypedConfig()).To(BeNil())

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseTypedConfig(filterChains[0].Filters[2], &cfg)
			Expect(err).NotTo(HaveOccurred())
			cluster, ok := cfg.GetClusterSpecifier().(*envoytcp.TcpProxy_Cluster)
			Expect(ok).To(BeTrue(), "must be a single cluster type")
			Expect(cluster.Cluster).To(Equal(""))
		})
	})

})
