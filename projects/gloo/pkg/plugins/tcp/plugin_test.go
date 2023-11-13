package tcp_test

import (
	"strings"
	"time"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_extensions_filters_network_sni_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/sni_cluster/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tcp"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/tcp"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	mock_utils "github.com/solo-io/gloo/projects/gloo/pkg/utils/mocks"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	"github.com/solo-io/solo-kit/test/matchers"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var _ = Describe("Plugin", func() {
	var (
		listener    *v1.Listener
		tcpListener *v1.TcpListener

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
			snap *v1snap.ApiSnapshot
			tcps *tcp.TcpProxySettings

			ns = "one"
			wd = []*v1.WeightedDestination{
				{
					Weight: &wrappers.UInt32Value{Value: 5},
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
					Weight: &wrappers.UInt32Value{Value: 1},
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
			snap = &v1snap.ApiSnapshot{
				Upstreams: v1.UpstreamList{
					{
						Metadata: &core.Metadata{
							Name:      "one",
							Namespace: ns,
						},
					},
					{
						Metadata: &core.Metadata{
							Name:      "two",
							Namespace: ns,
						},
					},
					{
						Metadata: &core.Metadata{
							Name:      "three",
							Namespace: ns,
						},
					},
				},
			}
			tcps = &tcp.TcpProxySettings{
				MaxConnectAttempts: &wrappers.UInt32Value{
					Value: 5,
				},
				IdleTimeout: prototime.DurationToProto(5 * time.Second),
				TunnelingConfig: &tcp.TcpProxySettings_TunnelingConfig{
					Hostname: "proxyhostname",
					HeadersToAdd: []*tcp.HeaderValueOption{
						{
							Header: &tcp.HeaderValue{
								Key:   "key",
								Value: "value",
							},
							Append: &wrapperspb.BoolValue{Value: true},
						},
					},
				},
			}
			listener = &v1.Listener{}
			tcpListener = &v1.TcpListener{
				TcpHosts: []*v1.TcpHost{},
				Options: &v1.TcpListenerOptions{
					TcpProxySettings: tcps,
				},
			}
		})

		createFilterChains := func() ([]*envoy_config_listener_v3.FilterChain, error) {
			p := NewPlugin(sslTranslator)
			return p.CreateTcpFilterChains(plugins.Params{Snapshot: snap}, listener, tcpListener)
		}

		Context("can copy over tcp plugin settings", func() {

			It("works with simple settings", func() {
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

				filterChains, err := createFilterChains()
				Expect(err).NotTo(HaveOccurred())
				Expect(filterChains).To(HaveLen(1))

				var cfg envoytcp.TcpProxy
				err = translatorutil.ParseTypedConfig(filterChains[0].Filters[0], &cfg)
				Expect(err).NotTo(HaveOccurred())

				Expect(cfg.IdleTimeout).To(matchers.MatchProto(tcps.IdleTimeout))
				Expect(cfg.MaxConnectAttempts).To(matchers.MatchProto(tcps.MaxConnectAttempts))
				Expect(cfg.TunnelingConfig.GetHostname()).To(Equal(tcps.TunnelingConfig.GetHostname()))

				hta := cfg.TunnelingConfig.HeadersToAdd
				Expect(len(hta)).To(Equal(1))

				tcpHeaders := tcps.TunnelingConfig.HeadersToAdd[0]
				Expect(hta[0].Header.Key).To(Equal(tcpHeaders.Header.Key))
				Expect(hta[0].Header.Value).To(Equal(tcpHeaders.Header.Value))
				Expect(hta[0].Append.Value).To(Equal(tcpHeaders.Append.Value))

				tcps.AccessLogFlushInterval = &duration.Duration{Seconds: 5}
				filterChains, err = createFilterChains()
				Expect(err).NotTo(HaveOccurred())
				Expect(filterChains).To(HaveLen(1))

				err = translatorutil.ParseTypedConfig(filterChains[0].Filters[0], &cfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.AccessLogFlushInterval).To(matchers.MatchProto(tcps.AccessLogFlushInterval))

			})
			It("rejects invalid settings", func() {
				tcps.AccessLogFlushInterval = &duration.Duration{Nanos: 5}
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

				_, err := createFilterChains()
				Expect(err).To(HaveOccurred())
				Expect(strings.Contains(err.Error(), "access log flush interval must have minimum of 1ms")).To(BeTrue())

			})
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

			filterChains, err := createFilterChains()
			Expect(err).NotTo(HaveOccurred())
			Expect(filterChains).To(HaveLen(1))

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseTypedConfig(filterChains[0].Filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())
			cluster := cfg.GetCluster()
			Expect(cluster).To(Equal(translatorutil.UpstreamToClusterName(&core.ResourceRef{Namespace: ns, Name: "one"})))
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

			filterChains, err := createFilterChains()
			Expect(err).NotTo(HaveOccurred())
			Expect(filterChains).To(HaveLen(1))

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseTypedConfig(filterChains[0].Filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())
			clusters := cfg.GetWeightedClusters()
			Expect(clusters.Clusters).To(HaveLen(2))
			Expect(clusters.Clusters[0].Name).To(Equal(translatorutil.UpstreamToClusterName(&core.ResourceRef{Namespace: ns, Name: "one"})))
			Expect(clusters.Clusters[0].Weight).To(Equal(uint32(5)))
			Expect(clusters.Clusters[1].Name).To(Equal(translatorutil.UpstreamToClusterName(&core.ResourceRef{Namespace: ns, Name: "two"})))
			Expect(clusters.Clusters[1].Weight).To(Equal(uint32(1)))
		})

		It("can transform an upstream group", func() {
			snap.UpstreamGroups = append(snap.UpstreamGroups, &v1.UpstreamGroup{
				Destinations: wd,
				Metadata: &core.Metadata{
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

			filterChains, err := createFilterChains()
			Expect(err).NotTo(HaveOccurred())
			Expect(filterChains).To(HaveLen(1))

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseTypedConfig(filterChains[0].Filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())
			clusters := cfg.GetWeightedClusters()
			Expect(clusters.Clusters).To(HaveLen(2))
			Expect(clusters.Clusters[0].Name).To(Equal(translatorutil.UpstreamToClusterName(&core.ResourceRef{Namespace: ns, Name: "one"})))
			Expect(clusters.Clusters[0].Weight).To(Equal(uint32(5)))
			Expect(clusters.Clusters[1].Name).To(Equal(translatorutil.UpstreamToClusterName(&core.ResourceRef{Namespace: ns, Name: "two"})))
			Expect(clusters.Clusters[1].Weight).To(Equal(uint32(1)))
		})

		It("can add the forward sni cluster name filter", func() {
			sslConfig := &ssl.SslConfig{
				SslSecrets: &ssl.SslConfig_SecretRef{
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
						ForwardSniClusterName: &empty.Empty{},
					},
				},
				SslConfig: sslConfig,
			})

			sslTranslator.EXPECT().
				ResolveDownstreamSslConfig(snap.Secrets, sslConfig).
				Return(&envoyauth.DownstreamTlsContext{}, nil)

			filterChains, err := createFilterChains()
			Expect(err).NotTo(HaveOccurred())
			Expect(filterChains).To(HaveLen(1))
			Expect(filterChains[0].Filters).To(HaveLen(2))
			Expect(filterChains[0].Filters[0].Name).To(Equal(SniFilter))
			sniClusterConfig := utils.MustAnyToMessage(filterChains[0].Filters[0].GetTypedConfig()).(*envoy_extensions_filters_network_sni_cluster_v3.SniCluster)
			Expect(sniClusterConfig).NotTo(BeNil())

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseTypedConfig(filterChains[0].Filters[1], &cfg)
			Expect(err).NotTo(HaveOccurred())
			cluster, ok := cfg.GetClusterSpecifier().(*envoytcp.TcpProxy_Cluster)
			Expect(ok).To(BeTrue(), "must be a single cluster type")
			Expect(cluster.Cluster).To(Equal(""))
		})

		It("should propagate proided `transport_socket_connect_timeout` to Envoy", func() {
			sslConfig := &ssl.SslConfig{
				SslSecrets: &ssl.SslConfig_SecretRef{
					SecretRef: &core.ResourceRef{
						Name:      "name",
						Namespace: "namespace",
					},
				},
				SniDomains: []string{"hello.world"},
				TransportSocketConnectTimeout: &durationpb.Duration{
					Seconds: 3,
					Nanos:   0,
				},
			}

			tcpListener.TcpHosts = append(tcpListener.TcpHosts, &v1.TcpHost{
				Name: "one",
				Destination: &v1.TcpHost_TcpAction{
					Destination: &v1.TcpHost_TcpAction_ForwardSniClusterName{
						ForwardSniClusterName: &empty.Empty{},
					},
				},
				SslConfig: sslConfig,
			})

			sslTranslator.EXPECT().
				ResolveDownstreamSslConfig(snap.Secrets, sslConfig).
				Return(&envoyauth.DownstreamTlsContext{}, nil)

			filterChains, err := createFilterChains()
			Expect(err).NotTo(HaveOccurred())
			Expect(filterChains).To(HaveLen(1))

			Expect(filterChains[0].TransportSocketConnectTimeout.Seconds).To(Equal(int64(3)))
			Expect(filterChains[0].TransportSocketConnectTimeout.Nanos).To(Equal(int32(0)))
		})
	})

})
