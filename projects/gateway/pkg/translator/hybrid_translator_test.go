package translator_test

import (
	"context"
	"fmt"
	"time"

	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

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
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tcp"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
)

var _ = Describe("Hybrid Translator", func() {

	var (
		ctx              context.Context
		cancel           context.CancelFunc
		hybridTranslator *HybridTranslator
		snap             *v1.ApiSnapshot
		reports          reporter.ResourceReports
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		hybridTranslator = &HybridTranslator{
			HttpTranslator: &HttpTranslator{
				WarnOnRouteShortCircuiting: false,
			},
			TcpTranslator: &TcpTranslator{},
		}
	})

	JustBeforeEach(func() {
		// In case sub-contexts modify the snapshot, ensure that we build the ResourceReports last
		reports = make(reporter.ResourceReports)
		reports.Accept(snap.Gateways.AsInputResources()...)
		reports.Accept(snap.VirtualServices.AsInputResources()...)
		reports.Accept(snap.RouteTables.AsInputResources()...)
	})

	AfterEach(func() {
		cancel()
	})

	Context("no sub-gateways", func() {

		BeforeEach(func() {
			snap = &v1.ApiSnapshot{
				Gateways: v1.GatewayList{
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "name"},
						GatewayType: &v1.Gateway_HybridGateway{
							HybridGateway: &v1.HybridGateway{},
						},
						BindPort: 1,
					},
				},
			}
		})

		It("does not generate a listener", func() {
			params := NewTranslatorParams(ctx, snap, reports)

			listener := hybridTranslator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
			Expect(listener).To(BeNil())
		})

	})

	Context("MatchedGateway", func() {

		Context("http", func() {

			Context("non-ssl", func() {

				BeforeEach(func() {
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
										},
									},
								},
								BindPort: 2,
							},
						},

						VirtualServices: v1.VirtualServiceList{
							createVirtualService("name1", ns, false),
							createVirtualService("name2", ns, false),
							createVirtualService("name3", ns+"-other-namespace", false),
						},
					}
				})

				It("works", func() {
					params := NewTranslatorParams(ctx, snap, reports)

					listener := hybridTranslator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
					Expect(listener).NotTo(BeNil())

					hybridListener := listener.ListenerType.(*gloov1.Listener_HybridListener).HybridListener
					Expect(hybridListener.MatchedListeners).To(HaveLen(1))

					matchedHttpListener := hybridListener.MatchedListeners[0]
					Expect(matchedHttpListener.Matcher.SourcePrefixRanges).To(HaveLen(1))
					Expect(matchedHttpListener.Matcher.SourcePrefixRanges[0].AddressPrefix).To(Equal("match1"))
					Expect(matchedHttpListener.GetHttpListener()).NotTo(BeNil())
					Expect(matchedHttpListener.GetHttpListener().VirtualHosts).To(HaveLen(len(snap.VirtualServices)))
				})

			})

			Context("ssl", func() {

				BeforeEach(func() {
					snap = &v1.ApiSnapshot{
						Gateways: v1.GatewayList{
							{
								Metadata: &core.Metadata{Namespace: ns, Name: "name"},
								GatewayType: &v1.Gateway_HybridGateway{
									HybridGateway: &v1.HybridGateway{
										MatchedGateways: []*v1.MatchedGateway{
											{
												Matcher: &v1.Matcher{
													// This is important as it means the Gateway will only select
													// VirtualServices with Ssl defined
													SslConfig: &gloov1.SslConfig{},
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
										},
									},
								},
								BindPort: 2,
							},
						},

						VirtualServices: v1.VirtualServiceList{
							createVirtualService("name1", ns, true),
							createVirtualService("name2", ns, true),
							createVirtualService("name3", ns+"-other-namespace", true),
						},
					}
				})

				It("works", func() {
					params := NewTranslatorParams(ctx, snap, reports)

					listener := hybridTranslator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
					Expect(listener).NotTo(BeNil())

					hybridListener := listener.ListenerType.(*gloov1.Listener_HybridListener).HybridListener
					Expect(hybridListener.MatchedListeners).To(HaveLen(1))

					matchedHttpListener := hybridListener.MatchedListeners[0]
					Expect(matchedHttpListener.Matcher.SourcePrefixRanges).To(HaveLen(1))
					Expect(matchedHttpListener.Matcher.SourcePrefixRanges[0].AddressPrefix).To(Equal("match1"))
					Expect(matchedHttpListener.GetHttpListener()).NotTo(BeNil())
					Expect(matchedHttpListener.GetHttpListener().VirtualHosts).To(HaveLen(len(snap.VirtualServices)))

					// Only the VirtualServices with SslConfig should be aggregated on the Gateway
					Expect(matchedHttpListener.GetSslConfigurations()).To(HaveLen(len(snap.VirtualServices)))
				})

			})

		})

		Context("tcp", func() {

			var (
				idleTimeout        *duration.Duration
				tcpListenerOptions *gloov1.TcpListenerOptions
				tcpHost            *gloov1.TcpHost
			)

			BeforeEach(func() {
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
				}
			})

			It("works", func() {
				params := NewTranslatorParams(ctx, snap, reports)

				listener := hybridTranslator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				Expect(listener).NotTo(BeNil())

				hybridListener := listener.ListenerType.(*gloov1.Listener_HybridListener).HybridListener
				Expect(hybridListener.MatchedListeners).To(HaveLen(1))

				matchedTcpListener := hybridListener.MatchedListeners[0]
				Expect(matchedTcpListener.Matcher.SourcePrefixRanges).To(HaveLen(1))
				Expect(matchedTcpListener.Matcher.SourcePrefixRanges[0].AddressPrefix).To(Equal("match2"))
				Expect(matchedTcpListener.GetTcpListener()).NotTo(BeNil())
				Expect(matchedTcpListener.GetTcpListener().Options).To(Equal(tcpListenerOptions))
				Expect(matchedTcpListener.GetTcpListener().TcpHosts).To(HaveLen(1))
				Expect(matchedTcpListener.GetTcpListener().TcpHosts[0]).To(Equal(tcpHost))
			})
		})

	})

	Context("DelegatedHttpGateways", func() {

		Context("http", func() {

			Context("non-ssl", func() {

				BeforeEach(func() {
					snap = &v1.ApiSnapshot{
						Gateways: v1.GatewayList{
							{
								Metadata: &core.Metadata{Namespace: ns, Name: "name"},
								GatewayType: &v1.Gateway_HybridGateway{
									HybridGateway: &v1.HybridGateway{
										DelegatedHttpGateways: &v1.DelegatedHttpGateway{
											SelectionType: &v1.DelegatedHttpGateway_Ref{
												Ref: &core.ResourceRef{
													Name:      "name",
													Namespace: ns,
												},
											},
										},
									},
								},
								BindPort: 2,
							},
						},

						HttpGateways: v1.MatchableHttpGatewayList{
							{
								Metadata: &core.Metadata{Namespace: ns, Name: "name"},
								Matcher: &v1.MatchableHttpGateway_Matcher{
									SourcePrefixRanges: []*v3.CidrRange{
										{
											AddressPrefix: "match1",
										},
									},
								},
								HttpGateway: &v1.HttpGateway{},
							},
						},

						VirtualServices: v1.VirtualServiceList{
							createVirtualService("name1", ns, false),
							createVirtualService("name2", ns, false),
							createVirtualService("name3", ns+"-other-namespace", false),
						},
					}
				})

				It("works", func() {
					params := NewTranslatorParams(ctx, snap, reports)

					listener := hybridTranslator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
					Expect(listener).NotTo(BeNil())

					hybridListener := listener.ListenerType.(*gloov1.Listener_HybridListener).HybridListener
					Expect(hybridListener.MatchedListeners).To(HaveLen(1))

					matchedHttpListener := hybridListener.MatchedListeners[0]
					Expect(matchedHttpListener.Matcher.SourcePrefixRanges).To(HaveLen(1))
					Expect(matchedHttpListener.Matcher.SourcePrefixRanges[0].AddressPrefix).To(Equal("match1"))
					Expect(matchedHttpListener.GetHttpListener()).NotTo(BeNil())
					Expect(matchedHttpListener.GetHttpListener().VirtualHosts).To(HaveLen(len(snap.VirtualServices)))
				})

			})

			Context("ssl", func() {

				BeforeEach(func() {
					snap = &v1.ApiSnapshot{
						Gateways: v1.GatewayList{
							{
								Metadata: &core.Metadata{Namespace: ns, Name: "name"},
								GatewayType: &v1.Gateway_HybridGateway{
									HybridGateway: &v1.HybridGateway{
										DelegatedHttpGateways: &v1.DelegatedHttpGateway{
											SelectionType: &v1.DelegatedHttpGateway_Ref{
												Ref: &core.ResourceRef{
													Name:      "name",
													Namespace: ns,
												},
											},
										},
									},
								},
								BindPort: 2,
							},
						},

						HttpGateways: v1.MatchableHttpGatewayList{
							{
								Metadata: &core.Metadata{Namespace: ns, Name: "name"},
								Matcher: &v1.MatchableHttpGateway_Matcher{
									// This is important as it means the Gateway will only select
									// VirtualServices with Ssl defined
									SslConfig: &gloov1.SslConfig{},
									SourcePrefixRanges: []*v3.CidrRange{
										{
											AddressPrefix: "match1",
										},
									},
								},
								HttpGateway: &v1.HttpGateway{},
							},
						},

						VirtualServices: v1.VirtualServiceList{
							createVirtualService("name1", ns, true),
							createVirtualService("name2", ns, true),
							createVirtualService("name3", ns+"-other-namespace", true),
						},
					}
				})

				It("works", func() {
					params := NewTranslatorParams(ctx, snap, reports)

					listener := hybridTranslator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
					Expect(listener).NotTo(BeNil())

					hybridListener := listener.ListenerType.(*gloov1.Listener_HybridListener).HybridListener
					Expect(hybridListener.MatchedListeners).To(HaveLen(1))

					matchedHttpListener := hybridListener.MatchedListeners[0]
					Expect(matchedHttpListener.Matcher.SourcePrefixRanges).To(HaveLen(1))
					Expect(matchedHttpListener.Matcher.SourcePrefixRanges[0].AddressPrefix).To(Equal("match1"))
					Expect(matchedHttpListener.GetHttpListener()).NotTo(BeNil())
					Expect(matchedHttpListener.GetHttpListener().VirtualHosts).To(HaveLen(len(snap.VirtualServices)))

					// Only the VirtualServices with SslConfig should be aggregated on the Gateway
					Expect(matchedHttpListener.GetSslConfigurations()).To(HaveLen(len(snap.VirtualServices)))
				})

			})

		})

	})

})

func createVirtualService(name, namespace string, includeSsl bool) *v1.VirtualService {
	vs := &v1.VirtualService{
		Metadata: &core.Metadata{
			Name:      name,
			Namespace: namespace,
		},
		VirtualHost: &v1.VirtualHost{
			Domains: []string{fmt.Sprintf("%s.com", name)},
			Routes: []*v1.Route{
				{
					Matchers: []*matchers.Matcher{{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: fmt.Sprintf("/%s", name),
						},
					}},
					Action: &v1.Route_DirectResponseAction{
						DirectResponseAction: &gloov1.DirectResponseAction{
							Body: name,
						},
					},
				},
			},
		},
	}

	if includeSsl {
		vs.SslConfig = &gloov1.SslConfig{
			SniDomains: []string{fmt.Sprintf("%s.com", name)},
		}
	}

	return vs
}
