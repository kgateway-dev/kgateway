package translator_test

import (
	"context"
	"time"

	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"

	gloo_matchers "github.com/solo-io/solo-kit/test/matchers"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/hashicorp/go-multierror"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	. "github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
)

var _ = Describe("Http Translator", func() {

	// This file contains the tests both for the HttpTranslator and VirtualServiceTranslator
	// It would be ideal to split the VirtualServiceTranslator tests into a distinct file

	var (
		ctx        context.Context
		cancel     context.CancelFunc
		translator *HttpTranslator
		snap       *gloov1snap.ApiSnapshot
		reports    reporter.ResourceReports

		labelSet = map[string]string{"a": "b"}
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		translator = &HttpTranslator{
			VirtualServiceTranslator: &VirtualServiceTranslator{
				WarnOnRouteShortCircuiting: false,
			},
		}
	})

	AfterEach(func() {
		cancel()
	})

	initializeReportsForSnap := func() {
		reports = make(reporter.ResourceReports)
		reports.Accept(snap.Gateways.AsInputResources()...)
		reports.Accept(snap.VirtualServices.AsInputResources()...)
		reports.Accept(snap.RouteTables.AsInputResources()...)
	}

	Context("all-in-one virtual service", func() {

		BeforeEach(func() {
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
			initializeReportsForSnap()
		})

		It("should translate an empty gateway to have all virtual services", func() {
			params := NewTranslatorParams(ctx, snap, reports)

			listener := translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
			Expect(listener).NotTo(BeNil())

			httpListener := listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
			Expect(httpListener.VirtualHosts).To(HaveLen(len(snap.VirtualServices)))
		})

		It("omitting matchers should default to '/' prefix matcher", func() {
			snap.VirtualServices[0].VirtualHost.Routes[0].Matchers = nil
			snap.VirtualServices[1].VirtualHost.Routes[0].Matchers = []*matchers.Matcher{}
			params := NewTranslatorParams(ctx, snap, reports)

			listener := translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
			Expect(listener).NotTo(BeNil())

			httpListener := listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
			Expect(httpListener.VirtualHosts).To(HaveLen(len(snap.VirtualServices)))
			Expect(httpListener.VirtualHosts[0].Routes[0].Matchers).To(HaveLen(1))
			Expect(httpListener.VirtualHosts[1].Routes[0].Matchers).To(HaveLen(1))
			Expect(httpListener.VirtualHosts[0].Routes[0].Matchers[0]).To(Equal(defaults.DefaultMatcher()))
			Expect(httpListener.VirtualHosts[1].Routes[0].Matchers[0]).To(Equal(defaults.DefaultMatcher()))
		})

		It("should have no ssl config", func() {
			params := NewTranslatorParams(ctx, snap, reports)

			listener := translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
			Expect(listener).NotTo(BeNil())
			Expect(listener.SslConfigurations).To(BeEmpty())
		})

		Context("with VirtualServices (refs)", func() {

			It("should translate a gateway to only have its virtual services", func() {
				snap.Gateways[0].GatewayType = &v1.Gateway_HttpGateway{
					HttpGateway: &v1.HttpGateway{
						VirtualServices: []*core.ResourceRef{snap.VirtualServices[0].Metadata.Ref()},
					},
				}
				params := NewTranslatorParams(ctx, snap, reports)

				listener := translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				Expect(listener).NotTo(BeNil())
				Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

				httpListener := listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
				Expect(httpListener.VirtualHosts).To(HaveLen(1))
			})

			It("can include a virtual service from some other namespace", func() {
				snap.Gateways[0].GatewayType = &v1.Gateway_HttpGateway{
					HttpGateway: &v1.HttpGateway{
						VirtualServices: []*core.ResourceRef{snap.VirtualServices[2].Metadata.Ref()},
					},
				}
				params := NewTranslatorParams(ctx, snap, reports)

				listener := translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				Expect(listener).NotTo(BeNil())
				Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

				httpListener := listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
				Expect(httpListener.VirtualHosts).To(HaveLen(1))
				Expect(httpListener.VirtualHosts[0].Domains).To(Equal(snap.VirtualServices[2].VirtualHost.Domains))
			})
		})

		Context("with VirtualServiceSelector", func() {

			It("should translate a gateway to only have its virtual services", func() {
				snap.Gateways[0].GatewayType = &v1.Gateway_HttpGateway{
					HttpGateway: &v1.HttpGateway{
						VirtualServiceSelector: labelSet,
					},
				}
				params := NewTranslatorParams(ctx, snap, reports)

				listener := translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				Expect(listener).NotTo(BeNil())
				Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

				httpListener := listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
				Expect(httpListener.VirtualHosts).To(HaveLen(2))
			})

			It("should translate a gateway to only have virtual services that match the provided expressions", func() {
				labelSuperSet := map[string]string{"a": "b", "extraLabel": "andValue"}
				vs := &v1.VirtualService{
					Metadata: &core.Metadata{Namespace: ns, Name: "name1", Labels: labelSuperSet},
					VirtualHost: &v1.VirtualHost{
						Domains: []string{"d4.com"},
						Routes: []*v1.Route{
							{
								Matchers: []*matchers.Matcher{{
									PathSpecifier: &matchers.Matcher_Prefix{
										Prefix: "/4",
									},
								}},
								Action: &v1.Route_DirectResponseAction{
									DirectResponseAction: &gloov1.DirectResponseAction{
										Body: "d4",
									},
								},
							},
						},
					},
				}

				snap.VirtualServices = append(snap.VirtualServices, vs)
				snap.Gateways[0].GatewayType = &v1.Gateway_HttpGateway{
					HttpGateway: &v1.HttpGateway{
						VirtualServiceExpressions: &v1.VirtualServiceSelectorExpressions{
							Expressions: []*v1.VirtualServiceSelectorExpressions_Expression{
								{
									Key:      "a",
									Operator: v1.VirtualServiceSelectorExpressions_Expression_In,
									Values:   []string{"b"},
								},
								{
									Key:      "extraLabel",
									Operator: v1.VirtualServiceSelectorExpressions_Expression_In,
									Values:   []string{"andValue"},
								},
							},
						},
					},
				}
				params := NewTranslatorParams(ctx, snap, reports)

				listener := translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				Expect(listener).NotTo(BeNil())
				Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

				httpListener := listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
				Expect(httpListener.VirtualHosts).To(HaveLen(1))
			})

			It("should not select using both labels and expressions", func() {
				snap.Gateways[0].GatewayType = &v1.Gateway_HttpGateway{
					HttpGateway: &v1.HttpGateway{
						VirtualServiceExpressions: &v1.VirtualServiceSelectorExpressions{
							Expressions: []*v1.VirtualServiceSelectorExpressions_Expression{},
						},
						VirtualServiceSelector: labelSet,
					},
				}
				params := NewTranslatorParams(ctx, snap, reports)

				listener := translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				Expect(listener).NotTo(BeNil())
				Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

				httpListener := listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
				Expect(httpListener.VirtualHosts).To(HaveLen(3))
			})

			It("should prevent a gateway from matching virtual services outside its own namespace if so configured", func() {
				snap.Gateways[0].GatewayType = &v1.Gateway_HttpGateway{
					HttpGateway: &v1.HttpGateway{
						VirtualServiceSelector:   labelSet,
						VirtualServiceNamespaces: []string{"gloo-system"},
					},
				}
				params := NewTranslatorParams(ctx, snap, reports)

				listener := translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				Expect(listener).NotTo(BeNil())
				Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

				httpListener := listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
				Expect(httpListener.VirtualHosts).To(HaveLen(1))
				Expect(httpListener.VirtualHosts[0].Domains).To(Equal(snap.VirtualServices[0].VirtualHost.Domains))
			})

		})

		Context("default virtual service oneWayTls from Settings", func() {

			It("Virtual services one way tls defaults to true", func() {
				settings := &gloov1.Settings{
					Gateway: &gloov1.GatewayOptions{
						VirtualServiceOptions: &gloov1.VirtualServiceOptions{
							OneWayTls: &wrappers.BoolValue{
								Value: true,
							},
						},
					},
				}
				ctx := settingsutil.WithSettings(context.Background(), settings)
				snap.Gateways[0].Ssl = true
				snap.VirtualServices[0].SslConfig = new(ssl.SslConfig)
				snap.VirtualServices = append(snap.VirtualServices, &v1.VirtualService{
					SslConfig: &ssl.SslConfig{
						OneWayTls: &wrappers.BoolValue{
							Value: false,
						},
					},
					Metadata: &core.Metadata{
						Name:      "test",
						Namespace: "gloo-system",
					},
					VirtualHost: &v1.VirtualHost{},
				})
				params := NewTranslatorParams(ctx, snap, reports)

				listener := translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				Expect(listener).NotTo(BeNil())
				Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

				sslConfigs := listener.GetSslConfigurations()
				Expect(sslConfigs).To(HaveLen(2))
				// This sslConfig derives from the first virtual service, which we did not explicitly set oneWayTls on, so it
				// should inherit from the VirtualServiceOptions default value in the settings
				Expect(sslConfigs[0].GetOneWayTls().GetValue()).To(BeTrue())
				// We explicitly set the second virtual service oneWayTls, so that should not be overidden by default
				Expect(sslConfigs[1].GetOneWayTls().GetValue()).To(BeFalse())
			})

			It("Virtual services one way tls defaults to false", func() {
				settings := &gloov1.Settings{
					Gateway: &gloov1.GatewayOptions{
						VirtualServiceOptions: &gloov1.VirtualServiceOptions{
							OneWayTls: &wrappers.BoolValue{
								Value: false,
							},
						},
					},
				}
				ctx := settingsutil.WithSettings(context.Background(), settings)
				snap.Gateways[0].Ssl = true
				snap.VirtualServices[0].SslConfig = new(ssl.SslConfig)
				snap.VirtualServices = append(snap.VirtualServices, &v1.VirtualService{
					SslConfig: &ssl.SslConfig{
						OneWayTls: &wrappers.BoolValue{
							Value: true,
						},
					},
					Metadata: &core.Metadata{
						Name:      "test",
						Namespace: "gloo-system",
					},
					VirtualHost: &v1.VirtualHost{},
				})
				params := NewTranslatorParams(ctx, snap, reports)

				listener := translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				Expect(listener).NotTo(BeNil())
				Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

				sslConfigs := listener.GetSslConfigurations()
				Expect(sslConfigs).To(HaveLen(2))
				// This sslConfig derives from the first virtual service, which we did not explicitly set oneWayTls on, so it
				// should inherit from the VirtualServiceOptions default value in the settings
				Expect(sslConfigs[0].GetOneWayTls().GetValue()).To(BeFalse())
				// We explicitly set the second virtual service oneWayTls, so that should not be overidden by default
				Expect(sslConfigs[1].GetOneWayTls().GetValue()).To(BeTrue())
			})

			It("No default set", func() {
				settings := &gloov1.Settings{
					Gateway: &gloov1.GatewayOptions{
						VirtualServiceOptions: &gloov1.VirtualServiceOptions{},
					},
				}
				ctx := settingsutil.WithSettings(context.Background(), settings)
				snap.Gateways[0].Ssl = true
				snap.VirtualServices[0].SslConfig = new(ssl.SslConfig)
				snap.VirtualServices = append(snap.VirtualServices, &v1.VirtualService{
					SslConfig: &ssl.SslConfig{
						OneWayTls: &wrappers.BoolValue{
							Value: true,
						},
					},
					Metadata: &core.Metadata{
						Name:      "test",
						Namespace: "gloo-system",
					},
					VirtualHost: &v1.VirtualHost{},
				})
				params := NewTranslatorParams(ctx, snap, reports)

				listener := translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				Expect(listener).NotTo(BeNil())
				Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

				sslConfigs := listener.GetSslConfigurations()
				Expect(sslConfigs).To(HaveLen(2))
				// We do not expect oneWayTls on the first virtual service, so it should be false
				Expect(sslConfigs[0].GetOneWayTls().GetValue()).To(BeFalse())
				// We explicitly set the second virtual service oneWayTls
				Expect(sslConfigs[1].GetOneWayTls().GetValue()).To(BeTrue())
			})

		})

		It("should not have vhosts with ssl", func() {
			snap.VirtualServices[0].SslConfig = new(ssl.SslConfig)
			params := NewTranslatorParams(ctx, snap, reports)

			listener := translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
			Expect(listener).NotTo(BeNil())
			Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

			var vsWithoutSsl v1.VirtualServiceList
			for _, vs := range snap.VirtualServices {
				if vs.SslConfig == nil {
					vsWithoutSsl = append(vsWithoutSsl, vs)
				}
			}

			httpListener := listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
			Expect(httpListener.VirtualHosts).To(HaveLen(len(vsWithoutSsl)))
			Expect(httpListener.VirtualHosts[0].Name).To(ContainSubstring("name2"))
			Expect(httpListener.VirtualHosts[1].Name).To(ContainSubstring("name3"))
		})

		It("should not have vhosts without ssl", func() {
			snap.Gateways[0].Ssl = true
			snap.VirtualServices[0].SslConfig = new(ssl.SslConfig)
			params := NewTranslatorParams(ctx, snap, reports)

			listener := translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
			Expect(listener).NotTo(BeNil())
			Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

			httpListener := listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
			Expect(httpListener.VirtualHosts).To(HaveLen(1))
			Expect(httpListener.VirtualHosts[0].Name).To(ContainSubstring("name1"))
		})

		Context("validate domains", func() {

			BeforeEach(func() {
				snap.VirtualServices[1].VirtualHost.Domains = snap.VirtualServices[0].VirtualHost.Domains
			})

			It("should error when 2 virtual services linked to the same gateway have overlapping domains", func() {
				params := NewTranslatorParams(ctx, snap, reports)

				listener := translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				Expect(listener).NotTo(BeNil())
				errs := reports.ValidateStrict()
				Expect(errs).To(HaveOccurred())

				multiErr, ok := errs.(*multierror.Error)
				Expect(ok).To(BeTrue())

				for _, expectedError := range []error{
					DomainInOtherVirtualServicesErr("d1.com", []string{"gloo-system.name1"}),
					DomainInOtherVirtualServicesErr("d1.com", []string{"gloo-system.name2"}),
					GatewayHasConflictingVirtualServicesErr([]string{"d1.com"}),
				} {
					Expect(multiErr.WrappedErrors()).To(ContainElement(testutils.HaveInErrorChain(expectedError)))
				}
			})

			It("should error when 2 virtual services linked to the same gateway have empty domains", func() {
				snap.VirtualServices[1].VirtualHost.Domains = nil
				snap.VirtualServices[0].VirtualHost.Domains = nil
				params := NewTranslatorParams(ctx, snap, reports)

				_ = translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				errs := reports.ValidateStrict()

				multiErr, ok := errs.(*multierror.Error)
				Expect(ok).To(BeTrue())

				for _, expectedError := range []error{
					DomainInOtherVirtualServicesErr("", []string{"gloo-system.name1"}),
					DomainInOtherVirtualServicesErr("", []string{"gloo-system.name2"}),
					GatewayHasConflictingVirtualServicesErr([]string{""}),
				} {
					Expect(multiErr.WrappedErrors()).To(ContainElement(testutils.HaveInErrorChain(expectedError)))
				}
			})

			It("should warn when a virtual service does not specify a virtual host", func() {
				snap.VirtualServices[0].VirtualHost = nil
				params := NewTranslatorParams(ctx, snap, reports)

				_ = translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				Expect(reports.Validate()).NotTo(HaveOccurred())

				errs := reports.ValidateStrict()
				Expect(errs).To(HaveOccurred())
				Expect(errs.Error()).To(ContainSubstring(NoVirtualHostErr(snap.VirtualServices[0]).Error()))
			})

			It("should error when a virtual service has invalid regex", func() {
				snap.VirtualServices[0].VirtualHost.Routes[0].Matchers[0] = &matchers.Matcher{PathSpecifier: &matchers.Matcher_Regex{Regex: "["}}
				params := NewTranslatorParams(ctx, snap, reports)

				_ = translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				Expect(reports.Validate()).To(HaveOccurred())

				errs := reports.ValidateStrict()
				Expect(errs).To(HaveOccurred())
				Expect(errs.Error()).To(ContainSubstring("missing closing ]: `[`"))
			})
			It("should error when a virtual service has invalid regex in host rewrite options", func() {
				hostRegexRewriteOption := &gloov1.RouteOptions{HostRewriteType: &gloov1.RouteOptions_HostRewritePathRegex{HostRewritePathRegex: &v3.RegexMatchAndSubstitute{Pattern: &v3.RegexMatcher{Regex: "["}}}}
				snap.VirtualServices[0].VirtualHost.Routes[0].Options = hostRegexRewriteOption
				params := NewTranslatorParams(ctx, snap, reports)

				_ = translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				Expect(reports.Validate()).To(HaveOccurred())

				errs := reports.ValidateStrict()
				Expect(errs).To(HaveOccurred())
				Expect(errs.Error()).To(ContainSubstring("missing closing ]: `[`"))
			})
			It("should error when a virtual service has invalid regex in regexRewrite options", func() {
				regexRewriteOption := &gloov1.RouteOptions{RegexRewrite: &v3.RegexMatchAndSubstitute{Pattern: &v3.RegexMatcher{Regex: "["}}}
				snap.VirtualServices[0].VirtualHost.Routes[0].Options = regexRewriteOption
				params := NewTranslatorParams(ctx, snap, reports)

				_ = translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				Expect(reports.Validate()).To(HaveOccurred())

				errs := reports.ValidateStrict()
				Expect(errs).To(HaveOccurred())
				Expect(errs.Error()).To(ContainSubstring("missing closing ]: `[`"))
			})
		})

		Context("validate matcher short-circuiting warnings", func() {

			BeforeEach(func() {
				translator = &HttpTranslator{
					VirtualServiceTranslator: &VirtualServiceTranslator{
						WarnOnRouteShortCircuiting: true,
					},
				}
			})

			DescribeTable("warns on route short-circuiting", func(earlyMatcher, lateMatcher *matchers.Matcher, expectedErr error) {
				vs := &v1.VirtualService{
					Metadata: &core.Metadata{Namespace: ns, Name: "name1", Labels: labelSet},
					VirtualHost: &v1.VirtualHost{
						Domains: []string{"d1.com"},
						Routes: []*v1.Route{
							{
								Matchers: []*matchers.Matcher{earlyMatcher},
								Action: &v1.Route_DirectResponseAction{
									DirectResponseAction: &gloov1.DirectResponseAction{
										Body: "d1",
									},
								},
							},
							// second route will be short-circuited by the first one
							{
								Matchers: []*matchers.Matcher{lateMatcher},
								Action: &v1.Route_DirectResponseAction{
									DirectResponseAction: &gloov1.DirectResponseAction{
										Body: "d2",
									},
								},
							},
						},
					},
				}

				snap.VirtualServices = v1.VirtualServiceList{vs}
				params := NewTranslatorParams(ctx, snap, reports)

				_ = translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				errs := reports.ValidateStrict()
				if expectedErr == nil {
					Expect(errs).ToNot(HaveOccurred())
					return
				}
				Expect(errs).To(HaveOccurred())

				multiErr, ok := errs.(*multierror.Error)
				Expect(ok).To(BeTrue())

				Expect(multiErr.ErrorOrNil()).To(MatchError(ContainSubstring(expectedErr.Error())))
			},
				Entry("duplicate matchers",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}},
					ConflictingMatcherErr("gloo-system.name1", &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}})),
				Entry("duplicate paths but earlier has query parameter matcher",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, QueryParameters: []*matchers.QueryParameterMatcher{{Name: "foo", Value: "bar"}}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}},
					nil),
				Entry("duplicate paths but later has query parameter matcher (prefix hijacking)",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, QueryParameters: []*matchers.QueryParameterMatcher{{Name: "foo", Value: "bar"}}},
					UnorderedPrefixErr("gloo-system.name1", "/1", &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, QueryParameters: []*matchers.QueryParameterMatcher{{Name: "foo", Value: "bar"}}})),
				Entry("duplicate paths but earlier has header matcher",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Headers: []*matchers.HeaderMatcher{{Name: "foo", Value: "bar"}}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}},
					nil),
				Entry("duplicate paths but later has header matcher (prefix hijacking)",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Headers: []*matchers.HeaderMatcher{{Name: "foo", Value: "bar"}}},
					UnorderedPrefixErr("gloo-system.name1", "/1", &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Headers: []*matchers.HeaderMatcher{{Name: "foo", Value: "bar"}}})),
				Entry("duplicate paths but earlier has query parameter matchers that don't short circuit the latter",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, QueryParameters: []*matchers.QueryParameterMatcher{{Name: "foo", Value: "bar"}}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, QueryParameters: []*matchers.QueryParameterMatcher{{Name: "foo", Value: "baz"}}},
					nil),
				Entry("duplicate paths but earlier has query parameter matchers that don't short circuit the latter, with extras first",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, QueryParameters: []*matchers.QueryParameterMatcher{{Name: "foo", Value: "bar"}, {Name: "foo2", Value: "bar2"}}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, QueryParameters: []*matchers.QueryParameterMatcher{{Name: "foo", Value: "bar"}}},
					nil),
				Entry("duplicate paths but earlier has query parameter matchers that don't short circuit the latter, with extras second",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, QueryParameters: []*matchers.QueryParameterMatcher{{Name: "foo", Value: "bar"}}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, QueryParameters: []*matchers.QueryParameterMatcher{{Name: "foo", Value: "bar"}, {Name: "foo2", Value: "bar2"}}},
					UnorderedPrefixErr("gloo-system.name1", "/1", &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, QueryParameters: []*matchers.QueryParameterMatcher{{Name: "foo", Value: "bar"}, {Name: "foo2", Value: "bar2"}}})),
				Entry("duplicate paths but earlier has header matchers that don't short circuit the latter",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Headers: []*matchers.HeaderMatcher{{Name: "foo", Value: "bar"}}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Headers: []*matchers.HeaderMatcher{{Name: "foo", Value: "baz"}}},
					nil),
				Entry("duplicate paths but earlier has header matchers that don't short circuit the latter, with extras first",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Headers: []*matchers.HeaderMatcher{{Name: "foo", Value: "bar"}, {Name: "foo2", Value: "bar2"}}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Headers: []*matchers.HeaderMatcher{{Name: "foo", Value: "bar"}}},
					nil),
				Entry("duplicate paths but earlier has header matchers that don't short circuit the latter, with extras second",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Headers: []*matchers.HeaderMatcher{{Name: "foo", Value: "bar"}}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Headers: []*matchers.HeaderMatcher{{Name: "foo", Value: "bar"}, {Name: "foo2", Value: "bar2"}}},
					UnorderedPrefixErr("gloo-system.name1", "/1", &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Headers: []*matchers.HeaderMatcher{{Name: "foo", Value: "bar"}, {Name: "foo2", Value: "bar2"}}})),
				Entry("prefix hijacking",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1/2"}},
					UnorderedPrefixErr("gloo-system.name1", "/1", &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1/2"}})),
				Entry("prefix hijacking with inverted header matcher, late matcher unreachable",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Headers: []*matchers.HeaderMatcher{{Name: ":method", Value: "GET", InvertMatch: true}}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Headers: []*matchers.HeaderMatcher{{Name: ":method", Value: "POST"}}},
					UnorderedPrefixErr("gloo-system.name1", "/1", &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Headers: []*matchers.HeaderMatcher{{Name: ":method", Value: "POST"}}})),
				Entry("prefix hijacking with inverted header matcher, late matcher reachable",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Headers: []*matchers.HeaderMatcher{{Name: ":method", Value: "GET", InvertMatch: true}}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Headers: []*matchers.HeaderMatcher{{Name: ":method", Value: "GET"}}},
					nil),
				Entry("regex hijacking",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Regex{Regex: "/foo/.*/bar"}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo/user/info/bar"}},
					UnorderedRegexErr("gloo-system.name1", "/foo/.*/bar", &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo/user/info/bar"}})),
				Entry("regex hijacking - with match all header matcher",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Regex{Regex: "/foo/.*/bar"}, Headers: []*matchers.HeaderMatcher{{Name: "foo", Value: ""}}}, // empty value will match anything
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo/user/info/bar"}, Headers: []*matchers.HeaderMatcher{{Name: "foo", Value: "bar"}}},
					UnorderedRegexErr("gloo-system.name1", "/foo/.*/bar", &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo/user/info/bar"}, Headers: []*matchers.HeaderMatcher{{Name: "foo", Value: "bar"}}})),
				Entry("regex hijacking - with match all query parameter matcher",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Regex{Regex: "/foo/.*/bar"}, QueryParameters: []*matchers.QueryParameterMatcher{{Name: "foo", Value: ""}}}, // empty value will match anything
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo/user/info/bar"}, QueryParameters: []*matchers.QueryParameterMatcher{{Name: "foo", Value: "bar"}}},
					UnorderedRegexErr("gloo-system.name1", "/foo/.*/bar", &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo/user/info/bar"}, QueryParameters: []*matchers.QueryParameterMatcher{{Name: "foo", Value: "bar"}}})),
				Entry("prefix hijacking - handles case sensitive",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo"}, CaseSensitive: &wrappers.BoolValue{Value: true}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo"}, CaseSensitive: &wrappers.BoolValue{Value: false}},
					nil),
				Entry("regex hijacking - handles case sensitive (by ignoring it if set on the regex, since envoy will ignore case sensitive on regex routes)",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Regex{Regex: "/foo/.*/bar"}, CaseSensitive: &wrappers.BoolValue{Value: true}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo/user/info/bar"}, CaseSensitive: &wrappers.BoolValue{Value: false}},
					UnorderedRegexErr("gloo-system.name1", "/foo/.*/bar", &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo/user/info/bar"}, CaseSensitive: &wrappers.BoolValue{Value: false}})),
				Entry("regex hijacking - handles case sensitive (by skipping validation; we can't validate case insensitive against a regex)",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Regex{Regex: "/foo/.*/bar"}, CaseSensitive: &wrappers.BoolValue{Value: true}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo/user/info/bar"}, CaseSensitive: &wrappers.BoolValue{Value: true}},
					nil),
				Entry("inverted header matcher hijacks possible method matchers",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo"},
						Headers: []*matchers.HeaderMatcher{
							{
								Name:        ":method",
								Value:       "GET",
								InvertMatch: true,
							},
						},
					},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo"},
						Methods: []string{"GET", "POST"}, // The POST method here is unreachable
					},
					UnorderedPrefixErr("gloo-system.name1", "/foo", &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo"},
						Methods: []string{"GET", "POST"},
					})),
				Entry("prefix hijacking with inverted header matcher, late matcher partially unreachable",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Headers: []*matchers.HeaderMatcher{{Name: ":method", Value: "GET", InvertMatch: true}}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Methods: []string{"GET", "POST"}}, // The POST method here is unreachable
					UnorderedPrefixErr("gloo-system.name1", "/1", &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"}, Methods: []string{"GET", "POST"}})),
				Entry("invalid regex doesn't crash",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Regex{Regex: "["}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/"}},
					InvalidRegexErr("gloo-system.name1", "error parsing regexp: missing closing ]: `[`")),
				Entry("regex hijacking - handles lack of methods, headers, or query params",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Regex{Regex: "/anything"}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/nomatch"}},
					nil),
				Entry("regex hijacking - handles later matcher with more specific methods",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Regex{Regex: "/anything"}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/nomatch"}, Methods: []string{"GET", "POST"}},
					nil),
				Entry("regex hijacking - handles later matcher with more specific headers",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Regex{Regex: "/anything"}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/nomatch"}, Headers: []*matchers.HeaderMatcher{{Name: "foo", Value: "bar"}}},
					nil),
				Entry("regex hijacking - handles later matcher with more specific query params",
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Regex{Regex: "/anything"}},
					&matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/nomatch"}, QueryParameters: []*matchers.QueryParameterMatcher{{Name: "foo", Value: "bar"}}},
					nil),
			)
		})
	})

	Context("using RouteTables and delegation", func() {

		Context("valid configuration", func() {
			dur := prototime.DurationToProto(time.Minute)

			rootLevelRoutePlugins := &gloov1.RouteOptions{PrefixRewrite: &wrappers.StringValue{Value: "root route plugin"}}
			midLevelRoutePlugins := &gloov1.RouteOptions{Timeout: dur}
			leafLevelRoutePlugins := &gloov1.RouteOptions{PrefixRewrite: &wrappers.StringValue{Value: "leaf level plugin"}}

			mergedMidLevelRoutePlugins := &gloov1.RouteOptions{PrefixRewrite: rootLevelRoutePlugins.PrefixRewrite, Timeout: dur}
			mergedLeafLevelRoutePlugins := &gloov1.RouteOptions{PrefixRewrite: &wrappers.StringValue{Value: "leaf level plugin"}, Timeout: midLevelRoutePlugins.Timeout}

			BeforeEach(func() {
				snap = &gloov1snap.ApiSnapshot{
					Gateways: v1.GatewayList{
						{
							Metadata: &core.Metadata{Namespace: ns, Name: "name"},
							GatewayType: &v1.Gateway_HttpGateway{
								HttpGateway: &v1.HttpGateway{},
							},
							BindPort: 2,
						},
					},
					VirtualServices: v1.VirtualServiceList{
						{
							Metadata: &core.Metadata{Namespace: ns, Name: "name1"},
							VirtualHost: &v1.VirtualHost{
								Domains: []string{"d1.com"},
								Routes: []*v1.Route{
									{
										Name: "testRouteName",
										Matchers: []*matchers.Matcher{{
											PathSpecifier: &matchers.Matcher_Prefix{
												Prefix: "/a",
											},
										}},
										Action: &v1.Route_DelegateAction{
											DelegateAction: &v1.DelegateAction{
												DelegationType: &v1.DelegateAction_Ref{
													Ref: &core.ResourceRef{
														Name:      "delegate-1",
														Namespace: ns,
													},
												},
											},
										},
										Options: rootLevelRoutePlugins,
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
												Prefix: "/b",
											},
										}},
										Action: &v1.Route_DelegateAction{
											DelegateAction: &v1.DelegateAction{
												DelegationType: &v1.DelegateAction_Ref{
													Ref: &core.ResourceRef{
														Name:      "delegate-2",
														Namespace: ns,
													},
												},
											},
										},
									},
								},
							},
						},
					},
					RouteTables: []*v1.RouteTable{
						{
							Metadata: &core.Metadata{
								Name:      "delegate-1",
								Namespace: ns,
							},
							Routes: []*v1.Route{
								{
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/a/1-upstream",
										},
									}},
									Action: &v1.Route_RouteAction{
										RouteAction: &gloov1.RouteAction{
											Destination: &gloov1.RouteAction_Single{
												Single: &gloov1.Destination{
													DestinationType: &gloov1.Destination_Upstream{
														Upstream: &core.ResourceRef{
															Name:      "my-upstream",
															Namespace: ns,
														},
													},
												},
											},
										},
									},
								},
								{
									Name: "delegate1Route2",
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/a/3-delegate",
										},
									}},
									Action: &v1.Route_DelegateAction{
										DelegateAction: &v1.DelegateAction{
											DelegationType: &v1.DelegateAction_Ref{
												Ref: &core.ResourceRef{
													Name:      "delegate-3",
													Namespace: ns,
												},
											},
										},
									},
									Options: midLevelRoutePlugins,
								},
							},
						},
						{
							Metadata: &core.Metadata{
								Name:      "delegate-2",
								Namespace: ns,
							},
							Routes: []*v1.Route{
								{
									Name: "delegate2Route1",
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/b/2-upstream",
										},
									}},
									Action: &v1.Route_RouteAction{
										RouteAction: &gloov1.RouteAction{
											Destination: &gloov1.RouteAction_Single{
												Single: &gloov1.Destination{
													DestinationType: &gloov1.Destination_Upstream{
														Upstream: &core.ResourceRef{
															Name:      "my-upstream",
															Namespace: ns,
														},
													},
												},
											},
										},
									},
								},
								{
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/b/2-upstream-plugin-override",
										},
									}},
									Action: &v1.Route_RouteAction{
										RouteAction: &gloov1.RouteAction{
											Destination: &gloov1.RouteAction_Single{
												Single: &gloov1.Destination{
													DestinationType: &gloov1.Destination_Upstream{
														Upstream: &core.ResourceRef{
															Name:      "my-upstream",
															Namespace: ns,
														},
													},
												},
											},
										},
									},
									Options: leafLevelRoutePlugins,
								},
							},
						},
						{
							Metadata: &core.Metadata{
								Name:      "delegate-3",
								Namespace: ns,
							},
							Routes: []*v1.Route{
								{
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/a/3-delegate/upstream1",
										},
									}},
									Action: &v1.Route_RouteAction{
										RouteAction: &gloov1.RouteAction{
											Destination: &gloov1.RouteAction_Single{
												Single: &gloov1.Destination{
													DestinationType: &gloov1.Destination_Upstream{
														Upstream: &core.ResourceRef{
															Name:      "my-upstream",
															Namespace: ns,
														},
													},
												},
											},
										},
									},
								},
								{
									Name: "delegate3Route2",
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/a/3-delegate/upstream2",
										},
									}},
									Action: &v1.Route_RouteAction{
										RouteAction: &gloov1.RouteAction{
											Destination: &gloov1.RouteAction_Single{
												Single: &gloov1.Destination{
													DestinationType: &gloov1.Destination_Upstream{
														Upstream: &core.ResourceRef{
															Name:      "my-upstream",
															Namespace: ns,
														},
													},
												},
											},
										},
									},
									Options: leafLevelRoutePlugins,
								},
							},
						},
					},
				}
				initializeReportsForSnap()
			})

			It("merges the vs and route tables to a single gloov1.VirtualHost", func() {
				params := NewTranslatorParams(ctx, snap, reports)

				listener := translator.ComputeListener(params, "proxy1", snap.Gateways[0])
				Expect(listener).NotTo(BeNil())
				Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

				httpListener := listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
				Expect(httpListener.VirtualHosts).To(HaveLen(2))

				routes := []*gloov1.Route{
					{
						Name: "vs:name_proxy1_gloo-system_name1_route:testRouteName_rt:gloo-system_delegate-1_route:<unnamed-0>",
						Matchers: []*matchers.Matcher{{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/a/1-upstream",
							},
						}},
						Action: &gloov1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: &core.ResourceRef{
												Name:      "my-upstream",
												Namespace: "gloo-system",
											},
										},
									},
								},
							},
						},
						Options: rootLevelRoutePlugins,
						OpaqueMetadata: &gloov1.Route_MetadataStatic{
							MetadataStatic: &gloov1.SourceMetadata{
								Sources: []*gloov1.SourceMetadata_SourceRef{
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
					},
					{
						Name: "vs:name_proxy1_gloo-system_name1_route:testRouteName_rt:gloo-system_delegate-1_route:delegate1Route2_rt:gloo-system_delegate-3_route:<unnamed-0>",
						Matchers: []*matchers.Matcher{{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/a/3-delegate/upstream1",
							},
						}},
						Action: &gloov1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: &core.ResourceRef{
												Name:      "my-upstream",
												Namespace: "gloo-system",
											},
										},
									},
								},
							},
						},
						Options: mergedMidLevelRoutePlugins,
						OpaqueMetadata: &gloov1.Route_MetadataStatic{
							MetadataStatic: &gloov1.SourceMetadata{
								Sources: []*gloov1.SourceMetadata_SourceRef{
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
					},
					{
						Name: "vs:name_proxy1_gloo-system_name1_route:testRouteName_rt:gloo-system_delegate-1_route:delegate1Route2_rt:gloo-system_delegate-3_route:delegate3Route2",
						Matchers: []*matchers.Matcher{{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/a/3-delegate/upstream2",
							},
						}},
						Action: &gloov1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: &core.ResourceRef{
												Name:      "my-upstream",
												Namespace: "gloo-system",
											},
										},
									},
								},
							},
						},
						Options: mergedLeafLevelRoutePlugins,
						OpaqueMetadata: &gloov1.Route_MetadataStatic{
							MetadataStatic: &gloov1.SourceMetadata{
								Sources: []*gloov1.SourceMetadata_SourceRef{
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
					},
				}
				for index, route := range routes {
					Expect(httpListener.VirtualHosts[0].Routes[index]).To(gloo_matchers.MatchProto(route))
				}
				routes = []*gloov1.Route{
					{
						Name: "vs:name_proxy1_gloo-system_name2_route:<unnamed-0>_rt:gloo-system_delegate-2_route:delegate2Route1",
						Matchers: []*matchers.Matcher{{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/b/2-upstream",
							},
						}},
						Action: &gloov1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: &core.ResourceRef{
												Name:      "my-upstream",
												Namespace: "gloo-system",
											},
										},
									},
								},
							},
						},
						OpaqueMetadata: &gloov1.Route_MetadataStatic{
							MetadataStatic: &gloov1.SourceMetadata{
								Sources: []*gloov1.SourceMetadata_SourceRef{
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
					},
					{
						Name: "",
						Matchers: []*matchers.Matcher{{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/b/2-upstream-plugin-override",
							},
						}},
						Action: &gloov1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: &core.ResourceRef{
												Name:      "my-upstream",
												Namespace: "gloo-system",
											},
										},
									},
								},
							},
						},
						Options: leafLevelRoutePlugins,
						OpaqueMetadata: &gloov1.Route_MetadataStatic{
							MetadataStatic: &gloov1.SourceMetadata{
								Sources: []*gloov1.SourceMetadata_SourceRef{
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
					},
				}
				for index, route := range routes {
					Expect(httpListener.VirtualHosts[1].Routes[index]).To(gloo_matchers.MatchProto(route))
				}
			})

		})

		Context("delegation cycle", func() {

			BeforeEach(func() {
				snap = &gloov1snap.ApiSnapshot{
					Gateways: v1.GatewayList{
						{
							Metadata: &core.Metadata{Namespace: ns, Name: "name"},
							GatewayType: &v1.Gateway_HttpGateway{
								HttpGateway: &v1.HttpGateway{},
							},
							BindPort: 2,
						},
					},
					VirtualServices: v1.VirtualServiceList{
						{
							Metadata: &core.Metadata{Namespace: ns, Name: "has-a-cycle"},
							VirtualHost: &v1.VirtualHost{
								Domains: []string{"d1.com"},
								Routes: []*v1.Route{
									{
										Action: &v1.Route_DelegateAction{
											DelegateAction: &v1.DelegateAction{
												DelegationType: &v1.DelegateAction_Ref{
													Ref: &core.ResourceRef{
														Name:      "delegate-1",
														Namespace: ns,
													},
												},
											},
										},
									},
								},
							},
						},
					},
					RouteTables: []*v1.RouteTable{
						{
							Metadata: &core.Metadata{
								Name:      "delegate-1",
								Namespace: ns,
							},
							Routes: []*v1.Route{
								{
									Action: &v1.Route_DelegateAction{
										DelegateAction: &v1.DelegateAction{
											DelegationType: &v1.DelegateAction_Ref{
												Ref: &core.ResourceRef{
													Name:      "delegate-2",
													Namespace: ns,
												},
											},
										},
									},
								},
							},
						},
						{
							Metadata: &core.Metadata{
								Name:      "delegate-2",
								Namespace: ns,
							},
							Routes: []*v1.Route{
								{
									Action: &v1.Route_DelegateAction{
										DelegateAction: &v1.DelegateAction{
											DelegationType: &v1.DelegateAction_Ref{
												Ref: &core.ResourceRef{
													Name:      "delegate-1",
													Namespace: ns,
												},
											},
										},
									},
								},
							},
						},
					},
				}
				initializeReportsForSnap()
			})

			It("detects cycle and returns error", func() {
				params := NewTranslatorParams(ctx, snap, reports)

				_ = translator.ComputeListener(params, defaults.GatewayProxyName, snap.Gateways[0])
				err := reports.ValidateStrict()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("cycle detected"))
			})
		})

	})

	Context("generating unique route names", func() {

		It("should generate unique names for multiple gateways", func() {
			snap = &gloov1snap.ApiSnapshot{
				Gateways: v1.GatewayList{
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "gw1"},
						BindPort: 1111,
						GatewayType: &v1.Gateway_HttpGateway{
							HttpGateway: &v1.HttpGateway{},
						},
					},
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "gw2"},
						BindPort: 2222,
						GatewayType: &v1.Gateway_HttpGateway{
							HttpGateway: &v1.HttpGateway{},
						},
					},
				},
				VirtualServices: v1.VirtualServiceList{
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "vs1"},
						VirtualHost: &v1.VirtualHost{
							Domains: []string{"*"},
							Routes: []*v1.Route{
								{
									Name: "route1",
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/a",
										},
									}},
									Action: &v1.Route_RouteAction{
										RouteAction: &gloov1.RouteAction{
											Destination: &gloov1.RouteAction_Single{
												Single: &gloov1.Destination{
													DestinationType: &gloov1.Destination_Upstream{
														Upstream: &core.ResourceRef{
															Name:      "my-upstream-1",
															Namespace: ns,
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			initializeReportsForSnap()
			params := NewTranslatorParams(ctx, snap, reports)

			listener := translator.ComputeListener(params, "proxy123", snap.Gateways[0])
			Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

			httpListener := listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
			Expect(httpListener.VirtualHosts).To(HaveLen(1))
			Expect(httpListener.VirtualHosts[0].Routes).To(HaveLen(1))
			Expect(httpListener.VirtualHosts[0].Routes[0].Name).To(Equal("vs:gw1_proxy123_gloo-system_vs1_route:route1"))

			listener = translator.ComputeListener(params, "proxy123", snap.Gateways[1])
			Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

			httpListener = listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
			Expect(httpListener.VirtualHosts).To(HaveLen(1))
			Expect(httpListener.VirtualHosts[0].Routes).To(HaveLen(1))
			Expect(httpListener.VirtualHosts[0].Routes[0].Name).To(Equal("vs:gw2_proxy123_gloo-system_vs1_route:route1"))
		})

		It("should generate unique names for multiple proxies", func() {
			snap = &gloov1snap.ApiSnapshot{
				Gateways: v1.GatewayList{
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "gw1"},
						GatewayType: &v1.Gateway_HttpGateway{
							HttpGateway: &v1.HttpGateway{},
						},
					},
				},
				VirtualServices: v1.VirtualServiceList{
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "vs1"},
						VirtualHost: &v1.VirtualHost{
							Domains: []string{"*"},
							Routes: []*v1.Route{
								{
									Name: "route1",
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/a",
										},
									}},
									Action: &v1.Route_RouteAction{
										RouteAction: &gloov1.RouteAction{
											Destination: &gloov1.RouteAction_Single{
												Single: &gloov1.Destination{
													DestinationType: &gloov1.Destination_Upstream{
														Upstream: &core.ResourceRef{
															Name:      "my-upstream-1",
															Namespace: ns,
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			initializeReportsForSnap()
			params := NewTranslatorParams(ctx, snap, reports)

			listener := translator.ComputeListener(params, "proxy123", snap.Gateways[0])
			Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

			httpListener := listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
			Expect(httpListener.VirtualHosts).To(HaveLen(1))
			Expect(httpListener.VirtualHosts[0].Routes).To(HaveLen(1))
			Expect(httpListener.VirtualHosts[0].Routes[0].Name).To(Equal("vs:gw1_proxy123_gloo-system_vs1_route:route1"))

			listener = translator.ComputeListener(params, "proxy456", snap.Gateways[0])
			Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

			httpListener = listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
			Expect(httpListener.VirtualHosts).To(HaveLen(1))
			Expect(httpListener.VirtualHosts[0].Routes).To(HaveLen(1))
			Expect(httpListener.VirtualHosts[0].Routes[0].Name).To(Equal("vs:gw1_proxy456_gloo-system_vs1_route:route1"))
		})

		It("should generate unique names for virtual services in different namespaces", func() {
			snap = &gloov1snap.ApiSnapshot{
				Gateways: v1.GatewayList{
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "gw1"},
						GatewayType: &v1.Gateway_HttpGateway{
							HttpGateway: &v1.HttpGateway{},
						},
					},
				},
				VirtualServices: v1.VirtualServiceList{
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "vs1"},
						VirtualHost: &v1.VirtualHost{
							Domains: []string{"vs1.example.com"},
							Routes: []*v1.Route{
								{
									Name: "route1",
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/a",
										},
									}},
									Action: &v1.Route_RouteAction{
										RouteAction: &gloov1.RouteAction{
											Destination: &gloov1.RouteAction_Single{
												Single: &gloov1.Destination{
													DestinationType: &gloov1.Destination_Upstream{
														Upstream: &core.ResourceRef{
															Name:      "my-upstream-1",
															Namespace: ns,
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
					{
						Metadata: &core.Metadata{Namespace: ns2, Name: "vs1"},
						VirtualHost: &v1.VirtualHost{
							Domains: []string{"vs2.example.com"},
							Routes: []*v1.Route{
								{
									Name: "route1",
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/a",
										},
									}},
									Action: &v1.Route_RouteAction{
										RouteAction: &gloov1.RouteAction{
											Destination: &gloov1.RouteAction_Single{
												Single: &gloov1.Destination{
													DestinationType: &gloov1.Destination_Upstream{
														Upstream: &core.ResourceRef{
															Name:      "my-upstream-1",
															Namespace: ns,
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			initializeReportsForSnap()
			params := NewTranslatorParams(ctx, snap, reports)

			listener := translator.ComputeListener(params, "proxy123", snap.Gateways[0])
			Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

			httpListener := listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
			Expect(httpListener.VirtualHosts).To(HaveLen(2))
			Expect(httpListener.VirtualHosts[0].Routes).To(HaveLen(1))
			Expect(httpListener.VirtualHosts[0].Routes[0].Name).To(Equal("vs:gw1_proxy123_gloo-system_vs1_route:route1"))
			Expect(httpListener.VirtualHosts[1].Routes).To(HaveLen(1))
			Expect(httpListener.VirtualHosts[1].Routes[0].Name).To(Equal("vs:gw1_proxy123_gloo-system2_vs1_route:route1"))
		})

		It("should generate unique names for multiple unnamed routes", func() {
			snap = &gloov1snap.ApiSnapshot{
				Gateways: v1.GatewayList{
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "gw1"},
						GatewayType: &v1.Gateway_HttpGateway{
							HttpGateway: &v1.HttpGateway{},
						},
					},
				},
				VirtualServices: v1.VirtualServiceList{
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "vs1"},
						VirtualHost: &v1.VirtualHost{
							Domains: []string{"*"},
							Routes: []*v1.Route{
								{
									Name: "route1",
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/a",
										},
									}},
									Action: &v1.Route_DelegateAction{
										DelegateAction: &v1.DelegateAction{
											DelegationType: &v1.DelegateAction_Ref{
												Ref: &core.ResourceRef{
													Name:      "rt1",
													Namespace: ns,
												},
											},
										},
									},
								},
							},
						},
					},
				},
				RouteTables: []*v1.RouteTable{
					{
						Metadata: &core.Metadata{
							Name:      "rt1",
							Namespace: ns,
						},
						Routes: []*v1.Route{
							{
								Matchers: []*matchers.Matcher{{
									PathSpecifier: &matchers.Matcher_Prefix{
										Prefix: "/a/1",
									},
								}},
								Action: &v1.Route_RouteAction{
									RouteAction: &gloov1.RouteAction{
										Destination: &gloov1.RouteAction_Single{
											Single: &gloov1.Destination{
												DestinationType: &gloov1.Destination_Upstream{
													Upstream: &core.ResourceRef{
														Name:      "my-upstream-1",
														Namespace: ns,
													},
												},
											},
										},
									},
								},
							},
							{
								Matchers: []*matchers.Matcher{{
									PathSpecifier: &matchers.Matcher_Prefix{
										Prefix: "/a/2",
									},
								}},
								Action: &v1.Route_RouteAction{
									RouteAction: &gloov1.RouteAction{
										Destination: &gloov1.RouteAction_Single{
											Single: &gloov1.Destination{
												DestinationType: &gloov1.Destination_Upstream{
													Upstream: &core.ResourceRef{
														Name:      "my-upstream-2",
														Namespace: ns,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			initializeReportsForSnap()
			params := NewTranslatorParams(ctx, snap, reports)

			listener := translator.ComputeListener(params, "proxy123", snap.Gateways[0])
			Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

			httpListener := listener.ListenerType.(*gloov1.Listener_HttpListener).HttpListener
			Expect(httpListener.VirtualHosts).To(HaveLen(1))
			Expect(httpListener.VirtualHosts[0].Routes).To(HaveLen(2))
			Expect(httpListener.VirtualHosts[0].Routes[0].Name).To(Equal("vs:gw1_proxy123_gloo-system_vs1_route:route1_rt:gloo-system_rt1_route:<unnamed-0>"))
			Expect(httpListener.VirtualHosts[0].Routes[1].Name).To(Equal("vs:gw1_proxy123_gloo-system_vs1_route:route1_rt:gloo-system_rt1_route:<unnamed-1>"))
		})

	})

	Context("No virtual Services", func() {
		BeforeEach(func() {
			snap = &gloov1snap.ApiSnapshot{
				Gateways: v1.GatewayList{
					{
						Metadata: &core.Metadata{Namespace: ns, Name: "gw1"},
						GatewayType: &v1.Gateway_HttpGateway{
							HttpGateway: &v1.HttpGateway{},
						},
					},
				},
				VirtualServices: v1.VirtualServiceList{},
			}
			initializeReportsForSnap()
		})

		It("Does not generate a listener", func() {
			params := NewTranslatorParams(ctx, snap, reports)
			listener := translator.ComputeListener(params, "proxy123", snap.Gateways[0])
			Expect(listener).To(BeNil())
			Expect(reports.ValidateStrict()).NotTo(HaveOccurred())
		})

		// It("Does generates a listener if TranslateEmptyGateways is set", func() {
		// 	ctx := settingsutil.WithSettings(ctx, &gloov1.Settings{
		// 		Gateway: &gloov1.GatewayOptions{
		// 			TranslateEmptyGateways: &wrapperspb.BoolValue{
		// 				Value: true,
		// 			},
		// 		},
		// 	})
		// 	params := NewTranslatorParams(ctx, snap, reports)
		// 	listener := translator.ComputeListener(params, "proxy123", snap.Gateways[0])
		// 	Expect(listener).NotTo(BeNil())
		// 	Expect(reports.ValidateStrict()).NotTo(HaveOccurred())
		// })
	})

})
