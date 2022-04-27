package validation_test

import (
	"context"
	"net"
	"sync"
	"time"

	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
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
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/test/matchers"
	"google.golang.org/grpc"
)

var _ = Describe("ValidationOpts Server", func() {
	var (
		ctrl              *gomock.Controller
		settings          *v1.Settings
		translator        Translator
		params            plugins.Params
		registeredPlugins []plugins.Plugin
		xdsSanitizer      sanitizer.XdsSanitizers
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
		pluginRegistryFactory := func(ctx context.Context) plugins.PluginRegistry {
			return registry.NewPluginRegistry(registeredPlugins)
		}
		translator = NewTranslator(utils.NewSslConfigTranslator(), settings, pluginRegistryFactory)
	})

	Context("proxy validation", func() {
		It("validates the requested proxy", func() {
			proxy := params.Snapshot.Proxies[0]
			s := NewValidator(context.TODO(), translator, xdsSanitizer)
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
		It("updates the proxy report when sanitization causes a change", func() {
			proxy := params.Snapshot.Proxies[0]
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

			s := NewValidator(context.TODO(), translator, xdsSanitizer)
			_ = s.Sync(context.TODO(), params.Snapshot)
			rpt, err := s.Validate(context.TODO(), &validationgrpc.GlooValidationServiceRequest{Proxy: proxy})
			Expect(err).NotTo(HaveOccurred())

			// http
			routeError := rpt.GetValidationReports()[0].GetProxyReport().GetListenerReports()[0].GetHttpListenerReport().GetVirtualHostReports()[0].GetRouteReports()[0].GetErrors()
			routeWarning := rpt.GetValidationReports()[0].GetProxyReport().GetListenerReports()[0].GetHttpListenerReport().GetVirtualHostReports()[0].GetRouteReports()[0].GetWarnings()
			Expect(routeError).To(BeEmpty())
			Expect(routeWarning[0].Reason).To(Equal("no destination type specified"))

			// hybrid
			routeError = rpt.GetValidationReports()[0].GetProxyReport().GetListenerReports()[2].GetHybridListenerReport().GetMatchedListenerReports()[utils.MatchedRouteConfigName(proxy.GetListeners()[2], proxy.GetListeners()[2].GetHybridListener().GetMatchedListeners()[0].GetMatcher())].GetHttpListenerReport().GetVirtualHostReports()[0].GetRouteReports()[0].GetErrors()
			routeWarning = rpt.GetValidationReports()[0].GetProxyReport().GetListenerReports()[2].GetHybridListenerReport().GetMatchedListenerReports()[utils.MatchedRouteConfigName(proxy.GetListeners()[2], proxy.GetListeners()[2].GetHybridListener().GetMatchedListeners()[0].GetMatcher())].GetHttpListenerReport().GetVirtualHostReports()[0].GetRouteReports()[0].GetWarnings()
			Expect(routeError).To(BeEmpty())
			Expect(routeWarning[0].Reason).To(Equal("no destination type specified"))
		})
		It("upstream validation succeeds", func() {
			proxy1 := params.Snapshot.Proxies[0]
			proxy2 := &v1.Proxy{
				Metadata: &core.Metadata{
					Name:      "proxy2",
					Namespace: "gloo-system",
				},
			}
			params.Snapshot.Proxies = v1.ProxyList{proxy1, proxy2}

			s := NewValidator(context.TODO(), translator, xdsSanitizer)
			_ = s.Sync(context.TODO(), params.Snapshot)
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
			Expect(report1.GetProxy()).To(matchers.MatchProto(proxy1))
			Expect(validation.GetProxyWarning(report1.GetProxyReport())).To(BeEmpty())
			Expect(validation.GetProxyError(report1.GetProxyReport())).NotTo(HaveOccurred())
			report2 := resp.ValidationReports[1]
			Expect(report2.GetProxy()).To(matchers.MatchProto(proxy2))
			Expect(validation.GetProxyWarning(report2.GetProxyReport())).To(BeEmpty())
			Expect(validation.GetProxyError(report2.GetProxyReport())).NotTo(HaveOccurred())
		})
		It("upstream validation fails", func() {
			// having no upstreams in the snapshot should cause translation to fail due to a proxy from the snapshot
			// referencing the "test" upstream. this should cause any new upstreams we try to apply to be rejected
			params.Snapshot.Upstreams = v1.UpstreamList{}

			s := NewValidator(context.TODO(), translator, xdsSanitizer)
			_ = s.Sync(context.TODO(), params.Snapshot)
			resp, err := s.Validate(context.TODO(), &validationgrpc.GlooValidationServiceRequest{
				Resources: &validationgrpc.GlooValidationServiceRequest_ModifiedResources{
					ModifiedResources: &validationgrpc.ModifiedResources{
						Upstreams: []*v1.Upstream{
							{
								Metadata: &core.Metadata{Name: "other-upstream", Namespace: "other-namespace"},
							},
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(resp.ValidationReports).To(HaveLen(1))
			proxyReport := resp.ValidationReports[0].GetProxyReport()
			warnings := validation.GetProxyWarning(proxyReport)
			errors := validation.GetProxyError(proxyReport)
			Expect(warnings).To(HaveLen(2)) // one each for http and hybrid
			Expect(errors).To(HaveOccurred())
		})
		It("upstream deletion validation succeeds", func() {
			// deleting an upstream that is not being used should succeed
			s := NewValidator(context.TODO(), translator, xdsSanitizer)
			_ = s.Sync(context.TODO(), params.Snapshot)
			resp, err := s.Validate(context.TODO(), &validationgrpc.GlooValidationServiceRequest{
				Resources: &validationgrpc.GlooValidationServiceRequest_DeletedResources{
					DeletedResources: &validationgrpc.DeletedResources{
						UpstreamRefs: []*core.ResourceRef{
							{Name: "unused-upstream", Namespace: "gloo-system"},
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.ValidationReports).To(HaveLen(1))
			proxyReport := resp.ValidationReports[0].GetProxyReport()
			warnings := validation.GetProxyWarning(proxyReport)
			errors := validation.GetProxyError(proxyReport)
			Expect(warnings).To(HaveLen(0))
			Expect(errors).NotTo(HaveOccurred())
		})
		It("upstream deletion validation fails", func() {
			// trying to delete an upstream that is being referenced by a proxy should cause an error
			s := NewValidator(context.TODO(), translator, xdsSanitizer)
			_ = s.Sync(context.TODO(), params.Snapshot)
			resp, err := s.Validate(context.TODO(), &validationgrpc.GlooValidationServiceRequest{
				Resources: &validationgrpc.GlooValidationServiceRequest_DeletedResources{
					DeletedResources: &validationgrpc.DeletedResources{
						UpstreamRefs: []*core.ResourceRef{
							{Name: "test", Namespace: "gloo-system"},
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.ValidationReports).To(HaveLen(1))
			proxyReport := resp.ValidationReports[0].GetProxyReport()
			warnings := validation.GetProxyWarning(proxyReport)
			errors := validation.GetProxyError(proxyReport)
			Expect(warnings).To(HaveLen(2))
			Expect(errors).To(HaveOccurred())
		})
		It("secret deletion validation succeeds", func() {
			// deleting a secret that is not being used should succeed
			s := NewValidator(context.TODO(), translator, xdsSanitizer)
			_ = s.Sync(context.TODO(), params.Snapshot)
			resp, err := s.Validate(context.TODO(), &validationgrpc.GlooValidationServiceRequest{
				Resources: &validationgrpc.GlooValidationServiceRequest_DeletedResources{
					DeletedResources: &validationgrpc.DeletedResources{
						SecretRefs: []*core.ResourceRef{
							{Name: "unused-secret", Namespace: "gloo-system"},
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.ValidationReports).To(HaveLen(1))
			proxyReport := resp.ValidationReports[0].GetProxyReport()
			warnings := validation.GetProxyWarning(proxyReport)
			errors := validation.GetProxyError(proxyReport)
			Expect(warnings).To(HaveLen(0))
			Expect(errors).NotTo(HaveOccurred())
		})
		It("secret deletion validation fails", func() {
			// trying to delete a secret that is being referenced by a proxy should cause an error
			s := NewValidator(context.TODO(), translator, xdsSanitizer)
			_ = s.Sync(context.TODO(), params.Snapshot)
			resp, err := s.Validate(context.TODO(), &validationgrpc.GlooValidationServiceRequest{
				Resources: &validationgrpc.GlooValidationServiceRequest_DeletedResources{
					DeletedResources: &validationgrpc.DeletedResources{
						SecretRefs: []*core.ResourceRef{
							{Name: "secret", Namespace: "gloo-system"},
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.ValidationReports).To(HaveLen(1))

			upstreamReports := resp.ValidationReports[0].GetUpstreamReports()
			Expect(upstreamReports).To(HaveLen(1))

			// Verify the report is for the upstream we expect
			usRef := upstreamReports[0].GetResourceRef()
			Expect(usRef.GetNamespace()).To(Equal("gloo-system"))
			Expect(usRef.GetName()).To(Equal("test"))

			// Verify report contains a warning for the secret we removed
			warnings := upstreamReports[0].GetWarnings()
			Expect(warnings).To(HaveLen(1))
			Expect(warnings[0]).To(ContainSubstring("SSL secret not found: list did not find secret gloo-system.secret"))
		})
	})

	Context("Watch Sync Notifications", func() {
		var (
			srv    *grpc.Server
			v      Validator
			client validationgrpc.GlooValidationServiceClient
		)
		BeforeEach(func() {
			lis, err := net.Listen("tcp", ":0")
			Expect(err).NotTo(HaveOccurred())

			srv = grpc.NewServer()

			v = NewValidator(context.TODO(), nil, xdsSanitizer)

			server := NewValidationServer()
			server.SetValidator(v)
			server.Register(srv)

			go func() {
				defer GinkgoRecover()
				err = srv.Serve(lis)
				Expect(err).NotTo(HaveOccurred())
			}()

			cc, err := grpc.DialContext(context.TODO(), lis.Addr().String(), grpc.WithInsecure(), grpc.WithBlock())
			Expect(err).NotTo(HaveOccurred())

			client = validationgrpc.NewGlooValidationServiceClient(cc)

		})
		AfterEach(func() {
			srv.Stop()
		})

		It("sends sync notifications", func() {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			stream, err := client.NotifyOnResync(ctx, &validationgrpc.NotifyOnResyncRequest{})
			Expect(err).NotTo(HaveOccurred())

			var notifications []*validationgrpc.NotifyOnResyncResponse
			var l sync.Mutex
			var desiredErrCode codes.Code

			// watch notifications
			go func() {
				defer GinkgoRecover()
				for {
					notification, err := stream.Recv()
					if desiredErrCode == 0 {
						Expect(err).To(BeNil())
					} else {
						Expect(err).NotTo(BeNil())
						st, ok := status.FromError(err)
						Expect(ok).To(BeTrue())
						Expect(st.Code()).To(Equal(desiredErrCode))
						continue
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
			desiredErrCode = codes.Unavailable
			srv.Stop()

			// create jitter by changing upstreams
			err = v.Sync(ctx, &v1snap.ApiSnapshot{Upstreams: v1.UpstreamList{{}, {}}})
			Expect(err).NotTo(HaveOccurred())

			Consistently(getNotifications, time.Second).Should(HaveLen(5))
		})
	})
})
