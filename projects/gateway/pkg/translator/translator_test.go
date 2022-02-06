package translator_test

import (
	"context"
	"time"

	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	. "github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/waf"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/als"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tcp"
	"github.com/solo-io/gloo/test/samples"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
)

const (
	ns  = "gloo-system"
	ns2 = "gloo-system2"
)

var _ = Describe("Translator", func() {
	var (
		snap       *v1.ApiSnapshot
		labelSet   = map[string]string{"a": "b"}
		translator Translator
	)

	Context("default translator", func() {

		BeforeEach(func() {
			translator = NewDefaultTranslator(Opts{})
			snap = &v1.ApiSnapshot{
				Gateways: v1.GatewayList{
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "name"},
						GatewayType: &v1.Gateway_HttpGateway{
							HttpGateway: &v1.HttpGateway{},
						},
						BindPort: 2,
					},
					{
						Metadata: &core.Metadata{Namespace: ns2, Name: "name2"},
						GatewayType: &v1.Gateway_HttpGateway{
							HttpGateway: &v1.HttpGateway{},
						},
						BindPort: 2,
						RouteOptions: &gloov1.RouteConfigurationOptions{
							MaxDirectResponseBodySizeBytes: &wrappers.UInt32Value{Value: 2048},
						},
					},
				},
				VirtualServices: v1.VirtualServiceList{
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "name1"},
						VirtualHost: &v1.VirtualHost{
							Domains: []string{"d1.com"},
							Routes: []*v1.Route{
								{
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/1",
										},
									}},
									Action: &v1.Route_DirectResponseAction{
										DirectResponseAction: &gloov1.DirectResponseAction{
											Body: "d1",
										},
									},
								},
							},
						},
					},
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "name2"},
						VirtualHost: &v1.VirtualHost{
							Domains: []string{"d2.com"},
							Routes: []*v1.Route{
								{
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/2",
										},
									}},
									Action: &v1.Route_DirectResponseAction{
										DirectResponseAction: &gloov1.DirectResponseAction{
											Body: "d2",
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
			proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

			Expect(errs).To(HaveLen(4))
			Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
			Expect(proxy.Metadata.Name).To(Equal(defaults.GatewayProxyName))
			Expect(proxy.Metadata.Namespace).To(Equal(ns))
			Expect(proxy.Listeners).To(HaveLen(1))
		})

		It("should properly translate listener plugins to proxy listener", func() {

			als := &als.AccessLoggingService{
				AccessLog: []*als.AccessLog{{
					OutputDestination: &als.AccessLog_FileSink{
						FileSink: &als.FileSink{
							Path: "/test",
						}},
				}},
			}
			snap.Gateways[0].Options = &gloov1.ListenerOptions{
				AccessLoggingService: als,
			}

			Expect(snap.Gateways[1].RouteOptions.MaxDirectResponseBodySizeBytes.Value).To(BeEquivalentTo(2048))

			httpGateway := snap.Gateways[0].GetHttpGateway()
			Expect(httpGateway).NotTo(BeNil())
			waf := &waf.Settings{
				CustomInterventionMessage: "custom",
			}
			httpGateway.Options = &gloov1.HttpListenerOptions{
				Waf: waf,
			}

			proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

			Expect(errs).To(HaveLen(4))
			Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
			Expect(proxy.Metadata.Name).To(Equal(defaults.GatewayProxyName))
			Expect(proxy.Metadata.Namespace).To(Equal(ns))
			Expect(proxy.Listeners).To(HaveLen(1))
			Expect(proxy.Listeners[0].Options.AccessLoggingService).To(Equal(als))
			httpListener := proxy.Listeners[0].GetHttpListener()
			Expect(httpListener).NotTo(BeNil())
			Expect(httpListener.Options.Waf).To(Equal(waf))
		})

		It("should translate three gateways with same name (different types) to one proxy with the same name", func() {
			snap.Gateways = append(
				snap.Gateways,
				&v1.Gateway{
					Metadata: &core.Metadata{Namespace: ns, Name: "name2"},
					GatewayType: &v1.Gateway_TcpGateway{
						TcpGateway: &v1.TcpGateway{},
					},
				},
				&v1.Gateway{
					Metadata: &core.Metadata{Namespace: ns, Name: "name2"},
					GatewayType: &v1.Gateway_HybridGateway{
						HybridGateway: &v1.HybridGateway{
							MatchedGateways: []*v1.MatchedGateway{
								{
									GatewayType: &v1.MatchedGateway_HttpGateway{
										HttpGateway: &v1.HttpGateway{},
									},
								},
							},
						},
					},
					BindPort: 3,
				},
			)

			proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

			Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
			Expect(proxy.Metadata.Name).To(Equal(defaults.GatewayProxyName))
			Expect(proxy.Metadata.Namespace).To(Equal(ns))
			Expect(proxy.Listeners).To(HaveLen(3))
		})

		It("should translate two gateways with same name (and types) to one proxy with the same name", func() {
			snap.Gateways = append(
				snap.Gateways,
				&v1.Gateway{
					Metadata: &core.Metadata{Namespace: ns, Name: "name2"},
					GatewayType: &v1.Gateway_HttpGateway{
						HttpGateway: &v1.HttpGateway{},
					},
				},
			)

			proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

			Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
			Expect(proxy.Metadata.Name).To(Equal(defaults.GatewayProxyName))
			Expect(proxy.Metadata.Namespace).To(Equal(ns))
			Expect(proxy.Listeners).To(HaveLen(2))
		})

		It("should error on two gateways with the same port in the same namespace", func() {
			dupeGateway := v1.Gateway{
				Metadata: &core.Metadata{Namespace: ns, Name: "name2"},
				BindPort: 2,
			}
			snap.Gateways = append(snap.Gateways, &dupeGateway)

			_, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)
			err := errs.ValidateStrict()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("bind-address :2 is not unique in a proxy. gateways: gloo-system.name,gloo-system.name2"))
		})

		It("should warn on vs with missing delegate action", func() {

			badRoute := &v1.Route{
				Action: &v1.Route_DelegateAction{
					DelegateAction: &v1.DelegateAction{
						DelegationType: &v1.DelegateAction_Ref{
							Ref: &core.ResourceRef{
								Name:      "don't",
								Namespace: "exist",
							},
						},
					},
				},
			}

			us := samples.SimpleUpstream()
			snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
			rt := snap.RouteTables[0]
			rt.Routes = append(rt.Routes, badRoute)

			_, reports := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)
			err := reports.Validate()
			Expect(err).NotTo(HaveOccurred())
			err = reports.ValidateStrict()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("route table exist.don't missing"))
		})

		Context("when the gateway CRDs don't clash", func() {
			BeforeEach(func() {
				translator = NewDefaultTranslator(Opts{
					ReadGatewaysFromAllNamespaces: true,
				})
				snap = &v1.ApiSnapshot{
					Gateways: v1.GatewayList{
						{
							Metadata: &core.Metadata{Namespace: ns, Name: "name"},
							GatewayType: &v1.Gateway_HttpGateway{
								HttpGateway: &v1.HttpGateway{},
							},
							BindPort: 2,
						},
						{
							Metadata: &core.Metadata{Namespace: ns2, Name: "name2"},
							GatewayType: &v1.Gateway_HttpGateway{
								HttpGateway: &v1.HttpGateway{},
							},
							BindPort: 3,
						},
					},
					VirtualServices: v1.VirtualServiceList{
						{
							Metadata: &core.Metadata{Namespace: ns, Name: "name1"},
							VirtualHost: &v1.VirtualHost{
								Domains: []string{"d1.com"},
								Routes: []*v1.Route{
									{
										Matchers: []*matchers.Matcher{{
											PathSpecifier: &matchers.Matcher_Prefix{
												Prefix: "/1",
											},
										}},
										Action: &v1.Route_DirectResponseAction{
											DirectResponseAction: &gloov1.DirectResponseAction{
												Body: "d1",
											},
										},
									},
								},
							},
						},
						{
							Metadata: &core.Metadata{Namespace: ns, Name: "name2"},
							VirtualHost: &v1.VirtualHost{
								Domains: []string{"d2.com"},
								Routes: []*v1.Route{
									{
										Matchers: []*matchers.Matcher{{
											PathSpecifier: &matchers.Matcher_Prefix{
												Prefix: "/2",
											},
										}},
										Action: &v1.Route_DirectResponseAction{
											DirectResponseAction: &gloov1.DirectResponseAction{
												Body: "d2",
											},
										},
									},
								},
							},
						},
					},
				}
			})

			It("should have the same number of listeners as gateways in the cluster", func() {
				proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

				Expect(errs).To(HaveLen(4))
				Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
				Expect(proxy.Metadata.Name).To(Equal(defaults.GatewayProxyName))
				Expect(proxy.Metadata.Namespace).To(Equal(ns))
				Expect(proxy.Listeners).To(HaveLen(2))
			})
		})
	})

	Context("hybrid", func() {

		var (
			idleTimeout        *duration.Duration
			tcpListenerOptions *gloov1.TcpListenerOptions
			tcpHost            *gloov1.TcpHost
		)

		BeforeEach(func() {
			hybridTranslator := &HybridTranslator{HttpTranslator: &HttpTranslator{}}
			translator = NewTranslator([]ListenerTranslator{hybridTranslator}, Opts{})

			idleTimeout = prototime.DurationToProto(5 * time.Second)
			tcpListenerOptions = &gloov1.TcpListenerOptions{
				TcpProxySettings: &tcp.TcpProxySettings{
					MaxConnectAttempts: &wrappers.UInt32Value{Value: 10},
					IdleTimeout:        idleTimeout,
					TunnelingConfig:    &tcp.TcpProxySettings_TunnelingConfig{Hostname: "proxyhostname"},
				},
			}
			tcpHost = &gloov1.TcpHost{
				Name: "host-one",
				Destination: &gloov1.TcpHost_TcpAction{
					Destination: &gloov1.TcpHost_TcpAction_UpstreamGroup{
						UpstreamGroup: &core.ResourceRef{
							Namespace: ns,
							Name:      "ug-name",
						},
					},
				},
			}

			snap = &v1.ApiSnapshot{
				Gateways: v1.GatewayList{
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "name"},
						GatewayType: &v1.Gateway_HybridGateway{
							HybridGateway: &v1.HybridGateway{
								MatchedGateways: []*v1.MatchedGateway{
									{
										Matcher: &v1.Matcher{
											SourcePrefixRanges: []*v3.CidrRange{
												{
													AddressPrefix: "match1",
												},
											},
										},
										GatewayType: &v1.MatchedGateway_HttpGateway{
											HttpGateway: &v1.HttpGateway{},
										},
									},
									{
										Matcher: &v1.Matcher{
											SourcePrefixRanges: []*v3.CidrRange{
												{
													AddressPrefix: "match2",
												},
											},
										},
										GatewayType: &v1.MatchedGateway_TcpGateway{
											TcpGateway: &v1.TcpGateway{
												Options:  tcpListenerOptions,
												TcpHosts: []*gloov1.TcpHost{tcpHost},
											},
										},
									},
								},
							},
						},
						BindPort: 2,
					},
				},

				VirtualServices: v1.VirtualServiceList{
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "name1", Labels: labelSet},
						VirtualHost: &v1.VirtualHost{
							Domains: []string{"d1.com"},
							Routes: []*v1.Route{
								{
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/1",
										},
									}},
									Action: &v1.Route_DirectResponseAction{
										DirectResponseAction: &gloov1.DirectResponseAction{
											Body: "d1",
										},
									},
								},
							},
						},
					},
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "name2"},
						VirtualHost: &v1.VirtualHost{
							Domains: []string{"d2.com"},
							Routes: []*v1.Route{
								{
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/2",
										},
									}},
									Action: &v1.Route_DirectResponseAction{
										DirectResponseAction: &gloov1.DirectResponseAction{
											Body: "d2",
										},
									},
								},
							},
						},
					},
					{
						Metadata: &core.Metadata{Namespace: ns + "-other-namespace", Name: "name3", Labels: labelSet},
						VirtualHost: &v1.VirtualHost{
							Domains: []string{"d3.com"},
							Routes: []*v1.Route{
								{
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/3",
										},
									}},
									Action: &v1.Route_DirectResponseAction{
										DirectResponseAction: &gloov1.DirectResponseAction{
											Body: "d3",
										},
									},
								},
							},
						},
					},
				},
			}
		})

		It("can properly translate a hybrid proxy", func() {
			proxy, _ := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

			Expect(proxy.Listeners).To(HaveLen(1))
			listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HybridListener).HybridListener
			Expect(listener.MatchedListeners).To(HaveLen(2))

			// http matched listener
			Expect(listener.MatchedListeners[0].Matcher.SourcePrefixRanges).To(HaveLen(1))
			Expect(listener.MatchedListeners[0].Matcher.SourcePrefixRanges[0].AddressPrefix).To(Equal("match1"))
			Expect(listener.MatchedListeners[0].GetHttpListener()).NotTo(BeNil())
			Expect(listener.MatchedListeners[0].GetHttpListener().VirtualHosts).To(HaveLen(len(snap.VirtualServices)))

			// tcp matched listener
			Expect(listener.MatchedListeners[1].Matcher.SourcePrefixRanges).To(HaveLen(1))
			Expect(listener.MatchedListeners[1].Matcher.SourcePrefixRanges[0].AddressPrefix).To(Equal("match2"))
			Expect(listener.MatchedListeners[1].GetTcpListener()).NotTo(BeNil())
			Expect(listener.MatchedListeners[1].GetTcpListener().Options).To(Equal(tcpListenerOptions))
			Expect(listener.MatchedListeners[1].GetTcpListener().TcpHosts).To(HaveLen(1))
			Expect(listener.MatchedListeners[1].GetTcpListener().TcpHosts[0]).To(Equal(tcpHost))
		})

		It("skips hybrid gateways that have no sub-gateways", func() {
			snap.Gateways = v1.GatewayList{
				{
					Metadata: &core.Metadata{Namespace: ns, Name: "name"},
					GatewayType: &v1.Gateway_HybridGateway{
						HybridGateway: &v1.HybridGateway{},
					},
					BindPort: 1,
				},
				{
					Metadata: &core.Metadata{Namespace: ns, Name: "name"},
					GatewayType: &v1.Gateway_HybridGateway{
						HybridGateway: &v1.HybridGateway{
							MatchedGateways: []*v1.MatchedGateway{
								{
									Matcher: &v1.Matcher{
										SourcePrefixRanges: []*v3.CidrRange{
											{
												AddressPrefix: "match1",
											},
										},
									},
								},
							},
						},
					},
					BindPort: 2,
				},
			}

			proxy, _ := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

			Expect(proxy.GetListeners()).To(BeNil())
		})

	})
})

