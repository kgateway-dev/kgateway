package validation_test

import (
	"context"
	"net"
	"sync"
	"time"

	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ratelimit "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	validationgrpc "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	enterprise_gloo_solo_io "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	. "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	mock_consul "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul/mocks"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	. "github.com/solo-io/gloo/projects/gloo/pkg/validation"
	"github.com/solo-io/gloo/test/samples"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/test/matchers"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
)

var _ = Describe("Validation Server", func() {
	var (
		ctrl              *gomock.Controller
		settings          *v1.Settings
		translator        Translator
		params            plugins.Params
		registeredPlugins []plugins.Plugin
		xdsSanitizer      sanitizer.XdsSanitizers
		vc                ValidatorConfig
	)

	BeforeEach(func() {

		ctrl = gomock.NewController(T)

		settings = &v1.Settings{}
		memoryClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}
		opts := bootstrap.Opts{
			Settings:  settings,
			Secrets:   memoryClientFactory,
			Upstreams: memoryClientFactory,
			Consul: bootstrap.Consul{
				ConsulWatcher: mock_consul.NewMockConsulWatcher(ctrl), // just needed to activate the consul plugin
			},
		}
		registeredPlugins = registry.Plugins(opts)

		params = plugins.Params{
			Ctx:      context.Background(),
			Snapshot: samples.SimpleGlooSnapshot("gloo-system"),
		}

		routeReplacingSanitizer, _ := sanitizer.NewRouteReplacingSanitizer(settings.GetGloo().GetInvalidConfigPolicy())
		xdsSanitizer = sanitizer.XdsSanitizers{
			sanitizer.NewUpstreamRemovingSanitizer(),
			routeReplacingSanitizer,
		}
	})

	JustBeforeEach(func() {
		pluginRegistry := registry.NewPluginRegistry(registeredPlugins)

		translator = NewTranslatorWithHasher(utils.NewSslConfigTranslator(), settings, pluginRegistry, EnvoyCacheResourcesListToFnvHash)
		vc = ValidatorConfig{
			Ctx: context.TODO(),
			GlooValidatorConfig: GlooValidatorConfig{
				XdsSanitizer: xdsSanitizer,
				Translator:   translator,
			},
		}
	})

	Context("proxy validation", func() {
		var proxy *v1.Proxy
		var s Validator
		Context("validates the requested proxy", func() {
			It("works with Validate", func() {
				proxy := params.Snapshot.Proxies[0]
				s := NewValidator(vc)
				_ = s.Sync(context.TODO(), params.Snapshot)
				rpt, err := s.Validate(context.TODO(), &validationgrpc.GlooValidationServiceRequest{Proxy: proxy})
				Expect(err).NotTo(HaveOccurred())
				Expect(rpt).To(matchers.MatchProto(&validationgrpc.GlooValidationServiceResponse{
					ValidationReports: []*validationgrpc.ValidationReport{
						{
							ProxyReport:     validation.MakeReport(proxy),
							UpstreamReports: []*validationgrpc.ResourceReport{},
							Proxy:           proxy,
						},
					},
				}))
			})
			It("works with Validate Gloo", func() {
				proxy := params.Snapshot.Proxies[0]
				s := NewValidator(vc)
				_ = s.Sync(context.TODO(), params.Snapshot)
				rpt, err := s.ValidateGloo(context.TODO(), proxy, nil, false)
				Expect(err).NotTo(HaveOccurred())
				r := rpt[0]
				Expect(r.Proxy).To(Equal(proxy))
				Expect(r.ResourceReports).To(Equal(reporter.ResourceReports{}))
				Expect(r.ProxyReport).To(matchers.MatchProto(validation.MakeReport(proxy)))
			})
		})

		Context("updates the proxy report when sanitization causes a change", func() {
			JustBeforeEach(func() {
				proxy = params.Snapshot.Proxies[0]
				// Update proxy so that it includes an invalid definition - the nil destination type should
				// raise an error since the destination type is not specified
				errorRouteAction := &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &v1.Destination{
								DestinationType: nil,
							},
						},
					},
				}
				proxy.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].GetRoutes()[0].Action = errorRouteAction
				proxy.GetListeners()[2].GetHybridListener().GetMatchedListeners()[0].GetHttpListener().GetVirtualHosts()[0].GetRoutes()[0].Action = errorRouteAction

				s = NewValidator(vc)
				_ = s.Sync(context.TODO(), params.Snapshot)
			})

			validateProxyReport := func(proxyReport *validationgrpc.ProxyReport, proxy *v1.Proxy) {
				// http
				routeError := proxyReport.GetListenerReports()[0].GetHttpListenerReport().GetVirtualHostReports()[0].GetRouteReports()[0].GetErrors()
				routeWarning := proxyReport.GetListenerReports()[0].GetHttpListenerReport().GetVirtualHostReports()[0].GetRouteReports()[0].GetWarnings()
				Expect(routeError).To(BeEmpty())
				Expect(routeWarning[0].Reason).To(Equal("no destination type specified"))

				// hybrid
				routeError = proxyReport.GetListenerReports()[2].GetHybridListenerReport().GetMatchedListenerReports()[utils.MatchedRouteConfigName(proxy.GetListeners()[2], proxy.GetListeners()[2].GetHybridListener().GetMatchedListeners()[0].GetMatcher())].GetHttpListenerReport().GetVirtualHostReports()[0].GetRouteReports()[0].GetErrors()
				routeWarning = proxyReport.GetListenerReports()[2].GetHybridListenerReport().GetMatchedListenerReports()[utils.MatchedRouteConfigName(proxy.GetListeners()[2], proxy.GetListeners()[2].GetHybridListener().GetMatchedListeners()[0].GetMatcher())].GetHttpListenerReport().GetVirtualHostReports()[0].GetRouteReports()[0].GetWarnings()
				Expect(routeError).To(BeEmpty())
				Expect(routeWarning[0].Reason).To(Equal("no destination type specified"))
			}

			It("works with Validate", func() {
				rpt, err := s.Validate(context.TODO(), &validationgrpc.GlooValidationServiceRequest{Proxy: proxy})
				Expect(err).NotTo(HaveOccurred())
				validateProxyReport(rpt.GetValidationReports()[0].GetProxyReport(), proxy)
			})

			It("works with Validate Gloo", func() {
				rpt, err := s.ValidateGloo(context.TODO(), proxy, nil, false)
				Expect(err).NotTo(HaveOccurred())
				validateProxyReport(rpt[0].ProxyReport, proxy)
			})
		})

		Context("upstream validation succeeds", func() {
			var proxy1, proxy2 *v1.Proxy

			JustBeforeEach(func() {
				proxy1 = params.Snapshot.Proxies[0]
				proxy2 = &v1.Proxy{
					Metadata: &core.Metadata{
						Name:      "proxy2",
						Namespace: "gloo-system",
					},
				}
				params.Snapshot.Proxies = v1.ProxyList{proxy1, proxy2}

				s = NewValidator(vc)
				_ = s.Sync(context.TODO(), params.Snapshot)
			})

			validateProxyAndReport := func(proxy *v1.Proxy, proxyToMatch *v1.Proxy, proxyReport *validationgrpc.ProxyReport) {
				Expect(proxy).To(matchers.MatchProto(proxyToMatch))
				Expect(validation.GetProxyWarning(proxyReport)).To(BeEmpty())
				Expect(validation.GetProxyError(proxyReport)).NotTo(HaveOccurred())
			}

			It("works with Validate", func() {
				resp, err := s.Validate(context.TODO(), &validationgrpc.GlooValidationServiceRequest{
					Resources: &validationgrpc.GlooValidationServiceRequest_ModifiedResources{
						ModifiedResources: &validationgrpc.ModifiedResources{
							Upstreams: []*v1.Upstream{samples.SimpleUpstream()},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				// should create a report for each proxy
				Expect(resp.ValidationReports).To(HaveLen(2))
				report1 := resp.ValidationReports[0]
				validateProxyAndReport(report1.GetProxy(), proxy1, report1.GetProxyReport())
				report2 := resp.ValidationReports[1]
				validateProxyAndReport(report2.GetProxy(), proxy2, report2.GetProxyReport())
			})

			It("works with Validate Gloo", func() {
				rprts, err := s.ValidateGloo(context.TODO(), nil, samples.SimpleUpstream(), false)
				Expect(err).NotTo(HaveOccurred())
				// should create a report for each proxy
				Expect(rprts).To(HaveLen(2))
				report1 := rprts[0]
				validateProxyAndReport(report1.Proxy, proxy1, report1.ProxyReport)
				report2 := rprts[1]
				validateProxyAndReport(report2.Proxy, proxy2, report2.ProxyReport)
			})
		})

		Context("upstream validation fails", func() {
			// having no upstreams in the snapshot should cause translation to fail due to a proxy from the snapshot
			// referencing the "test" upstream. this should cause any new upstreams we try to apply to be rejected
			var upstream v1.Upstream

			validateProxyReport := func(proxyReport *validationgrpc.ProxyReport) {
				warnings := validation.GetProxyWarning(proxyReport)
				errors := validation.GetProxyError(proxyReport)
				Expect(warnings).To(HaveLen(4))
				Expect(errors).NotTo(HaveOccurred())
			}

			JustBeforeEach(func() {
				params.Snapshot.Upstreams = v1.UpstreamList{}
				s = NewValidator(vc)
				_ = s.Sync(context.TODO(), params.Snapshot)
				upstream = v1.Upstream{
					Metadata: &core.Metadata{Name: "other-upstream", Namespace: "other-namespace"},
				}
			})

			It("works with Validate", func() {
				resp, err := s.Validate(context.TODO(), &validationgrpc.GlooValidationServiceRequest{
					Resources: &validationgrpc.GlooValidationServiceRequest_ModifiedResources{
						ModifiedResources: &validationgrpc.ModifiedResources{
							Upstreams: []*v1.Upstream{
								{
									Metadata: upstream.GetMetadata(),
								},
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.ValidationReports).To(HaveLen(1))
				validateProxyReport(resp.ValidationReports[0].GetProxyReport())
			})

			It("works with validate Gloo", func() {
				reports, err := s.ValidateGloo(context.TODO(), nil, &upstream, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(reports).To(HaveLen(1))
				validateProxyReport(reports[0].ProxyReport)
			})
		})

		Context("upstream deletion validation succeeds", func() {
			// deleting an upstream that is not being used should succeed
			var upstream v1.Upstream

			JustBeforeEach(func() {
				upstream = v1.Upstream{
					Metadata: &core.Metadata{Name: "unused-upstream", Namespace: "gloo-system"},
				}
				s = NewValidator(vc)
			})

			validateProxyReport := func(proxyReport *validationgrpc.ProxyReport) {
				warnings := validation.GetProxyWarning(proxyReport)
				errors := validation.GetProxyError(proxyReport)
				Expect(warnings).To(HaveLen(0))
				Expect(errors).NotTo(HaveOccurred())
			}

			It("works with Validate", func() {
				_ = s.Sync(context.TODO(), params.Snapshot)
				resp, err := s.Validate(context.TODO(), &validationgrpc.GlooValidationServiceRequest{
					Resources: &validationgrpc.GlooValidationServiceRequest_DeletedResources{
						DeletedResources: &validationgrpc.DeletedResources{
							UpstreamRefs: []*core.ResourceRef{
								upstream.GetMetadata().Ref(),
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.ValidationReports).To(HaveLen(1))
				validateProxyReport(resp.ValidationReports[0].GetProxyReport())
			})

			It("works with Gloo Validate", func() {
				_ = s.Sync(context.TODO(), params.Snapshot)
				reports, err := s.ValidateGloo(context.TODO(), nil, &upstream, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(reports).To(HaveLen(1))
				validateProxyReport(reports[0].ProxyReport)
			})
		})
		Context("upstream deletion validation fails", func() {
			// trying to delete an upstream that is being referenced by a proxy should cause an error
			var upstream v1.Upstream

			JustBeforeEach(func() {
				upstream = v1.Upstream{
					Metadata: &core.Metadata{Name: "test", Namespace: "gloo-system"},
				}
				s = NewValidator(vc)
				_ = s.Sync(context.TODO(), params.Snapshot)
			})

			validateProxyReport := func(proxyReport *validationgrpc.ProxyReport) {
				warnings := validation.GetProxyWarning(proxyReport)
				errors := validation.GetProxyError(proxyReport)
				Expect(warnings).To(HaveLen(4))
				Expect(errors).NotTo(HaveOccurred())
			}

			It("works with Validate", func() {
				resp, err := s.Validate(context.TODO(), &validationgrpc.GlooValidationServiceRequest{
					Resources: &validationgrpc.GlooValidationServiceRequest_DeletedResources{
						DeletedResources: &validationgrpc.DeletedResources{
							UpstreamRefs: []*core.ResourceRef{
								upstream.GetMetadata().Ref(),
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.ValidationReports).To(HaveLen(1))
				validateProxyReport(resp.ValidationReports[0].GetProxyReport())
			})

			It("works with Validate Gloo", func() {
				reports, err := s.ValidateGloo(context.TODO(), nil, &upstream, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(reports).To(HaveLen(1))
				validateProxyReport(reports[0].ProxyReport)
			})
		})
		Context("secret deletion validation succeeds", func() {
			// deleting a secret that is not being used should succeed
			var secret v1.Secret

			JustBeforeEach(func() {
				s = NewValidator(vc)
				_ = s.Sync(context.TODO(), params.Snapshot)
				secret = v1.Secret{
					Metadata: &core.Metadata{Name: "unused-secret", Namespace: "gloo-system"},
				}
			})

			validateProxyReport := func(proxyReport *validationgrpc.ProxyReport) {
				warnings := validation.GetProxyWarning(proxyReport)
				errors := validation.GetProxyError(proxyReport)
				Expect(warnings).To(HaveLen(0))
				Expect(errors).NotTo(HaveOccurred())
			}

			It("works with Validate", func() {
				resp, err := s.Validate(context.TODO(), &validationgrpc.GlooValidationServiceRequest{
					Resources: &validationgrpc.GlooValidationServiceRequest_DeletedResources{
						DeletedResources: &validationgrpc.DeletedResources{
							SecretRefs: []*core.ResourceRef{
								secret.Metadata.Ref(),
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.ValidationReports).To(HaveLen(1))
				validateProxyReport(resp.ValidationReports[0].GetProxyReport())
			})

			It("works with Validate Gloo", func() {
				reports, err := s.ValidateGloo(context.TODO(), nil, &secret, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(reports).To(HaveLen(1))
				validateProxyReport(reports[0].ProxyReport)
			})
		})

		Context("secret deletion validation fails", func() {
			// trying to delete a secret that is being referenced by a proxy should cause an error
			var secret v1.Secret

			JustBeforeEach(func() {
				s = NewValidator(vc)
				_ = s.Sync(context.TODO(), params.Snapshot)
				secret = v1.Secret{
					Metadata: &core.Metadata{Name: "secret", Namespace: "gloo-system"},
				}
			})

			validateResourceRefAndWarnings := func(ref *core.ResourceRef, warnings []string) {
				// Verify the report is for the upstream we expect
				Expect(ref.GetNamespace()).To(Equal("gloo-system"))
				Expect(ref.GetName()).To(Equal("test"))

				// Verify report contains a warning for the secret we removed
				Expect(warnings).To(HaveLen(1))
				Expect(warnings[0]).To(ContainSubstring("SSL secret not found: list did not find secret gloo-system.secret"))
			}

			It("works with Validate", func() {
				resp, err := s.Validate(context.TODO(), &validationgrpc.GlooValidationServiceRequest{
					Resources: &validationgrpc.GlooValidationServiceRequest_DeletedResources{
						DeletedResources: &validationgrpc.DeletedResources{
							SecretRefs: []*core.ResourceRef{
								secret.GetMetadata().Ref(),
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.ValidationReports).To(HaveLen(1))

				upstreamReports := resp.ValidationReports[0].GetUpstreamReports()
				Expect(upstreamReports).To(HaveLen(1))

				validateResourceRefAndWarnings(upstreamReports[0].GetResourceRef(), upstreamReports[0].GetWarnings())
			})

			It("works with Validate Gloo", func() {
				reports, err := s.ValidateGloo(context.TODO(), nil, &secret, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(reports).To(HaveLen(1))

				resourceReport := reports[0].ResourceReports
				Expect(resourceReport).To(HaveLen(1))

				// have to get the resource reference from the resource Report keys
				keyResources := make([]resources.InputResource, len(resourceReport))
				i := 0
				for k := range resourceReport {
					keyResources[i] = k
					i++
				}
				keyForResource := keyResources[0]
				validateResourceRefAndWarnings(keyForResource.GetMetadata().Ref(), resourceReport[keyForResource].Warnings)
			})
		})
	})

	Context("Watch Sync Notifications", func() {

		var (
			ctx    context.Context
			cancel context.CancelFunc
			srv    *grpc.Server
			v      Validator
			client validationgrpc.GlooValidationServiceClient
		)

		// The ValidatorConfig is initialized in an outer JustBeforeEach, so we want to run
		// this block after that has been initialized. That way the inner resources (namely, the context)
		// are all valid by the time this Setup Node is executed
		JustBeforeEach(func() {
			ctx, cancel = context.WithCancel(context.TODO())
			lis, err := net.Listen("tcp", ":0")
			Expect(err).NotTo(HaveOccurred())

			srv = grpc.NewServer()

			v = NewValidator(vc)

			server := NewValidationServer()
			server.SetValidator(v)
			server.Register(srv)

			go func() {
				defer GinkgoRecover()
				err = srv.Serve(lis)
				Expect(err).NotTo(HaveOccurred())
			}()

			cc, err := grpc.DialContext(ctx, lis.Addr().String(), grpc.WithInsecure(), grpc.WithBlock())
			Expect(err).NotTo(HaveOccurred())

			client = validationgrpc.NewGlooValidationServiceClient(cc)

		})
		AfterEach(func() {
			cancel()

			srv.Stop()
		})

		It("sends sync notifications", func() {
			stream, err := client.NotifyOnResync(ctx, &validationgrpc.NotifyOnResyncRequest{})
			Expect(err).NotTo(HaveOccurred())

			var notifications []*validationgrpc.NotifyOnResyncResponse
			var l sync.Mutex

			terminalState := make(chan codes.Code, 1)

			// watch notifications
			go func() {
				defer GinkgoRecover()
				var state codes.Code // ENUM value 0
				for {
					notification, err := stream.Recv()

					select {
					case someCode := <-terminalState:
						state = someCode
					default:

					}
					if state != 0 {
						Expect(err).NotTo(BeNil())
						st, ok := status.FromError(err)
						Expect(ok).To(BeTrue())
						Expect(st.Code()).To(Equal(state))
						continue
					} else {
						Expect(err).To(BeNil())
					}

					l.Lock()
					notifications = append(notifications, notification)
					l.Unlock()
				}
			}()

			getNotifications := func() []*validationgrpc.NotifyOnResyncResponse {
				l.Lock()
				notesCopy := make([]*validationgrpc.NotifyOnResyncResponse, len(notifications))
				copy(notesCopy, notifications)
				l.Unlock()
				return notesCopy
			}

			// check that we received ACK
			Eventually(getNotifications, time.Hour).Should(HaveLen(1))

			// do some syncs
			err = v.Sync(ctx, &v1snap.ApiSnapshot{})
			Expect(err).NotTo(HaveOccurred())

			Eventually(getNotifications, time.Second).Should(HaveLen(2))

			// add an auth config
			err = v.Sync(ctx, &v1snap.ApiSnapshot{
				AuthConfigs: enterprise_gloo_solo_io.AuthConfigList{{}}},
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(getNotifications, time.Second).Should(HaveLen(3))

			// add a rate limit config
			err = v.Sync(ctx, &v1snap.ApiSnapshot{
				Ratelimitconfigs: ratelimit.RateLimitConfigList{{}}},
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(getNotifications, time.Second).Should(HaveLen(4))

			// create jitter by changing upstreams
			err = v.Sync(ctx, &v1snap.ApiSnapshot{Upstreams: v1.UpstreamList{{}}})
			Expect(err).NotTo(HaveOccurred())

			Eventually(getNotifications, time.Second).Should(HaveLen(5))

			// test close
			terminalState <- codes.Unavailable
			srv.Stop()

			// create jitter by changing upstreams
			err = v.Sync(ctx, &v1snap.ApiSnapshot{Upstreams: v1.UpstreamList{{}, {}}})
			Expect(err).NotTo(HaveOccurred())

			Consistently(getNotifications, time.Second).Should(HaveLen(5))
		})
	})
})
