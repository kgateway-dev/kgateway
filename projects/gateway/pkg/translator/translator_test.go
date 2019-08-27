package translator_test

import (
	"context"
	"time"

	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/tcp"

	. "github.com/solo-io/gloo/projects/gateway/pkg/translator"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	ns  = "gloo-system"
	ns2 = "gloo-system2"
)

var _ = Describe("Translator", func() {
	var (
		snap       *v2.ApiSnapshot
		translator Translator
	)

	Context("translator", func() {
		BeforeEach(func() {
			translator = NewTranslator([]ListenerFactory{&HttpTranslator{}, &TcpTranslator{}})
			snap = &v2.ApiSnapshot{
				Gateways: v2.GatewayList{
					{
						Metadata: core.Metadata{Namespace: ns, Name: "name"},
						GatewayType: &v2.Gateway_HttpGateway{
							HttpGateway: &v2.HttpGateway{},
						},
						BindPort: 2,
					},
					{
						Metadata: core.Metadata{Namespace: ns2, Name: "name2"},
						GatewayType: &v2.Gateway_HttpGateway{
							HttpGateway: &v2.HttpGateway{},
						},
						BindPort: 2,
					},
				},
				VirtualServices: v1.VirtualServiceList{
					{
						Metadata: core.Metadata{Namespace: ns, Name: "name1"},
						VirtualHost: &gloov1.VirtualHost{
							Domains: []string{"d1.com"},
							Routes: []*gloov1.Route{
								{
									Matcher: &gloov1.Matcher{
										PathSpecifier: &gloov1.Matcher_Prefix{
											Prefix: "/1",
										},
									},
								},
							},
						},
					},
					{
						Metadata: core.Metadata{Namespace: ns, Name: "name2"},
						VirtualHost: &gloov1.VirtualHost{
							Domains: []string{"d2.com"},
							Routes: []*gloov1.Route{
								{
									Matcher: &gloov1.Matcher{
										PathSpecifier: &gloov1.Matcher_Prefix{
											Prefix: "/2",
										},
									},
								},
							},
						},
					},
				},
			}
		})

		It("should translate proxy with default name", func() {
			proxy, errs := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

			Expect(errs).To(HaveLen(3))
			Expect(errs.Validate()).NotTo(HaveOccurred())
			Expect(proxy.Metadata.Name).To(Equal(GatewayProxyName))
			Expect(proxy.Metadata.Namespace).To(Equal(ns))
		})

		It("should properly translate listener plugins to proxy listener", func() {
			extensions := map[string]*types.Struct{
				"plugin": &types.Struct{},
			}

			snap.Gateways[0].Plugins = &gloov1.ListenerPlugins{
				Extensions: &gloov1.Extensions{
					Configs: extensions,
				},
			}

			proxy, errs := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

			Expect(errs).To(HaveLen(3))
			Expect(errs.Validate()).NotTo(HaveOccurred())
			Expect(proxy.Metadata.Name).To(Equal(GatewayProxyName))
			Expect(proxy.Metadata.Namespace).To(Equal(ns))
			Expect(proxy.Listeners).To(HaveLen(1))
			Expect(proxy.Listeners[0].Plugins.Extensions.Configs).To(HaveKey("plugin"))
			Expect(proxy.Listeners[0].Plugins.Extensions.Configs["plugin"]).To(Equal(extensions["plugin"]))
		})

		It("should translate two gateways with same name (different types) to one proxy with the same name", func() {
			snap.Gateways = append(
				snap.Gateways,
				&v2.Gateway{
					Metadata: core.Metadata{Namespace: ns, Name: "name2"},
					GatewayType: &v2.Gateway_TcpGateway{
						TcpGateway: &v2.TcpGateway{},
					},
				},
			)

			proxy, errs := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

			Expect(errs.Validate()).NotTo(HaveOccurred())
			Expect(proxy.Metadata.Name).To(Equal(GatewayProxyName))
			Expect(proxy.Metadata.Namespace).To(Equal(ns))
			Expect(proxy.Listeners).To(HaveLen(2))
		})

		It("should translate two gateways with same name (and types) to one proxy with the same name", func() {
			snap.Gateways = append(
				snap.Gateways,
				&v2.Gateway{
					Metadata: core.Metadata{Namespace: ns, Name: "name2"},
					GatewayType: &v2.Gateway_HttpGateway{
						HttpGateway: &v2.HttpGateway{},
					},
				},
			)

			proxy, errs := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

			Expect(errs.Validate()).NotTo(HaveOccurred())
			Expect(proxy.Metadata.Name).To(Equal(GatewayProxyName))
			Expect(proxy.Metadata.Namespace).To(Equal(ns))
			Expect(proxy.Listeners).To(HaveLen(2))
		})

		It("should error on two gateways with the same port in the same namespace", func() {
			dupeGateway := v2.Gateway{
				Metadata: core.Metadata{Namespace: ns, Name: "name2"},
				BindPort: 2,
			}
			snap.Gateways = append(snap.Gateways, &dupeGateway)

			_, errs := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)
			err := errs.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("bind-address :2 is not unique in a proxy. gateways: gloo-system.name,gloo-system.name2"))
		})

	})

	Context("http", func() {
		var (
			factory *HttpTranslator
		)

		BeforeEach(func() {
			factory = &HttpTranslator{}
			translator = NewTranslator([]ListenerFactory{factory})
			snap = &v2.ApiSnapshot{
				Gateways: v2.GatewayList{
					{
						Metadata: core.Metadata{Namespace: ns, Name: "name"},
						GatewayType: &v2.Gateway_HttpGateway{
							HttpGateway: &v2.HttpGateway{},
						},
						BindPort: 2,
					},
					{
						Metadata: core.Metadata{Namespace: ns2, Name: "name2"},
						GatewayType: &v2.Gateway_HttpGateway{
							HttpGateway: &v2.HttpGateway{},
						},
						BindPort: 2,
					},
				},
				VirtualServices: v1.VirtualServiceList{
					{
						Metadata: core.Metadata{Namespace: ns, Name: "name1"},
						VirtualHost: &gloov1.VirtualHost{
							Domains: []string{"d1.com"},
							Routes: []*gloov1.Route{
								{
									Matcher: &gloov1.Matcher{
										PathSpecifier: &gloov1.Matcher_Prefix{
											Prefix: "/1",
										},
									},
								},
							},
						},
					},
					{
						Metadata: core.Metadata{Namespace: ns, Name: "name2"},
						VirtualHost: &gloov1.VirtualHost{
							Domains: []string{"d2.com"},
							Routes: []*gloov1.Route{
								{
									Matcher: &gloov1.Matcher{
										PathSpecifier: &gloov1.Matcher_Prefix{
											Prefix: "/2",
										},
									},
								},
							},
						},
					},
				},
			}
		})

		It("should translate an empty gateway to have all vservices", func() {

			proxy, _ := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

			Expect(proxy.Listeners).To(HaveLen(1))
			listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
			Expect(listener.VirtualHosts).To(HaveLen(2))
		})

		It("should have no ssl config", func() {
			proxy, _ := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

			Expect(proxy.Listeners).To(HaveLen(1))
			Expect(proxy.Listeners[0].SslConfigurations).To(BeEmpty())
		})

		It("should translate a gateway to only have its vservices", func() {
			snap.Gateways[0].GatewayType = &v2.Gateway_HttpGateway{
				HttpGateway: &v2.HttpGateway{
					VirtualServices: []core.ResourceRef{snap.VirtualServices[0].Metadata.Ref()},
				},
			}

			proxy, errs := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

			Expect(errs.Validate()).NotTo(HaveOccurred())
			Expect(proxy).NotTo(BeNil())
			Expect(proxy.Listeners).To(HaveLen(1))
			listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
			Expect(listener.VirtualHosts).To(HaveLen(1))
		})

		It("should not have vhosts with ssl", func() {
			snap.VirtualServices[0].SslConfig = new(gloov1.SslConfig)

			proxy, errs := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

			Expect(errs.Validate()).NotTo(HaveOccurred())

			Expect(proxy.Listeners).To(HaveLen(1))
			listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
			Expect(listener.VirtualHosts).To(HaveLen(1))
			Expect(listener.VirtualHosts[0].Name).To(ContainSubstring("name2"))
		})

		It("should not have vhosts without ssl", func() {
			snap.Gateways[0].Ssl = true
			snap.VirtualServices[0].SslConfig = new(gloov1.SslConfig)

			proxy, errs := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

			Expect(errs.Validate()).NotTo(HaveOccurred())

			Expect(proxy.Listeners).To(HaveLen(1))
			listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
			Expect(listener.VirtualHosts).To(HaveLen(1))
			Expect(listener.VirtualHosts[0].Name).To(ContainSubstring("name1"))
		})

		Context("merge", func() {
			BeforeEach(func() {
				snap.VirtualServices[1].VirtualHost.Domains = snap.VirtualServices[0].VirtualHost.Domains
			})

			It("should translate 2 virtual services with the same domains to 1 virtual service", func() {

				proxy, errs := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

				Expect(errs.Validate()).NotTo(HaveOccurred())
				Expect(proxy.Metadata.Name).To(Equal(GatewayProxyName))
				Expect(proxy.Metadata.Namespace).To(Equal(ns))
				Expect(proxy.Listeners).To(HaveLen(1))
				listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
				Expect(listener.VirtualHosts).To(HaveLen(1))
			})

			It("should translate 2 virtual services with the empty domains", func() {
				snap.VirtualServices[1].VirtualHost.Domains = nil
				snap.VirtualServices[0].VirtualHost.Domains = nil

				proxy, errs := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

				Expect(errs.Validate()).NotTo(HaveOccurred())
				Expect(proxy.Listeners).To(HaveLen(1))
				listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
				Expect(listener.VirtualHosts).To(HaveLen(1))
				Expect(listener.VirtualHosts[0].Name).NotTo(BeEmpty())
				Expect(listener.VirtualHosts[0].Name).NotTo(Equal(ns + "."))
			})

			It("should not error with one contains plugins", func() {
				snap.VirtualServices[0].VirtualHost.VirtualHostPlugins = new(gloov1.VirtualHostPlugins)

				_, errs := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

				Expect(errs.Validate()).NotTo(HaveOccurred())
			})

			It("should error with both having plugins", func() {
				snap.VirtualServices[0].VirtualHost.VirtualHostPlugins = new(gloov1.VirtualHostPlugins)
				snap.VirtualServices[1].VirtualHost.VirtualHostPlugins = new(gloov1.VirtualHostPlugins)

				_, errs := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

				Expect(errs.Validate()).To(HaveOccurred())
			})

			It("should not error with one contains ssl config", func() {
				snap.VirtualServices[0].SslConfig = new(gloov1.SslConfig)

				proxy, errs := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

				Expect(errs.Validate()).NotTo(HaveOccurred())
				listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
				Expect(listener.VirtualHosts).To(HaveLen(1))
			})

			It("should not error with one contains ssl config", func() {
				snap.Gateways[0].Ssl = true
				snap.VirtualServices[0].SslConfig = new(gloov1.SslConfig)

				proxy, errs := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

				Expect(errs.Validate()).NotTo(HaveOccurred())
				listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
				Expect(listener.VirtualHosts).To(HaveLen(1))
				Expect(listener.VirtualHosts[0].Routes).To(HaveLen(1))
			})

			It("should error when two virtual services conflict", func() {
				snap.Gateways[0].Ssl = true
				snap.VirtualServices[0].SslConfig = new(gloov1.SslConfig)
				snap.VirtualServices[1].SslConfig = new(gloov1.SslConfig)
				snap.VirtualServices[0].SslConfig.SniDomains = []string{"bar"}
				snap.VirtualServices[1].SslConfig.SniDomains = []string{"foo"}

				_, errs := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

				Expect(errs.Validate()).To(HaveOccurred())
			})

			It("should error when two virtual services conflict", func() {
				snap.Gateways[0].Ssl = true
				snap.VirtualServices[0].SslConfig = new(gloov1.SslConfig)
				snap.VirtualServices[1].SslConfig = new(gloov1.SslConfig)
				snap.VirtualServices[0].SslConfig.SniDomains = []string{"foo"}
				snap.VirtualServices[1].SslConfig.SniDomains = []string{"foo"}

				_, errs := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

				Expect(errs.Validate()).NotTo(HaveOccurred())
			})

		})
	})

	Context("tcp", func() {
		var (
			factory     *TcpTranslator
			idleTimeout time.Duration
			plugins     *gloov1.TcpListenerPlugins
			destination *gloov1.TcpHost
		)
		BeforeEach(func() {
			factory = &TcpTranslator{}
			translator = NewTranslator([]ListenerFactory{factory})

			idleTimeout = 5 * time.Second
			plugins = &gloov1.TcpListenerPlugins{
				TcpProxySettings: &tcp.TcpProxySettings{
					MaxConnectAttempts: &types.UInt32Value{Value: 10},
					IdleTimeout:        &idleTimeout,
				},
			}
			destination = &gloov1.TcpHost{
				Name: "host-one",
				Destination: &gloov1.RouteAction{
					Destination: &gloov1.RouteAction_UpstreamGroup{
						UpstreamGroup: &core.ResourceRef{
							Namespace: ns,
							Name:      "ug-name",
						},
					},
				},
			}

			snap = &v2.ApiSnapshot{
				Gateways: v2.GatewayList{
					{
						Metadata: core.Metadata{Namespace: ns, Name: "name"},
						GatewayType: &v2.Gateway_TcpGateway{
							TcpGateway: &v2.TcpGateway{
								Destinations: []*gloov1.TcpHost{destination},
								Plugins:      plugins,
							},
						},
						BindPort: 2,
					},
				},
			}
		})

		It("can properly translate a tcp proxy", func() {
			proxy, _ := translator.Translate(context.Background(), GatewayProxyName, ns, snap, snap.Gateways)

			Expect(proxy.Listeners).To(HaveLen(1))
			listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_TcpListener).TcpListener
			Expect(listener.Plugins).To(Equal(plugins))
			Expect(listener.TcpHosts).To(HaveLen(1))
			Expect(listener.TcpHosts[0]).To(Equal(destination))
		})

	})

})
