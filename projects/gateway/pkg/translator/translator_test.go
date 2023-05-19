package translator_test

import (
	"context"
	"net/http"

	"github.com/solo-io/gloo/test/helpers"

	"github.com/golang/protobuf/ptypes/wrappers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	. "github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/waf"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/als"
	"github.com/solo-io/gloo/test/samples"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	ns  = "gloo-system"
	ns2 = "gloo-system2"
)

var _ = Describe("Translator", func() {

	var (
		snap       *gloov1snap.ApiSnapshot
		translator Translator
	)

	Context("default GwTranslator", func() {

		BeforeEach(func() {
			translator = NewDefaultTranslator(Opts{
				WriteNamespace: ns,
			})
			snap = &gloov1snap.ApiSnapshot{
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
			proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, snap, snap.Gateways)

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

			proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, snap, snap.Gateways)

			Expect(errs).To(HaveLen(4))
			Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
			Expect(proxy).NotTo(BeNil())
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

			proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, snap, snap.Gateways)

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

			proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, snap, snap.Gateways)

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

			_, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, snap, snap.Gateways)
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

			snap := samples.GlooSnapshotWithDelegates(ns)
			rt := snap.RouteTables[0]
			rt.Routes = append(rt.Routes, badRoute)

			_, reports := translator.Translate(context.Background(), defaults.GatewayProxyName, snap, snap.Gateways)
			err := reports.Validate()
			Expect(err).NotTo(HaveOccurred())
			err = reports.ValidateStrict()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("route table exist.don't missing"))
		})

		Context("when the gateway CRDs don't clash", func() {
			BeforeEach(func() {
				translator = NewDefaultTranslator(Opts{
					WriteNamespace:                ns,
					ReadGatewaysFromAllNamespaces: true,
				})
				snap = &gloov1snap.ApiSnapshot{
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
				proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, snap, snap.Gateways)

				Expect(errs).To(HaveLen(4))
				Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
				Expect(proxy.Metadata.Name).To(Equal(defaults.GatewayProxyName))
				Expect(proxy.Metadata.Namespace).To(Equal(ns))
				Expect(proxy.Listeners).To(HaveLen(2))
			})
		})

		It("should error on gateway without gateway type", func() {
			gatewayWithoutType := &v1.Gateway{
				Metadata: &core.Metadata{
					Name:      "gateway-without-type",
					Namespace: ns,
				},
			}
			snap.Gateways = []*v1.Gateway{gatewayWithoutType}

			_, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, snap, snap.Gateways)
			err := errs.ValidateStrict()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(MissingGatewayTypeErr.Error()))
		})

		Context("TranslatorOpts", func() {

			var (
				httpGateway = &v1.Gateway{
					Metadata: &core.Metadata{
						Name:      "http-gateway",
						Namespace: ns,
					},
					GatewayType: &v1.Gateway_HttpGateway{
						HttpGateway: &v1.HttpGateway{},
					},
				}

				tcpGateway = &v1.Gateway{
					Metadata: &core.Metadata{
						Name:      "tcp-gateway",
						Namespace: ns,
					},
					GatewayType: &v1.Gateway_TcpGateway{
						TcpGateway: &v1.TcpGateway{},
					},
				}

				hybridGateway = &v1.Gateway{
					Metadata: &core.Metadata{
						Name:      "hybrid-gateway",
						Namespace: ns,
					},
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
				}
			)

			type listenerValidator func(l *gloov1.Listener)

			DescribeTable("IsolateVirtualHostsBySslConfig",
				func(gateway *v1.Gateway, globalSetting bool, annotation string, listenerValidator listenerValidator) {
					gwTranslator := NewDefaultTranslator(Opts{
						IsolateVirtualHostsBySslConfig: globalSetting,
						WriteNamespace:                 ns,
						ReadGatewaysFromAllNamespaces:  true,
					})

					// Apply the annotation, if provided
					annotatedGateway := gateway.Clone().(*v1.Gateway)
					if annotation != "" {
						annotatedGateway.Metadata.Annotations = map[string]string{
							IsolateVirtualHostsAnnotation: annotation,
						}
					}

					// Create the minimal snapshot necessary to produce a Proxy
					snapshot := &gloov1snap.ApiSnapshot{
						Gateways: v1.GatewayList{annotatedGateway},
						VirtualServices: v1.VirtualServiceList{
							helpers.NewVirtualServiceBuilder().
								WithName("vs").
								WithNamespace(ns).
								WithDomain("custom-domain").
								WithRoutePrefixMatcher("route", "/route").
								WithRouteDirectResponseAction("route", &gloov1.DirectResponseAction{
									Body:   "direct-response",
									Status: http.StatusOK,
								}).
								Build(),
						},
					}

					proxy, errs := gwTranslator.Translate(
						context.Background(),
						defaults.GatewayProxyName,
						snapshot,
						snapshot.Gateways)

					Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
					Expect(proxy.GetListeners()).To(HaveLen(1))
					listenerValidator(proxy.GetListeners()[0])
				},

				// HttpGateways
				Entry(
					"HttpGateway - false,no annotation", httpGateway, false, "",
					func(l *gloov1.Listener) {
						Expect(l.GetHttpListener()).NotTo(BeNil())
					},
				),
				Entry(
					"HttpGateway - true,no annotation", httpGateway, true, "",
					func(l *gloov1.Listener) {
						Expect(l.GetAggregateListener()).NotTo(BeNil())
					},
				),
				Entry(
					"HttpGateway - false,annotation override", httpGateway, false, "true",
					func(l *gloov1.Listener) {
						Expect(l.GetAggregateListener()).NotTo(BeNil())
					},
				),
				Entry(
					"HttpGateway - true,annotation override", httpGateway, true, "false",
					func(l *gloov1.Listener) {
						Expect(l.GetHttpListener()).NotTo(BeNil())
					},
				),

				// TcpGateway
				Entry(
					"TcpGateway - false,no annotation", tcpGateway, false, "",
					func(l *gloov1.Listener) {
						Expect(l.GetTcpListener()).NotTo(BeNil())
					},
				),
				Entry(
					"TcpGateway - true,no annotation", tcpGateway, true, "",
					func(l *gloov1.Listener) {
						Expect(l.GetTcpListener()).NotTo(BeNil())
					},
				),
				Entry(
					"TcpGateway - false,annotation override", tcpGateway, false, "true",
					func(l *gloov1.Listener) {
						Expect(l.GetTcpListener()).NotTo(BeNil())
					},
				),
				Entry(
					"TcpGateway - true,annotation override", tcpGateway, true, "false",
					func(l *gloov1.Listener) {
						Expect(l.GetTcpListener()).NotTo(BeNil())
					},
				),

				// HybridGateways
				Entry(
					"HybridGateway - false,no annotation", hybridGateway, false, "",
					func(l *gloov1.Listener) {
						Expect(l.GetHybridListener()).NotTo(BeNil())
					},
				),
				Entry(
					"HybridGateway - true,no annotation", hybridGateway, true, "",
					func(l *gloov1.Listener) {
						Expect(l.GetAggregateListener()).NotTo(BeNil())
					},
				),
				Entry(
					"HybridGateway - false,annotation override", hybridGateway, false, "true",
					func(l *gloov1.Listener) {
						Expect(l.GetAggregateListener()).NotTo(BeNil())
					},
				),
				Entry(
					"HybridGateway - true,annotation override", hybridGateway, true, "false",
					func(l *gloov1.Listener) {
						Expect(l.GetHybridListener()).NotTo(BeNil())
					},
				),
			)
		})

	})

})
