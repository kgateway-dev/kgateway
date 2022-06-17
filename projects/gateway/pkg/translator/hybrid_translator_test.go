package translator_test

import (
	"context"
	"fmt"
	"time"

	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"google.golang.org/protobuf/types/known/wrapperspb"

	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	. "github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	hcm "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tcp"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
)

var _ = Describe("Hybrid Translator", func() {

	var (
		ctx              context.Context
		cancel           context.CancelFunc
		hybridTranslator *HybridTranslator
		snap             *gloov1snap.ApiSnapshot
		reports          reporter.ResourceReports
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		hybridTranslator = &HybridTranslator{
			VirtualServiceTranslator: &VirtualServiceTranslator{
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
			snap = &gloov1snap.ApiSnapshot{
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
					snap = &gloov1snap.ApiSnapshot{
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
					snap = &gloov1snap.ApiSnapshot{
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

				snap = &gloov1snap.ApiSnapshot{
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

			Context("anscestry override logic", func() {
				var (
					parent   *v1.DelegatedHttpGateway
					child    *v1.MatchableHttpGateway
					ssl_true *gloov1.SslConfig = &gloov1.SslConfig{
						OneWayTls: &wrapperspb.BoolValue{
							Value: true,
						},
					}
					ssl_false *gloov1.SslConfig = &gloov1.SslConfig{
						OneWayTls: &wrapperspb.BoolValue{
							Value: false,
						},
					}
					ssl_empty *gloov1.SslConfig                  = &gloov1.SslConfig{}
					hcm_true  *hcm.HttpConnectionManagerSettings = &hcm.HttpConnectionManagerSettings{
						SkipXffAppend: true,
					}
					hcm_false *hcm.HttpConnectionManagerSettings = &hcm.HttpConnectionManagerSettings{
						SkipXffAppend: false,
					}
					hcm_empty *hcm.HttpConnectionManagerSettings = &hcm.HttpConnectionManagerSettings{}
				)

				BeforeEach(func() {
					snap = &gloov1snap.ApiSnapshot{
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
								HttpGateway: &v1.HttpGateway{
									Options: &gloov1.HttpListenerOptions{},
								},
							},
						},

						VirtualServices: v1.VirtualServiceList{
							createVirtualService("name1", ns, false),
							createVirtualService("name2", ns, false),
							createVirtualService("name3", ns+"-other-namespace", false),
						},
					}

					parent = snap.Gateways[0].GetHybridGateway().GetDelegatedHttpGateways()
					child = snap.HttpGateways[0]
				})

				DescribeTable("anscestry override tests", func(child_ssl *gloov1.SslConfig, parent_ssl *gloov1.SslConfig, child_hcm *hcm.HttpConnectionManagerSettings, parent_hcm *hcm.HttpConnectionManagerSettings, prevent_child_overrides bool) {
					parent.PreventChildOverrides = prevent_child_overrides
					// config setting
					child.GetMatcher().SslConfig = child_ssl
					parent.SslConfig = parent_ssl
					child.GetHttpGateway().GetOptions().HttpConnectionManagerSettings = child_hcm
					parent.HttpConnectionManagerSettings = parent_hcm

					// perform translation
					params := NewTranslatorParams(ctx, snap, reports)
					hybridTranslator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])

					// evaluate results
					hcm_after := child.GetHttpGateway().GetOptions().GetHttpConnectionManagerSettings()
					ssl_after := child.GetMatcher().GetSslConfig()

					Expect(hcm_after.GetSkipXffAppend()).To(Equal(true))
					Expect(ssl_after.GetOneWayTls().GetValue()).To(Equal(true))
				},
					Entry("PreventChildOverrides, child == nil", // should prefer parent fields
						nil, ssl_true, nil, hcm_true, true),
					Entry("PreventChildOverrides, parent == nil", // should prefer child fields
						ssl_true, nil, hcm_true, nil, true),
					Entry("PreventChildOverrides, child.subfield != nil && parent.subfield != nil", // should prefer parent.subfield
						ssl_false, ssl_true, hcm_false, hcm_true, true),
					Entry("PreventChildOverrides, child.subfield != nil && parent.subfield == nil", // should prefer child.subfield
						ssl_true, ssl_empty, hcm_true, hcm_empty, true),
					Entry("PreventChildOverrides, child.subfield == nil && parent.subfield != nil", // should prefer parent.subfield
						ssl_empty, ssl_true, hcm_empty, hcm_true, true),
					Entry("!PreventChildOverrides, child == nil", // should prefer parent fields
						nil, ssl_true, nil, hcm_true, false),
					Entry("!PreventChildOverrides, parent == nil", // should prefer child fields
						ssl_true, nil, hcm_true, nil, false),
					Entry("!PreventChildOverrides, child.subfield != nil && parent.subfield != nil", // should prefer child.subfield
						ssl_true, ssl_false, hcm_true, hcm_false, false),
					Entry("!PreventChildOverrides, child.subfield != nil && parent.subfield == nil", // should prefer child.subfield
						ssl_true, ssl_empty, hcm_true, hcm_empty, false),
					Entry("!PreventChildOverrides, child.subfield == nil && parent.subfield != nil", // should prefer parent.subfield
						ssl_empty, ssl_true, hcm_empty, hcm_true, false),
				)

				It("Should set child HCM options when child has `nil` options field", func() {
					// config setting
					child.GetHttpGateway().Options = nil
					parent.HttpConnectionManagerSettings = hcm_true

					// perform transformation
					params := NewTranslatorParams(ctx, snap, reports)
					hybridTranslator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])

					// evaluate results
					hcm_after := child.GetHttpGateway().GetOptions().GetHttpConnectionManagerSettings()
					Expect(hcm_after.GetSkipXffAppend()).To(Equal(true))
				})

				It("Should overwrite nested nil child fields", func() {
					// config setting
					child.GetMatcher().SslConfig = ssl_empty
					parent.SslConfig = ssl_empty
					parent.GetSslConfig().TransportSocketConnectTimeout = &duration.Duration{
						Seconds: 10,
					}

					// perform transformation
					params := NewTranslatorParams(ctx, snap, reports)
					hybridTranslator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])

					// evaluate results
					ssl_after := child.GetMatcher().GetSslConfig()
					Expect(ssl_after.GetTransportSocketConnectTimeout().GetSeconds()).To(Equal(int64(10)))
				})
			})

			Context("non-ssl", func() {

				BeforeEach(func() {
					snap = &gloov1snap.ApiSnapshot{
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
					snap = &gloov1snap.ApiSnapshot{
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
