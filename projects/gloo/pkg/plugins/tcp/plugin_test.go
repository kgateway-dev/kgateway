package tcp_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/tcp"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/tcp"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Plugin", func() {
	var (
		in *v1.Listener
	)

	Context("listener plugin", func() {

		var (
			outl    *envoyapi.Listener
			filters []envoylistener.Filter
			tcps    *tcp.TcpProxySettings
		)

		BeforeEach(func() {
			pd := func(t time.Duration) *time.Duration { return &t }
			tcps = &tcp.TcpProxySettings{
				MaxConnectAttempts: &types.UInt32Value{
					Value: 5,
				},
				IdleTimeout: pd(5 * time.Second),
			}
			tl := &v1.TcpListener{
				Plugins: &v1.TcpListenerPlugins{
					TcpProxySettings: tcps,
				},
			}

			in = &v1.Listener{
				ListenerType: &v1.Listener_TcpListener{
					TcpListener: tl,
				},
			}
			filters = []envoylistener.Filter{{
				Name: envoyutil.TCPProxy,
			}}
			outl = &envoyapi.Listener{
				FilterChains: []envoylistener.FilterChain{{
					Filters: filters,
				}},
			}
		})
		It("copy all settings to tcp filter", func() {

			p := NewPlugin()
			err := p.ProcessListener(plugins.Params{}, in, outl)
			Expect(err).NotTo(HaveOccurred())

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseConfig(&filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.IdleTimeout).To(Equal(tcps.IdleTimeout))
			Expect(cfg.MaxConnectAttempts).To(Equal(tcps.MaxConnectAttempts))
		})

	})

	Context("listener filter chain plugin", func() {
		var (
			tcpListener *v1.TcpListener
			snap        *v1.ApiSnapshot

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
			tcpListener = &v1.TcpListener{
				TcpHosts: []*v1.TcpHost{},
			}
			in = &v1.Listener{
				ListenerType: &v1.Listener_TcpListener{
					TcpListener: tcpListener,
				},
			}
		})

		It("can transform a single destination", func() {
			tcpListener.TcpHosts = append(tcpListener.TcpHosts, &v1.TcpHost{
				Name: "one",
				Destination: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
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
				SslConfig: nil,
			})
			p := NewPlugin()
			filterChains, err := p.ProcessListenerFilterChain(plugins.Params{Snapshot: snap}, in)
			Expect(err).NotTo(HaveOccurred())
			Expect(filterChains).To(HaveLen(1))

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseConfig(&filterChains[0].Filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())
			cluster := cfg.GetCluster()
			Expect(cluster).To(Equal(translatorutil.UpstreamToClusterName(core.ResourceRef{Namespace: ns, Name: "one"})))
		})
		It("can transform a multi destination", func() {
			tcpListener.TcpHosts = append(tcpListener.TcpHosts, &v1.TcpHost{
				Name: "one",
				Destination: &v1.RouteAction{
					Destination: &v1.RouteAction_Multi{
						Multi: &v1.MultiDestination{
							Destinations: wd,
						},
					},
				},
				SslConfig: nil,
			})
			p := NewPlugin()
			filterChains, err := p.ProcessListenerFilterChain(plugins.Params{Snapshot: snap}, in)
			Expect(err).NotTo(HaveOccurred())
			Expect(filterChains).To(HaveLen(1))

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseConfig(&filterChains[0].Filters[0], &cfg)
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
				Destination: &v1.RouteAction{
					Destination: &v1.RouteAction_UpstreamGroup{
						UpstreamGroup: &core.ResourceRef{
							Namespace: ns,
							Name:      "one",
						},
					},
				},
				SslConfig: nil,
			})
			p := NewPlugin()
			filterChains, err := p.ProcessListenerFilterChain(plugins.Params{Snapshot: snap}, in)
			Expect(err).NotTo(HaveOccurred())
			Expect(filterChains).To(HaveLen(1))

			var cfg envoytcp.TcpProxy
			err = translatorutil.ParseConfig(&filterChains[0].Filters[0], &cfg)
			Expect(err).NotTo(HaveOccurred())
			clusters := cfg.GetWeightedClusters()
			Expect(clusters.Clusters).To(HaveLen(2))
			Expect(clusters.Clusters[0].Name).To(Equal(translatorutil.UpstreamToClusterName(core.ResourceRef{Namespace: ns, Name: "one"})))
			Expect(clusters.Clusters[0].Weight).To(Equal(uint32(5)))
			Expect(clusters.Clusters[1].Name).To(Equal(translatorutil.UpstreamToClusterName(core.ResourceRef{Namespace: ns, Name: "two"})))
			Expect(clusters.Clusters[1].Weight).To(Equal(uint32(1)))
		})
	})

})
