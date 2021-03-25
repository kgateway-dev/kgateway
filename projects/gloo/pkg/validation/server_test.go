package validation_test

import (
	"context"
	"net"
	"sync"
	"time"

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
	sslutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	. "github.com/solo-io/gloo/projects/gloo/pkg/validation"
	"github.com/solo-io/gloo/test/samples"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/test/matchers"
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
			Snapshot: samples.SimpleGlooSnapshot(),
		}

		routeReplacingSanitizer, _ := sanitizer.NewRouteReplacingSanitizer(settings.GetGloo().GetInvalidConfigPolicy())
		xdsSanitizer = sanitizer.XdsSanitizers{
			sanitizer.NewUpstreamRemovingSanitizer(),
			routeReplacingSanitizer,
		}
	})

	JustBeforeEach(func() {
		getPlugins := func() []plugins.Plugin {
			return registeredPlugins
		}
		translator = NewTranslator(sslutils.NewSslConfigTranslator(), settings, getPlugins)
	})

	Context("proxy validation", func() {
		It("validates the requested proxy", func() {
			proxy := params.Snapshot.Proxies[0]
			s := NewValidator(context.TODO(), translator, xdsSanitizer)
			_ = s.Sync(context.TODO(), params.Snapshot)
			rpt, err := s.ValidateProxy(context.TODO(), &validationgrpc.ProxyValidationServiceRequest{Proxy: proxy})
			Expect(err).NotTo(HaveOccurred())
			Expect(rpt).To(matchers.MatchProto(&validationgrpc.ProxyValidationServiceResponse{ProxyReport: validation.MakeReport(proxy)}))
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

			s := NewValidator(context.TODO(), translator, xdsSanitizer)
			_ = s.Sync(context.TODO(), params.Snapshot)
			rpt, err := s.ValidateProxy(context.TODO(), &validationgrpc.ProxyValidationServiceRequest{Proxy: proxy})
			routeError := rpt.GetProxyReport().GetListenerReports()[0].GetHttpListenerReport().GetVirtualHostReports()[0].GetRouteReports()[0].GetErrors()
			routeWarning := rpt.GetProxyReport().GetListenerReports()[0].GetHttpListenerReport().GetVirtualHostReports()[0].GetRouteReports()[0].GetWarnings()
			Expect(err).NotTo(HaveOccurred())
			Expect(routeError).To(BeEmpty())
			Expect(routeWarning[0].Reason).To(Equal("no destination type specified"))
		})
	})

	Context("Watch Sync Notifications", func() {
		var (
			srv    *grpc.Server
			v      Validator
			client validationgrpc.ProxyValidationServiceClient
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

			client = validationgrpc.NewProxyValidationServiceClient(cc)

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
			var desiredErr string

			// watch notifications
			go func() {
				defer GinkgoRecover()
				for {
					notification, err := stream.Recv()
					if desiredErr == "" {
						Expect(err).To(BeNil())
					} else {
						Expect(err).NotTo(BeNil())
						Expect(err.Error()).To(ContainSubstring(desiredErr))
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
			err = v.Sync(ctx, &v1.ApiSnapshot{})
			Expect(err).NotTo(HaveOccurred())

			Eventually(getNotifications, time.Second).Should(HaveLen(2))

			// add an auth config
			err = v.Sync(ctx, &v1.ApiSnapshot{
				AuthConfigs: enterprise_gloo_solo_io.AuthConfigList{{}}},
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(getNotifications, time.Second).Should(HaveLen(3))

			// add a rate limit config
			err = v.Sync(ctx, &v1.ApiSnapshot{
				Ratelimitconfigs: ratelimit.RateLimitConfigList{{}}},
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(getNotifications, time.Second).Should(HaveLen(4))

			// create jitter by changing upstreams
			err = v.Sync(ctx, &v1.ApiSnapshot{Upstreams: v1.UpstreamList{{}}})
			Expect(err).NotTo(HaveOccurred())

			Eventually(getNotifications, time.Second).Should(HaveLen(5))

			// test close
			desiredErr = "transport is closing"
			srv.Stop()

			// create jitter by changing upstreams
			err = v.Sync(ctx, &v1.ApiSnapshot{Upstreams: v1.UpstreamList{{}, {}}})
			Expect(err).NotTo(HaveOccurred())

			Consistently(getNotifications, time.Second).Should(HaveLen(5))
		})
	})
})