var expectedRouteMetadatas = [][]*SourceMetadata{
	{
		{
			Sources: []SourceRef{
				{
					ResourceRef: &core.ResourceRef{
						Name:      "delegate-1",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: &core.ResourceRef{
						Name:      "name1",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.VirtualService",
					ObservedGeneration: 0,
				},
			},
		},
		{
			Sources: []SourceRef{
				{
					ResourceRef: &core.ResourceRef{
						Name:      "delegate-3",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: &core.ResourceRef{
						Name:      "delegate-1",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: &core.ResourceRef{
						Name:      "name1",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.VirtualService",
					ObservedGeneration: 0,
				},
			},
		},
		{
			Sources: []SourceRef{
				{
					ResourceRef: &core.ResourceRef{
						Name:      "delegate-3",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: &core.ResourceRef{
						Name:      "delegate-1",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: &core.ResourceRef{
						Name:      "name1",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.VirtualService",
					ObservedGeneration: 0,
				},
			},
		},
	},
	{
		{
			Sources: []SourceRef{
				{
					ResourceRef: &core.ResourceRef{
						Name:      "delegate-2",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: &core.ResourceRef{
						Name:      "name2",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.VirtualService",
					ObservedGeneration: 0,
				},
			},
		},
		{
			Sources: []SourceRef{
				{
					ResourceRef: &core.ResourceRef{
						Name:      "delegate-2",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: &core.ResourceRef{
						Name:      "name2",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.VirtualService",
					ObservedGeneration: 0,
				},
			},
		},
	},
}

func expectedRouteMetadata(virtualHostIndex, routeIndex int) *SourceMetadata {
	return expectedRouteMetadatas[virtualHostIndex][routeIndex]
}
