package validation_test

import (
	"context"

	validationgrpc "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/validation"

	"github.com/golang/mock/gomock"

	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"

	"github.com/solo-io/gloo/pkg/utils"
	sslutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"

	. "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Validation Server", func() {
	var (
		ctrl              *gomock.Controller
		settings          *v1.Settings
		translator        Translator
		upstream          *v1.Upstream
		upName            core.Metadata
		proxy             *v1.Proxy
		params            plugins.Params
		registeredPlugins []plugins.Plugin
		matcher           *v1.Matcher
		routes            []*v1.Route
	)

	BeforeEach(func() {

		ctrl = gomock.NewController(T)

		settings = &v1.Settings{}
		memoryClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}
		opts := bootstrap.Opts{
			Settings:      settings,
			Secrets:       memoryClientFactory,
			Upstreams:     memoryClientFactory,
			ConsulWatcher: consul.NewMockConsulWatcher(ctrl), // just needed to activate the consul plugin
		}
		registeredPlugins = registry.Plugins(opts)

		upName = core.Metadata{
			Name:      "test",
			Namespace: "gloo-system",
		}
		upstream = &v1.Upstream{
			Metadata: upName,
			UpstreamSpec: &v1.UpstreamSpec{
				UpstreamType: &v1.UpstreamSpec_Static{
					Static: &v1static.UpstreamSpec{
						Hosts: []*v1static.Host{
							{
								Addr: "Test",
								Port: 124,
							},
						},
					},
				},
			},
		}

		params = plugins.Params{
			Ctx: context.Background(),
			Snapshot: &v1.ApiSnapshot{
				Upstreams: v1.UpstreamList{
					upstream,
				},
			},
		}
		matcher = &v1.Matcher{
			PathSpecifier: &v1.Matcher_Prefix{
				Prefix: "/",
			},
		}
		routes = []*v1.Route{{
			Matcher: matcher,
			Action: &v1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: utils.ResourceRefPtr(upName.Ref()),
							},
						},
					},
				},
			},
		}}
	})

	JustBeforeEach(func() {
		translator = NewTranslator(sslutils.NewSslConfigTranslator(), settings, registeredPlugins...)
		httpListener := &v1.Listener{
			Name:        "http-listener",
			BindAddress: "127.0.0.1",
			BindPort:    80,
			ListenerType: &v1.Listener_HttpListener{
				HttpListener: &v1.HttpListener{
					VirtualHosts: []*v1.VirtualHost{{
						Name:    "virt1",
						Domains: []string{"*"},
						Routes:  routes,
					}},
				},
			},
		}
		tcpListener := &v1.Listener{
			Name:        "tcp-listener",
			BindAddress: "127.0.0.1",
			BindPort:    8080,
			ListenerType: &v1.Listener_TcpListener{
				TcpListener: &v1.TcpListener{
					TcpHosts: []*v1.TcpHost{
						{
							Destination: &v1.RouteAction{
								Destination: &v1.RouteAction_Single{
									Single: &v1.Destination{
										DestinationType: &v1.Destination_Upstream{
											Upstream: &core.ResourceRef{
												Name:      "test",
												Namespace: "gloo-system",
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
		proxy = &v1.Proxy{
			Metadata: core.Metadata{
				Name:      "test",
				Namespace: "gloo-system",
			},
			Listeners: []*v1.Listener{
				httpListener,
				tcpListener,
			},
		}
	})

	It("validates the requested proxy", func() {
		s := NewValidationServer(translator)
		_ = s.Sync(context.TODO(), params.Snapshot)
		rpt, err := s.ValidateProxy(context.TODO(), &validationgrpc.ProxyValidationServiceRequest{Proxy: proxy})
		Expect(err).NotTo(HaveOccurred())
		Expect(rpt).To(Equal(&validationgrpc.ProxyValidationServiceResponse{ProxyReport: validation.MakeReport(proxy)}))
	})
})
