package translator_test

import (
	"context"
	"fmt"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/selectors"

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
					parent  *v1.DelegatedHttpGateway
					child   *v1.MatchableHttpGateway
					sslTrue *gloov1.SslConfig = &gloov1.SslConfig{
						OneWayTls: &wrapperspb.BoolValue{
							Value: true,
						},
					}

					// sslEmpty *gloov1.SslConfig                  = &gloov1.SslConfig{}
					hcmTrue *hcm.HttpConnectionManagerSettings = &hcm.HttpConnectionManagerSettings{
						SkipXffAppend: &wrapperspb.BoolValue{
							Value: true,
						},
					}

					sslNil  *gloov1.SslConfig
					sslZero = &gloov1.SslConfig{
						OneWayTls: &wrapperspb.BoolValue{
							Value: false,
						},
					}
					sslEmpty *gloov1.SslConfig = &gloov1.SslConfig{}
					sslSet   *gloov1.SslConfig = &gloov1.SslConfig{
						OneWayTls: &wrapperspb.BoolValue{
							Value: true,
						},
					}
				)

				type DesiredResult int64

				const (
					False DesiredResult = iota
					True
					Nil
					Error
					EmptyString
					StringA
					StringB
				)

				BeforeEach(func() {
					snap = &gloov1snap.ApiSnapshot{
						Gateways: v1.GatewayList{
							{
								Metadata: &core.Metadata{Namespace: ns, Name: "name"},
								GatewayType: &v1.Gateway_HybridGateway{
									HybridGateway: &v1.HybridGateway{
										DelegatedHttpGateways: &v1.DelegatedHttpGateway{
											SelectionType: &v1.DelegatedHttpGateway_Selector{
												Selector: &selectors.Selector{
													Namespaces: []string{ns},
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
							createVirtualService("name1", ns, true),
						},
					}

					parent = snap.Gateways[0].GetHybridGateway().GetDelegatedHttpGateways()
					child = snap.HttpGateways[0]
				})

				DescribeTable("SslConfig anscestry override tests", func(childSsl, parentSsl *gloov1.SslConfig, preventChildOverrides bool, desiredResult DesiredResult) {
					parent.PreventChildOverrides = preventChildOverrides
					// config setting
					child.GetMatcher().SslConfig = childSsl
					parent.SslConfig = parentSsl

					// perform translation
					params := NewTranslatorParams(ctx, snap, reports)
					listener := hybridTranslator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])

					if desiredResult == Error {
						// evaluate results
						Expect(reports.ValidateStrict()).To(HaveOccurred())
						Expect(listener).To(BeNil())
						return
					} else {
						// evaluate results
						Expect(reports.ValidateStrict()).NotTo(HaveOccurred())
						Expect(listener).NotTo(BeNil())
					}

					matchedListeners := listener.GetHybridListener().GetMatchedListeners()
					Expect(matchedListeners).To(HaveLen(1))

					singleMatchedListener := matchedListeners[0]
					Expect(singleMatchedListener).NotTo(BeNil())

					sslAfter := singleMatchedListener.GetMatcher().GetSslConfig()

					// assertion ladder is a bit annoying, given that I needed a custom type that could be one of [true, false, nil]
					if desiredResult == Nil {
						Expect(sslAfter.GetOneWayTls()).To(BeNil())
					} else if desiredResult == True {
						Expect(sslAfter.GetOneWayTls().GetValue()).To(BeTrue())
					} else if desiredResult == False {
						Expect(sslAfter.GetOneWayTls().GetValue()).To(BeFalse())
					}
				},
					/*┏(･o･)┛♪┗ (･o･) ┓			┏(･o･)┛♪┗ (･o･) ┓			┏(･o･)┛♪┗ (･o･) ┓			┏(･o･)┛♪┗ (･o･) ┓			┏(･o･)┛♪┗ (･o･) ┓
					I am sitting here, possibly having descended into madness.  In my madness, I have found it necessary/expedient to construct a 36-entry table.
					So some explanations are in order.  In general, a field can be affected in any 1 of 4 ways:
						* nil		| an object not existing/instantiated.							|	ssl = nil
						* zero		| a zero-value being set on a subfield.  ex: (0, "", false)		|	ssl.OneWayTls.Value = false
						* empty 	| object exists, but has nil subfield.							|	ssl.OneWayTls = nil
						* set		| object exists and has populated subfield						|	ssl.OneWayTls.Value = true
					Since our merge operations take in (childSsl, parentSsl, PreventChildOverrides), this leaves 4*4*2 = 36 possible combinations to test.

					When performing all of said combinations on a _nested_ field, such as ssl.OneWayTls.Value, there are 4 possible results:
						* True		| ssl.OneWayTls.Value is true
						* False		| ssl.OneWayTls.Value is false
						* Nil		| ssl or OneWayTls is nil
						* Error		| translation failed; see "anscestry override tests for mismatched ssl definitions" for exact details
					(┏(･o･)┛♪┗ (･o･) ┓			┏(･o･)┛♪┗ (･o･) ┓			┏(･o･)┛♪┗ (･o･) ┓			┏(･o･)┛♪┗ (･o･) ┓			┏(･o･)┛♪┗ (･o･) ┓)*/
					Entry("nil,nil,1", sslNil, sslNil, true, Nil),
					Entry("nil,nil,0", sslNil, sslNil, false, Nil),
					Entry("nil,zero,1", sslNil, sslZero, true, Error),
					Entry("nil,zero,0", sslNil, sslZero, false, Error),
					Entry("nil,empty,1", sslNil, sslEmpty, true, Error),
					Entry("nil,empty,0", sslNil, sslEmpty, false, Error),
					Entry("nil,set,1", sslNil, sslSet, true, Error),
					Entry("nil,set,0", sslNil, sslSet, false, Error),

					Entry("zero,nil,1", sslZero, sslNil, true, Error),
					Entry("zero,nil,0", sslZero, sslNil, false, Error),
					Entry("zero,zero,1", sslZero, sslZero, true, False),
					Entry("zero,zero,0", sslZero, sslZero, false, False),
					Entry("zero,empty,1", sslZero, sslEmpty, true, False),
					Entry("zero,empty,0", sslZero, sslEmpty, false, False),
					Entry("zero,set,1", sslZero, sslSet, true, True),
					Entry("zero,set,0", sslZero, sslSet, false, True),

					Entry("empty,nil,1", sslEmpty, sslNil, true, Error),
					Entry("empty,nil,0", sslEmpty, sslNil, false, Error),
					Entry("empty,zero,1", sslEmpty, sslZero, true, False),
					Entry("empty,zero,0", sslEmpty, sslZero, false, False),
					Entry("empty,empty,1", sslEmpty, sslEmpty, true, Nil),
					Entry("empty,empty,0", sslEmpty, sslEmpty, false, Nil),
					Entry("empty,set,1", sslEmpty, sslSet, true, True),
					Entry("empty,set,0", sslEmpty, sslSet, false, True),

					Entry("set,nil,1", sslSet, sslNil, true, Error),
					Entry("set,nil,0", sslSet, sslNil, false, Error),
					Entry("set,zero,1", sslSet, sslZero, true, True),
					Entry("set,zero,0", sslSet, sslZero, false, True),
					Entry("set,empty,1", sslSet, sslEmpty, true, True),
					Entry("set,empty,0", sslSet, sslEmpty, false, True),
					Entry("set,set,1", sslSet, sslSet, true, True),
					Entry("set,set,0", sslSet, sslSet, false, True),
				)

				DescribeTable("anscestry override tests for mismatched ssl definitions", func(childSsl *gloov1.SslConfig, parentSsl *gloov1.SslConfig, childHcm *hcm.HttpConnectionManagerSettings, parentHcm *hcm.HttpConnectionManagerSettings, preventChildOverrides bool) {
					// In this test we configure Gateway and MatchableHttpGateway with mismatched SslConfigs
					// that is, one has ssl defined and the other does not
					// that should not generate a Listener and instead yield an error on the report

					parent.PreventChildOverrides = preventChildOverrides
					// config setting
					child.GetMatcher().SslConfig = childSsl
					parent.SslConfig = parentSsl
					child.GetHttpGateway().GetOptions().HttpConnectionManagerSettings = childHcm
					parent.HttpConnectionManagerSettings = parentHcm

					// perform translation
					params := NewTranslatorParams(ctx, snap, reports)
					listener := hybridTranslator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])

					// evaluate results
					Expect(reports.ValidateStrict()).To(HaveOccurred())
					Expect(listener).To(BeNil())
				},
					Entry("PreventChildOverrides, child == nil", // should prefer parent fields
						nil, sslTrue, nil, hcmTrue, true),
					Entry("PreventChildOverrides, parent == nil", // should prefer child fields
						sslTrue, nil, hcmTrue, nil, true),
					Entry("!PreventChildOverrides, child == nil", // should prefer parent fields
						nil, sslTrue, nil, hcmTrue, false),
					Entry("!PreventChildOverrides, parent == nil", // should prefer child fields
						sslTrue, nil, hcmTrue, nil, false),
				)

				It("Should set child HCM options when child has `nil` options field", func() {
					// config setting
					child.GetHttpGateway().Options = nil
					parent.HttpConnectionManagerSettings = hcmTrue

					// perform transformation
					params := NewTranslatorParams(ctx, snap, reports)
					listener := hybridTranslator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])

					// evaluate results
					Expect(listener).NotTo(BeNil())

					matchedListeners := listener.GetHybridListener().GetMatchedListeners()
					Expect(matchedListeners).To(HaveLen(1))

					singleMatchedListener := matchedListeners[0]
					Expect(singleMatchedListener).NotTo(BeNil())

					hcmAfter := singleMatchedListener.GetHttpListener().GetOptions().GetHttpConnectionManagerSettings()
					Expect(hcmAfter.GetSkipXffAppend().GetValue()).To(Equal(true))
				})

				It("Should overwrite nested nil child fields", func() {
					// config setting
					child.GetMatcher().SslConfig = sslEmpty
					parent.SslConfig = sslEmpty
					parent.GetSslConfig().TransportSocketConnectTimeout = &duration.Duration{
						Seconds: 10,
					}

					// perform transformation
					params := NewTranslatorParams(ctx, snap, reports)
					listener := hybridTranslator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])

					// evaluate results
					Expect(listener).NotTo(BeNil())

					matchedListeners := listener.GetHybridListener().GetMatchedListeners()
					Expect(matchedListeners).To(HaveLen(1))

					singleMatchedListener := matchedListeners[0]
					Expect(singleMatchedListener).NotTo(BeNil())

					sslAfter := singleMatchedListener.GetMatcher().GetSslConfig()
					Expect(sslAfter.GetTransportSocketConnectTimeout().GetSeconds()).To(Equal(int64(10)))
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
											// This is important as it means the Gateway will only select
											// HttpGateways with Ssl defined
											SslConfig: &gloov1.SslConfig{},
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
