package translator_test

import (
	"context"
	"errors"
	"fmt"
	"os"

	types2 "github.com/onsi/gomega/types"

	_struct "github.com/golang/protobuf/ptypes/struct"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	envoy_v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/buffer/v3"
	csrf_v31 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/csrf/v3"
	v31 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	v32 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol_upgrade"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/envoyproxy/go-control-plane/pkg/wellknown"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/duration"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/wrappers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/api_conversion"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	"github.com/solo-io/gloo/projects/gloo/constants"
	gloo_envoy_core "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/core"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	v1plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	consul2 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
	v1grpc "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	v1kubernetes "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/retries"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tracing"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	. "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	mock_consul "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul/mocks"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/kubernetes"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	envoycore_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/types"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	. "github.com/solo-io/solo-kit/test/matchers"
	k8scorev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("Translator", func() {
	var (
		ctrl              *gomock.Controller
		settings          *v1.Settings
		translator        Translator
		upstream          *v1.Upstream
		upName            *core.Metadata
		proxy             *v1.Proxy
		params            plugins.Params
		registeredPlugins []plugins.Plugin
		matcher           *matchers.Matcher
		routes            []*v1.Route

		snapshot           envoycache.Snapshot
		cluster            *envoy_config_cluster_v3.Cluster
		listener           *envoy_config_listener_v3.Listener
		endpoints          envoycache.Resources
		hcmCfg             *envoyhttp.HttpConnectionManager
		routeConfiguration *envoy_config_route_v3.RouteConfiguration
		virtualHostName    string
	)

	beforeEach := func() {

		ctrl = gomock.NewController(T)

		cluster = nil
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

		upName = &core.Metadata{
			Name:      "test",
			Namespace: "gloo-system",
		}
		upstream = &v1.Upstream{
			Metadata: upName,
			UpstreamType: &v1.Upstream_Static{
				Static: &v1static.UpstreamSpec{
					Hosts: []*v1static.Host{
						{
							Addr: "Test",
							Port: 124,
						},
					},
				},
			},
		}

		params = plugins.Params{
			Ctx: context.Background(),
			Snapshot: &v1snap.ApiSnapshot{
				Endpoints: v1.EndpointList{
					{
						Upstreams: []*core.ResourceRef{upName.Ref()},
						Address:   "1.2.3.4",
						Port:      32,
						Metadata: &core.Metadata{
							Name:      "test-ep",
							Namespace: "gloo-system",
						},
					},
				},
				Upstreams: v1.UpstreamList{
					upstream,
				},
			},
		}
		matcher = &matchers.Matcher{
			PathSpecifier: &matchers.Matcher_Prefix{
				Prefix: "/",
			},
		}
		routes = []*v1.Route{{
			Name:     "testRouteName",
			Matchers: []*matchers.Matcher{matcher},
			Action: &v1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: upName.Ref(),
							},
						},
					},
				},
			},
		}}
		virtualHostName = "virt1"
	}
	BeforeEach(beforeEach)

	justBeforeEach := func() {
		pluginRegistry := registry.NewPluginRegistry(registeredPlugins)

		translator = NewTranslatorWithHasher(glooutils.NewSslConfigTranslator(), settings, pluginRegistry, EnvoyCacheResourcesListToFnvHash)
		httpListener := &v1.Listener{
			Name:        "http-listener",
			BindAddress: "127.0.0.1",
			BindPort:    80,
			ListenerType: &v1.Listener_HttpListener{
				HttpListener: &v1.HttpListener{
					VirtualHosts: []*v1.VirtualHost{{
						Name:    virtualHostName,
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
							Destination: &v1.TcpHost_TcpAction{
								Destination: &v1.TcpHost_TcpAction_Single{
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
							SslConfig: &ssl.SslConfig{
								SslSecrets: &ssl.SslConfig_SslFiles{
									SslFiles: &ssl.SSLFiles{
										TlsCert: gloohelpers.Certificate(),
										TlsKey:  gloohelpers.PrivateKey(),
									},
								},
								SniDomains: []string{
									"sni1",
								},
							},
						},
					},
				},
			},
		}
		hybridListener := &v1.Listener{
			Name:        "hybrid-listener",
			BindAddress: "127.0.0.1",
			BindPort:    8888,
			ListenerType: &v1.Listener_HybridListener{
				HybridListener: &v1.HybridListener{
					MatchedListeners: []*v1.MatchedListener{
						{
							Matcher: &v1.Matcher{
								SslConfig: &ssl.SslConfig{
									SslSecrets: &ssl.SslConfig_SslFiles{
										SslFiles: &ssl.SSLFiles{
											TlsCert: gloohelpers.Certificate(),
											TlsKey:  gloohelpers.PrivateKey(),
										},
									},
									SniDomains: []string{
										"sni1",
									},
								},
								SourcePrefixRanges: []*v3.CidrRange{
									{
										AddressPrefix: "1.2.3.4",
										PrefixLen: &wrappers.UInt32Value{
											Value: 32,
										},
									},
								},
							},
							ListenerType: &v1.MatchedListener_TcpListener{
								TcpListener: &v1.TcpListener{
									TcpHosts: []*v1.TcpHost{
										{
											Destination: &v1.TcpHost_TcpAction{
												Destination: &v1.TcpHost_TcpAction_Single{
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
											SslConfig: &ssl.SslConfig{
												SslSecrets: &ssl.SslConfig_SslFiles{
													SslFiles: &ssl.SSLFiles{
														TlsCert: gloohelpers.Certificate(),
														TlsKey:  gloohelpers.PrivateKey(),
													},
												},
												SniDomains: []string{
													"sni1",
												},
											},
										},
									},
								},
							},
						},
						{
							Matcher: &v1.Matcher{
								SslConfig: &ssl.SslConfig{
									SslSecrets: &ssl.SslConfig_SslFiles{
										SslFiles: &ssl.SSLFiles{
											TlsCert: gloohelpers.Certificate(),
											TlsKey:  gloohelpers.PrivateKey(),
										},
									},
									SniDomains: []string{
										"sni2",
									},
								},
								SourcePrefixRanges: []*v3.CidrRange{
									{
										AddressPrefix: "5.6.7.8",
										PrefixLen: &wrappers.UInt32Value{
											Value: 32,
										},
									},
								},
							},
							ListenerType: &v1.MatchedListener_HttpListener{
								HttpListener: &v1.HttpListener{
									VirtualHosts: []*v1.VirtualHost{{
										Name:    virtualHostName,
										Domains: []string{"*"},
										Routes:  routes,
									}},
								},
							},
						},
					},
				},
			},
		}
		proxy = &v1.Proxy{
			Metadata: &core.Metadata{
				Name:      "test",
				Namespace: "gloo-system",
			},
			Listeners: []*v1.Listener{
				httpListener,
				tcpListener,
				hybridListener,
			},
		}
	}
	JustBeforeEach(justBeforeEach)

	translateWithError := func() *validation.ProxyReport {
		_, errs, report := translator.Translate(params, proxy)
		ExpectWithOffset(1, errs.Validate()).To(HaveOccurred())
		return report
	}

	translateWithInvalidRoutePath := func() *validation.ProxyReport {
		_, errs, report := translator.Translate(params, proxy)
		err := errs.Validate()
		ExpectWithOffset(1, err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cannot contain [/../]"))
		return report

	}

	translateWithBuggyHasher := func() *validation.ProxyReport {
		buggyHasher := func(resources []envoycache.Resource) (uint64, error) {
			return 0, errors.New("This is a buggy hasher error")
		}
		translator = NewTranslatorWithHasher(glooutils.NewSslConfigTranslator(), settings, registry.NewPluginRegistry(registeredPlugins), buggyHasher)
		snapshot, _, report := translator.Translate(params, proxy)
		Expect(snapshot.GetResources(types.EndpointTypeV3).Version).To(Equal("endpoints-hashErr"))
		Expect(snapshot.GetResources(types.ClusterTypeV3).Version).To(Equal("clusters-hashErr"))
		Expect(snapshot.GetResources(types.ListenerTypeV3).Version).To(Equal("listeners-hashErr"))
		return report
	}

	// returns md5 Sum of current snapshot
	translate := func() {
		snap, errs, report := translator.Translate(params, proxy)
		ExpectWithOffset(1, errs.Validate()).NotTo(HaveOccurred())
		ExpectWithOffset(1, snap).NotTo(BeNil())
		ExpectWithOffset(1, report).To(Equal(validationutils.MakeReport(proxy)))

		clusters := snap.GetResources(types.ClusterTypeV3)
		clusterResource := clusters.Items[UpstreamToClusterName(upstream.Metadata.Ref())]
		cluster = clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)
		ExpectWithOffset(1, cluster).NotTo(BeNil())

		listeners := snap.GetResources(types.ListenerTypeV3)
		listenerResource := listeners.Items["http-listener"]
		listener = listenerResource.ResourceProto().(*envoy_config_listener_v3.Listener)
		ExpectWithOffset(1, listener).NotTo(BeNil())

		hcmFilter := listener.FilterChains[0].Filters[0]
		hcmCfg = &envoyhttp.HttpConnectionManager{}
		err := ParseTypedConfig(hcmFilter, hcmCfg)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		routes := snap.GetResources(types.RouteTypeV3)
		ExpectWithOffset(1, routes.Items).To(HaveKey("http-listener-routes"))
		routeResource := routes.Items["http-listener-routes"]
		routeConfiguration = routeResource.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
		ExpectWithOffset(1, routeConfiguration).NotTo(BeNil())

		endpoints = snap.GetResources(types.EndpointTypeV3)

		snapshot = snap

	}

	It("sanitizes an invalid virtual host name", func() {
		proxyClone := proto.Clone(proxy).(*v1.Proxy)
		proxyClone.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].Name = "invalid.name"

		snap, errs, report := translator.Translate(params, proxyClone)
		Expect(errs.Validate()).NotTo(HaveOccurred())
		Expect(snap).NotTo(BeNil())
		Expect(report).To(Equal(validationutils.MakeReport(proxy)))

		routes := snap.GetResources(types.RouteTypeV3)
		Expect(routes.Items).To(HaveKey("http-listener-routes"))
		routeResource := routes.Items["http-listener-routes"]
		routeConfiguration = routeResource.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
		Expect(routeConfiguration).NotTo(BeNil())
		Expect(routeConfiguration.GetVirtualHosts()).To(HaveLen(1))
		Expect(routeConfiguration.GetVirtualHosts()[0].Name).To(Equal("invalid_name"))
	})

	It("sanitizes an invalid virtual host name in a hybrid listener", func() {
		proxyClone := proto.Clone(proxy).(*v1.Proxy)
		proxyClone.GetListeners()[2].GetHybridListener().GetMatchedListeners()[1].GetHttpListener().GetVirtualHosts()[0].Name = "invalid.name"

		snap, errs, report := translator.Translate(params, proxyClone)
		Expect(errs.Validate()).NotTo(HaveOccurred())
		Expect(snap).NotTo(BeNil())
		Expect(report).To(Equal(validationutils.MakeReport(proxy)))

		routes := snap.GetResources(types.RouteTypeV3)
		expRouteConfigName := glooutils.MatchedRouteConfigName(proxyClone.GetListeners()[2], proxyClone.GetListeners()[2].GetHybridListener().GetMatchedListeners()[1].GetMatcher())
		Expect(routes.Items).To(HaveKey(expRouteConfigName))
		routeResource := routes.Items[expRouteConfigName]
		routeConfiguration = routeResource.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
		Expect(routeConfiguration).NotTo(BeNil())
		Expect(routeConfiguration.GetVirtualHosts()).To(HaveLen(1))
		Expect(routeConfiguration.GetVirtualHosts()[0].Name).To(Equal("invalid_name"))
	})

	It("translates listener options", func() {
		proxyClone := proto.Clone(proxy).(*v1.Proxy)
		proxyClone.GetListeners()[0].Options = &v1.ListenerOptions{
			PerConnectionBufferLimitBytes: &wrappers.UInt32Value{Value: 4096},
			ConnectionBalanceConfig: &v1.ConnectionBalanceConfig{
				ExactBalance: &v1.ConnectionBalanceConfig_ExactBalance{},
			},
		}

		snap, errs, report := translator.Translate(params, proxyClone)
		Expect(errs.Validate()).NotTo(HaveOccurred())
		Expect(snap).NotTo(BeNil())
		Expect(report).To(Equal(validationutils.MakeReport(proxy)))

		listeners := snap.GetResources(types.ListenerTypeV3)
		Expect(listeners.Items).To(HaveKey("http-listener"))
		listenerResource := listeners.Items["http-listener"]
		listenerConfiguration := listenerResource.ResourceProto().(*envoy_config_listener_v3.Listener)
		Expect(listenerConfiguration).NotTo(BeNil())
		Expect(listenerConfiguration.PerConnectionBufferLimitBytes).To(MatchProto(&wrappers.UInt32Value{Value: 4096}))
		Expect(listenerConfiguration.GetConnectionBalanceConfig().GetExactBalance()).To(Not(BeNil()))
		Expect(listenerConfiguration.GetConnectionBalanceConfig()).To(MatchProto(&envoy_config_listener_v3.Listener_ConnectionBalanceConfig{
			BalanceType: &envoy_config_listener_v3.Listener_ConnectionBalanceConfig_ExactBalance_{
				ExactBalance: &envoy_config_listener_v3.Listener_ConnectionBalanceConfig_ExactBalance{},
			},
		}))
	})

	Context("Auth configs", func() {
		It("will error if auth config is missing", func() {
			proxyClone := proto.Clone(proxy).(*v1.Proxy)
			proxyClone.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].Options =
				&v1.VirtualHostOptions{
					Extauth: &extauth.ExtAuthExtension{
						Spec: &extauth.ExtAuthExtension_ConfigRef{
							ConfigRef: &core.ResourceRef{},
						},
					},
				}

			_, errs, _ := translator.Translate(params, proxyClone)
			Expect(errs.Validate()).To(HaveOccurred())
			Expect(errs.Validate().Error()).To(ContainSubstring("VirtualHost Error: ProcessingError. Reason: auth config not found:"))
		})

		It("will error if auth config is missing from a hybrid listener", func() {
			proxyClone := proto.Clone(proxy).(*v1.Proxy)
			proxyClone.GetListeners()[2].GetHybridListener().GetMatchedListeners()[1].GetHttpListener().GetVirtualHosts()[0].Options =
				&v1.VirtualHostOptions{
					Extauth: &extauth.ExtAuthExtension{
						Spec: &extauth.ExtAuthExtension_ConfigRef{
							ConfigRef: &core.ResourceRef{},
						},
					},
				}

			_, errs, _ := translator.Translate(params, proxyClone)
			Expect(errs.Validate()).To(HaveOccurred())
			Expect(errs.Validate().Error()).To(ContainSubstring("VirtualHost Error: ProcessingError. Reason: auth config not found:"))
		})
	})

	Context("service spec", func() {
		It("changes in service spec should create a different snapshot", func() {
			translate()
			oldVersion := snapshot.GetResources(types.ClusterTypeV3).Version

			svcSpec := &v1plugins.ServiceSpec{
				PluginType: &v1plugins.ServiceSpec_Grpc{
					Grpc: &v1grpc.ServiceSpec{},
				},
			}
			upstream.UpstreamType.(*v1.Upstream_Static).SetServiceSpec(svcSpec)
			translate()
			newVersion := snapshot.GetResources(types.ClusterTypeV3).Version
			Expect(oldVersion).ToNot(Equal(newVersion))
		})
	})

	Context("route no path", func() {
		BeforeEach(func() {
			matcher.PathSpecifier = nil
			matcher.Headers = []*matchers.HeaderMatcher{
				{
					Name: "test",
				},
			}
		})
		It("should error when path match is missing", func() {
			_, errs, report := translator.Translate(params, proxy)
			Expect(errs.Validate()).To(HaveOccurred())
			invalidMatcherName := fmt.Sprintf("%s-route-0", virtualHostName)
			Expect(errs.Validate().Error()).To(ContainSubstring(fmt.Sprintf("Route Error: InvalidMatcherError. Reason: no path specifier provided. Route Name: %s", invalidMatcherName)))
			expectedReport := validationutils.MakeReport(proxy)
			expectedReport.ListenerReports[0].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport.VirtualHostReports[0].RouteReports[0].Errors = []*validation.RouteReport_Error{
				{
					Type:   validation.RouteReport_Error_InvalidMatcherError,
					Reason: fmt.Sprintf("no path specifier provided. Route Name: %s", invalidMatcherName),
				},
			}
			Expect(report.ListenerReports[0]).To(Equal(expectedReport.ListenerReports[0])) // hybridReports may not match due to map order
		})
		It("should error when path match is missing even if we have grpc spec", func() {
			dest := routes[0].GetRouteAction().GetSingle()
			dest.DestinationSpec = &v1.DestinationSpec{
				DestinationType: &v1.DestinationSpec_Grpc{
					Grpc: &v1grpc.DestinationSpec{
						Package:  "glootest",
						Function: "TestMethod",
						Service:  "TestService",
					},
				},
			}
			_, errs, report := translator.Translate(params, proxy)
			Expect(errs.Validate()).To(HaveOccurred())
			invalidMatcherName := fmt.Sprintf("%s-route-0", virtualHostName)
			processingErrorName := fmt.Sprintf("%s-route-0-%s-matcher-0", virtualHostName, routes[0].Name)
			Expect(errs.Validate().Error()).To(ContainSubstring(
				fmt.Sprintf(
					"Route Error: InvalidMatcherError. Reason: no path specifier provided. Route Name: %s; Route Error: ProcessingError. Reason: *grpc.plugin: missing path for grpc route. Route Name: %s",
					invalidMatcherName,
					processingErrorName,
				)))

			expectedReport := validationutils.MakeReport(proxy)
			expectedReport.ListenerReports[0].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport.VirtualHostReports[0].RouteReports[0].Errors = []*validation.RouteReport_Error{
				{
					Type:   validation.RouteReport_Error_InvalidMatcherError,
					Reason: fmt.Sprintf("no path specifier provided. Route Name: %s", invalidMatcherName),
				},
				{
					Type:   validation.RouteReport_Error_ProcessingError,
					Reason: fmt.Sprintf("*grpc.plugin: missing path for grpc route. Route Name: %s", processingErrorName),
				},
			}
			Expect(report.ListenerReports[0]).To(Equal(expectedReport.ListenerReports[0]))
		})
	})

	Context("route path match - sensitive case", func() {
		It("should translate path matcher with sensitive case", func() {

			routes[0].Matchers = []*matchers.Matcher{
				{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/foo",
					},
					CaseSensitive: &wrappers.BoolValue{Value: true},
				},
			}

			translate()
			fooRoute := routeConfiguration.VirtualHosts[0].Routes[0]
			Expect(fooRoute.Match.GetPrefix()).To(Equal("/foo"))
			Expect(fooRoute.Match.CaseSensitive.Value).To(Equal(true))
		})

		It("should translate path matcher with case insensitive", func() {

			routes[0].Matchers = []*matchers.Matcher{
				{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/foo",
					},
					CaseSensitive: &wrappers.BoolValue{Value: false},
				},
			}

			translate()
			fooRoute := routeConfiguration.VirtualHosts[0].Routes[0]
			Expect(fooRoute.Match.GetPrefix()).To(Equal("/foo"))
			Expect(fooRoute.Match.CaseSensitive).To(Equal(&wrappers.BoolValue{Value: false}))
		})

		It("should translate path matcher with regex rewrite on redirectAction", func() {

			glooRoute := &v1.Route{
				Action: &v1.Route_RedirectAction{
					RedirectAction: &v1.RedirectAction{
						PathRewriteSpecifier: &v1.RedirectAction_RegexRewrite{
							RegexRewrite: &v31.RegexMatchAndSubstitute{
								Pattern: &v31.RegexMatcher{
									EngineType: &v31.RegexMatcher_GoogleRe2{},
									Regex:      "/redirect",
								},
								Substitution: "/target",
							},
						},
						ResponseCode: 400,
					},
				},
				Matchers: []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/matchprefix",
						},
						CaseSensitive: &wrappers.BoolValue{Value: false},
					},
				},
			}
			routes[0] = glooRoute

			expectedRedirectAction := &envoy_config_route_v3.Route_Redirect{

				Redirect: &envoy_config_route_v3.RedirectAction{
					PathRewriteSpecifier: &envoy_config_route_v3.RedirectAction_RegexRewrite{
						RegexRewrite: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
							Pattern: &envoy_type_matcher_v3.RegexMatcher{
								Regex: "/redirect",
							},
							Substitution: "/target",
						},
					},

					//PathRewriteSpecifier: envoy_config_route_v3.RedirectAction_RegexRewrite{},
					ResponseCode: 301,
					StripQuery:   false,
				},
			}

			translate()
			envoyRoute := routeConfiguration.VirtualHosts[0].Routes[0]
			Expect(envoyRoute.Match.GetPrefix()).To(Equal("/matchprefix"))
			Expect(envoyRoute.Action).To(BeAssignableToTypeOf(expectedRedirectAction))
			actualRegexRedirect := envoyRoute.Action.(*envoy_config_route_v3.Route_Redirect).Redirect.GetRegexRewrite()
			Expect(actualRegexRedirect.Pattern.Regex).To(Equal(expectedRedirectAction.Redirect.GetRegexRewrite().Pattern.Regex))
			Expect(actualRegexRedirect.Substitution).To(Equal(expectedRedirectAction.Redirect.GetRegexRewrite().Substitution))
			Expect(envoyRoute.Match.CaseSensitive).To(Equal(&wrappers.BoolValue{Value: false}))
		})
	})

	Context("invalid route paths", func() {
		It("should report an invalid path redirect", func() {
			glooRoute := &v1.Route{
				Action: &v1.Route_RedirectAction{
					RedirectAction: &v1.RedirectAction{
						PathRewriteSpecifier: &v1.RedirectAction_PathRedirect{
							// invalid sequence
							PathRedirect: "/../../home/secretdata",
						},
					},
				},
			}
			routes[0] = glooRoute
			translateWithInvalidRoutePath()
		})
		It("should report an invalid prefix rewrite", func() {
			glooRoute := &v1.Route{
				Action: &v1.Route_RedirectAction{
					RedirectAction: &v1.RedirectAction{
						PathRewriteSpecifier: &v1.RedirectAction_PrefixRewrite{
							PrefixRewrite: "home/../secret",
						},
					},
				},
			}
			routes[0] = glooRoute
			translateWithInvalidRoutePath()
		})

		It("should report an invalid host redirect", func() {
			glooRoute := &v1.Route{
				Action: &v1.Route_RedirectAction{
					RedirectAction: &v1.RedirectAction{
						HostRedirect: "otherhost/../secret",
					},
				},
			}
			routes[0] = glooRoute
			translateWithInvalidRoutePath()
		})

		It("should report an invalid prefix rewrite", func() {
			glooRoute := &v1.Route{
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &v1.Destination{
								DestinationType: &v1.Destination_Upstream{
									Upstream: &core.ResourceRef{
										Name:      "somename",
										Namespace: "someNamespace",
									},
								},
							},
						},
					},
				},
				Options: &v1.RouteOptions{
					PrefixRewrite: &wrapperspb.StringValue{Value: "/home/../secret"},
				},
			}
			routes[0] = glooRoute
			translateWithInvalidRoutePath()
		})

		It("should report an invalid prefix", func() {
			glooRoute := &v1.Route{
				Matchers: []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "home/../secret",
						},
					},
				},
			}
			routes[0] = glooRoute
			translateWithInvalidRoutePath()
		})

		It("should report an invalid prefix", func() {
			glooRoute := &v1.Route{
				Matchers: []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Exact{
							Exact: "home/../secret",
						},
					},
				},
			}
			routes[0] = glooRoute
			translateWithInvalidRoutePath()
		})
	})

	Context("route header match", func() {
		It("should translate header matcher with no value to a PresentMatch", func() {

			matcher.Headers = []*matchers.HeaderMatcher{
				{
					Name: "test",
				},
			}
			translate()
			headerMatch := routeConfiguration.VirtualHosts[0].Routes[0].Match.Headers[0]
			Expect(headerMatch.Name).To(Equal("test"))
			Expect(headerMatch.InvertMatch).To(Equal(false))
			presentMatch := headerMatch.GetPresentMatch()
			Expect(presentMatch).To(BeTrue())
		})

		It("should translate header matcher with value to exact match", func() {

			matcher.Headers = []*matchers.HeaderMatcher{
				{
					Name:  "test",
					Value: "testvalue",
				},
			}
			translate()

			headerMatch := routeConfiguration.VirtualHosts[0].Routes[0].Match.Headers[0]
			Expect(headerMatch.Name).To(Equal("test"))
			Expect(headerMatch.InvertMatch).To(Equal(false))
			exactMatch := headerMatch.GetExactMatch()
			Expect(exactMatch).To(Equal("testvalue"))
		})

		It("should translate header matcher with regex becomes regex match", func() {

			matcher.Headers = []*matchers.HeaderMatcher{
				{
					Name:  "test",
					Value: "testvalue",
					Regex: true,
				},
			}
			translate()

			headerMatch := routeConfiguration.VirtualHosts[0].Routes[0].Match.Headers[0]
			Expect(headerMatch.Name).To(Equal("test"))
			Expect(headerMatch.InvertMatch).To(Equal(false))
			regex := headerMatch.GetSafeRegexMatch().GetRegex()
			Expect(regex).To(Equal("testvalue"))
		})

		It("should translate header matcher with regex becomes regex match, with non default program size", func() {

			settings := &v1.Settings{
				Gloo: &v1.GlooOptions{
					RegexMaxProgramSize: &wrappers.UInt32Value{Value: 200},
				},
			}
			params.Ctx = settingsutil.WithSettings(params.Ctx, settings)

			matcher.Headers = []*matchers.HeaderMatcher{
				{
					Name:  "test",
					Value: "testvalue",
					Regex: true,
				},
			}
			translate()

			headerMatch := routeConfiguration.VirtualHosts[0].Routes[0].Match.Headers[0]
			Expect(headerMatch.Name).To(Equal("test"))
			Expect(headerMatch.InvertMatch).To(Equal(false))
			regex := headerMatch.GetSafeRegexMatch().GetRegex()
			Expect(regex).To(Equal("testvalue"))
			maxsize := headerMatch.GetSafeRegexMatch().GetGoogleRe2().GetMaxProgramSize().GetValue()
			Expect(maxsize).To(BeEquivalentTo(200))
		})

		It("should translate header matcher logic inversion flag", func() {

			matcher.Headers = []*matchers.HeaderMatcher{
				{
					Name:        "test",
					InvertMatch: true,
				},
			}
			translate()

			headerMatch := routeConfiguration.VirtualHosts[0].Routes[0].Match.Headers[0]
			Expect(headerMatch.InvertMatch).To(Equal(true))
		})

		It("should default to '/' prefix matcher if none is provided", func() {
			matcher = nil
			translate()

			prefix := routeConfiguration.VirtualHosts[0].Routes[0].Match.GetPrefix()
			Expect(prefix).To(Equal("/"))
		})

		Context("Multiple matchers", func() {
			BeforeEach(func() {
				routes[0].Matchers = []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/foo",
						},
					},
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/bar",
						},
					},
				}
			})

			It("should translate multiple matchers on a single Gloo route to multiple Envoy routes", func() {
				translate()
				fooRoute := routeConfiguration.VirtualHosts[0].Routes[0]
				barRoute := routeConfiguration.VirtualHosts[0].Routes[1]

				fooPrefix := fooRoute.Match.GetPrefix()
				barPrefix := barRoute.Match.GetPrefix()

				Expect(fooPrefix).To(Equal("/foo"))
				Expect(barPrefix).To(Equal("/bar"))

				Expect(fooRoute.Name).To(MatchRegexp("testRouteName-[0-9]*"))
				Expect(barRoute.Name).To(MatchRegexp("testRouteName-[0-9]*"))

				// the routes should be otherwise identical. wipe the matchers and names and compare them
				fooRoute.Match = &envoy_config_route_v3.RouteMatch{}
				barRoute.Match = &envoy_config_route_v3.RouteMatch{}
				fooRoute.Name = ""
				barRoute.Name = ""

				Expect(fooRoute).To(Equal(barRoute))
			})
		})
	})

	Context("non route_routeaction routes", func() {
		var options *v1.RouteOptions
		BeforeEach(func() {
			// specify routeOptions which can be processed on non route_routeaction routes
			options = &v1.RouteOptions{
				Transformations:    &transformation.Transformations{ClearRouteCache: true},
				Tracing:            &tracing.RouteTracingSettings{},
				HeaderManipulation: &headers.HeaderManipulation{RequestHeadersToRemove: []string{"test-header"}},
				BufferPerRoute: &envoy_v3.BufferPerRoute{
					Override: &envoy_v3.BufferPerRoute_Disabled{
						Disabled: true,
					},
				},
				Csrf: &csrf_v31.CsrfPolicy{
					FilterEnabled: &v3.RuntimeFractionalPercent{
						RuntimeKey: "test-key",
					},
				},
				EnvoyMetadata: map[string]*_struct.Struct{
					"test-struct": {
						Fields: map[string]*_struct.Value{
							"test-field": {
								Kind: &_struct.Value_NumberValue{
									NumberValue: 123,
								},
							},
						},
					},
				},
			}
			redirectRoute := &v1.Route{
				Action: &v1.Route_RedirectAction{
					RedirectAction: &v1.RedirectAction{
						ResponseCode: 400,
					},
				},
				Options: options,
			}
			directResponseRoute := &v1.Route{
				Action: &v1.Route_DirectResponseAction{
					DirectResponseAction: &v1.DirectResponseAction{
						Status: 400,
					},
				},
				Options: options,
			}
			routes = []*v1.Route{redirectRoute, directResponseRoute}
		})

		It("can process routeOptions properly", func() {
			translate()
			for _, route := range routeConfiguration.VirtualHosts[0].Routes {
				Expect(route.TypedPerFilterConfig).NotTo(BeNil())
				Expect(route.TypedPerFilterConfig).To(HaveKey("io.solo.transformation"))
				Expect(route.TypedPerFilterConfig).To(HaveKey("envoy.filters.http.buffer"))
				Expect(route.TypedPerFilterConfig).To(HaveKey("envoy.filters.http.csrf"))
				Expect(route.Tracing).NotTo(BeNil())
				Expect(route.RequestHeadersToRemove).To(HaveLen(1))
				Expect(route.Metadata).NotTo(BeNil())
			}
		})

		It("does not affect envoy config when options only processed on routeActions are specified", func() {
			// translate the config specified in the BeforeEach block and store the routeConfig
			translate()
			oldRouteConfig := routeConfiguration.VirtualHosts[0].Routes

			// append unprocessed settings to the routeConfig
			invalidOptions := options
			invalidOptions.Faults = &faultinjection.RouteFaults{
				Abort: &faultinjection.RouteAbort{Percentage: 50, HttpStatus: 401},
			}
			invalidOptions.PrefixRewrite = &wrappers.StringValue{Value: "test"}
			invalidOptions.Timeout = &duration.Duration{Seconds: 1}
			invalidOptions.Retries = &retries.RetryPolicy{RetryOn: "test"}
			invalidOptions.HostRewriteType = &v1.RouteOptions_HostRewrite{HostRewrite: "test"}
			invalidOptions.Extensions = &v1.Extensions{Configs: map[string]*_struct.Struct{
				"test-struct": {
					Fields: map[string]*_struct.Value{
						"test-field": {
							Kind: &_struct.Value_NumberValue{
								NumberValue: 123,
							},
						},
					},
				},
			}}
			invalidOptions.Upgrades = []*protocol_upgrade.ProtocolUpgradeConfig{
				{UpgradeType: &protocol_upgrade.ProtocolUpgradeConfig_Websocket{
					Websocket: &protocol_upgrade.ProtocolUpgradeConfig_ProtocolUpgradeSpec{
						Enabled: &wrapperspb.BoolValue{Value: true},
					},
				}},
			}
			invalidOptions.RatelimitBasic = &ratelimit.IngressRateLimit{
				AuthorizedLimits: &v1alpha1.RateLimit{
					Unit:            v1alpha1.RateLimit_SECOND,
					RequestsPerUnit: 1,
				},
			}
			invalidOptions.RateLimitEarlyConfigType = &v1.RouteOptions_RatelimitEarly{
				RatelimitEarly: &ratelimit.RateLimitRouteExtension{
					IncludeVhRateLimits: true,
				},
			}
			invalidOptions.RegexRewrite = &v32.RegexMatchAndSubstitute{
				Pattern: &v32.RegexMatcher{
					Regex: "Test",
				},
			}

			for _, route := range proxy.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].GetRoutes() {
				route.Options = invalidOptions
			}

			// re-run translation and confirm that the new settings do not affect envoy config
			translate()
			newRouteConfig := routeConfiguration.VirtualHosts[0].Routes
			Expect(oldRouteConfig).To(BeEquivalentTo(newRouteConfig))
		})
	})

	Context("Health check config", func() {

		It("will error if required field is nil", func() {
			upstream.HealthChecks = []*gloo_envoy_core.HealthCheck{
				{
					Interval: DefaultHealthCheckInterval,
				},
			}
			report := translateWithError()

			Expect(report).To(Equal(validationutils.MakeReport(proxy)))
		})

		It("will error if no health checker is supplied", func() {
			upstream.HealthChecks = []*gloo_envoy_core.HealthCheck{
				{
					Timeout:            DefaultHealthCheckTimeout,
					Interval:           DefaultHealthCheckInterval,
					HealthyThreshold:   DefaultThreshold,
					UnhealthyThreshold: DefaultThreshold,
				},
			}
			report := translateWithError()
			Expect(report).To(Equal(validationutils.MakeReport(proxy)))
		})

		It("can translate the http health check", func() {
			expectedResult := []*envoy_config_core_v3.HealthCheck{
				{
					Timeout:            DefaultHealthCheckTimeout,
					Interval:           DefaultHealthCheckInterval,
					HealthyThreshold:   DefaultThreshold,
					UnhealthyThreshold: DefaultThreshold,
					HealthChecker: &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
						HttpHealthCheck: &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
							Host: "host",
							Path: "path",
							ServiceNameMatcher: &envoy_type_matcher_v3.StringMatcher{
								MatchPattern: &envoy_type_matcher_v3.StringMatcher_Prefix{
									Prefix: "svc",
								},
							},
							RequestHeadersToAdd:    []*envoy_config_core_v3.HeaderValueOption{},
							RequestHeadersToRemove: []string{},
							CodecClientType:        envoy_type_v3.CodecClientType_HTTP2,
							ExpectedStatuses:       []*envoy_type_v3.Int64Range{},
							Method:                 envoy_config_core_v3.RequestMethod_POST,
						},
					},
				},
			}
			var err error
			upstream.HealthChecks, err = api_conversion.ToGlooHealthCheckList(expectedResult)
			Expect(err).NotTo(HaveOccurred())
			translate()
			var msgList []proto.Message
			for _, v := range expectedResult {
				msgList = append(msgList, v)
			}
			Expect(cluster.HealthChecks).To(ConsistOfProtos(msgList...))

			By("rejects http health checkers with CONNECT method")
			expectedResult[0].GetHttpHealthCheck().Method = envoy_config_core_v3.RequestMethod_CONNECT
			upstream.HealthChecks, err = api_conversion.ToGlooHealthCheckList(expectedResult)
			Expect(err).NotTo(HaveOccurred())
			_, errs, _ := translator.Translate(params, proxy)
			_, usReport := errs.Find("*v1.Upstream", upstream.Metadata.Ref())
			Expect(usReport.Errors).To(Not(BeNil()))
			Expect(usReport.Errors.Error()).To(ContainSubstring("method CONNECT is not allowed on http health checkers"))
		})

		It("can translate the grpc health check", func() {
			expectedResult := []*envoy_config_core_v3.HealthCheck{
				{
					Timeout:            DefaultHealthCheckTimeout,
					Interval:           DefaultHealthCheckInterval,
					HealthyThreshold:   DefaultThreshold,
					UnhealthyThreshold: DefaultThreshold,
					HealthChecker: &envoy_config_core_v3.HealthCheck_GrpcHealthCheck_{
						GrpcHealthCheck: &envoy_config_core_v3.HealthCheck_GrpcHealthCheck{
							ServiceName:     "svc",
							Authority:       "authority",
							InitialMetadata: []*envoy_config_core_v3.HeaderValueOption{},
						},
					},
				},
			}
			var err error
			upstream.HealthChecks, err = api_conversion.ToGlooHealthCheckList(expectedResult)
			Expect(err).NotTo(HaveOccurred())
			translate()
			var msgList []proto.Message
			for _, v := range expectedResult {
				msgList = append(msgList, v)
			}
			Expect(cluster.HealthChecks).To(ConsistOfProtos(msgList...))
		})

		It("can properly translate outlier detection config", func() {
			dur := &duration.Duration{Seconds: 1}
			expectedResult := &envoy_config_cluster_v3.OutlierDetection{
				Consecutive_5Xx:                        DefaultThreshold,
				Interval:                               dur,
				BaseEjectionTime:                       dur,
				MaxEjectionPercent:                     DefaultThreshold,
				EnforcingConsecutive_5Xx:               DefaultThreshold,
				EnforcingSuccessRate:                   DefaultThreshold,
				SuccessRateMinimumHosts:                DefaultThreshold,
				SuccessRateRequestVolume:               DefaultThreshold,
				SuccessRateStdevFactor:                 nil,
				ConsecutiveGatewayFailure:              DefaultThreshold,
				EnforcingConsecutiveGatewayFailure:     nil,
				SplitExternalLocalOriginErrors:         true,
				ConsecutiveLocalOriginFailure:          nil,
				EnforcingConsecutiveLocalOriginFailure: nil,
				EnforcingLocalOriginSuccessRate:        nil,
			}
			upstream.OutlierDetection = api_conversion.ToGlooOutlierDetection(expectedResult)
			translate()
			Expect(cluster.OutlierDetection).To(MatchProto(expectedResult))
		})

		It("can properly validate outlier detection config", func() {
			expectedResult := &envoy_config_cluster_v3.OutlierDetection{}
			upstream.OutlierDetection = api_conversion.ToGlooOutlierDetection(expectedResult)
			report := translateWithError()
			Expect(report).To(Equal(validationutils.MakeReport(proxy)))
		})

		It("doesnt add resources to snapshot if hashing error", func() {
			params = plugins.Params{
				Ctx:      context.Background(),
				Snapshot: &v1snap.ApiSnapshot{},
			}
			translateWithBuggyHasher()
		})

		Context("Healthcheck with Forbidden headers", func() {
			var healthChecks []*gloo_envoy_core.HealthCheck

			BeforeEach(func() {
				healthChecks = []*gloo_envoy_core.HealthCheck{
					{
						Timeout:            DefaultHealthCheckTimeout,
						Interval:           DefaultHealthCheckInterval,
						HealthyThreshold:   DefaultThreshold,
						UnhealthyThreshold: DefaultThreshold,
					},
				}
				Expect(healthChecks[0].HealthChecker).To(BeNil())
			})

			DescribeTable("http health check", func(key string, expectError bool) {
				upstream.HealthChecks = healthChecks
				healthChecks[0].HealthChecker = &gloo_envoy_core.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &gloo_envoy_core.HealthCheck_HttpHealthCheck{
						RequestHeadersToAdd: []*envoycore_sk.HeaderValueOption{
							{
								HeaderOption: &envoycore_sk.HeaderValueOption_Header{
									Header: &envoycore_sk.HeaderValue{
										Key:   key,
										Value: "value",
									},
								},
								Append: &wrappers.BoolValue{
									Value: true,
								},
							},
						},
					},
				}
				_, errs, _ := translator.Translate(params, proxy)

				if expectError {
					Expect(errs.Validate()).To(MatchError(ContainSubstring(": -prefixed or host headers may not be modified")))
					return
				}
				Expect(errs.Validate()).NotTo(HaveOccurred())
			},
				Entry("Allowed header", "some-header", false),
				Entry(":-prefixed header", ":path", true))

			DescribeTable("grpc health check", func(key string, expectError bool) {
				upstream.HealthChecks = healthChecks
				healthChecks[0].HealthChecker = &gloo_envoy_core.HealthCheck_GrpcHealthCheck_{
					GrpcHealthCheck: &gloo_envoy_core.HealthCheck_GrpcHealthCheck{
						InitialMetadata: []*envoycore_sk.HeaderValueOption{
							{
								HeaderOption: &envoycore_sk.HeaderValueOption_Header{
									Header: &envoycore_sk.HeaderValue{
										Key:   key,
										Value: "value",
									},
								},
								Append: &wrappers.BoolValue{
									Value: true,
								},
							},
						},
					},
				}
				_, errs, _ := translator.Translate(params, proxy)

				if expectError {
					Expect(errs.Validate()).To(MatchError(ContainSubstring(": -prefixed or host headers may not be modified")))
					return
				}
				Expect(errs.Validate()).NotTo(HaveOccurred())
			},
				Entry("Allowed header", "some-header", false),
				Entry("host header", "host", true))
		})

		Context("Health checks with secret header", func() {
			var expectedResult []*envoy_config_core_v3.HealthCheck
			var expectedHeaders []*envoy_config_core_v3.HeaderValueOption
			var upstreamHeaders []*envoycore_sk.HeaderValueOption

			BeforeEach(func() {
				params.Snapshot.Secrets = v1.SecretList{
					{
						Kind: &v1.Secret_Header{
							Header: &v1.HeaderSecret{
								Headers: map[string]string{
									"Authorization": "basic dXNlcjpwYXNzd29yZA==",
								},
							},
						},
						Metadata: &core.Metadata{
							Name: "foo",
						},
					},
				}

				expectedHeaders = []*envoy_config_core_v3.HeaderValueOption{
					{
						Header: &envoy_config_core_v3.HeaderValue{
							Key:   "Authorization",
							Value: "basic dXNlcjpwYXNzd29yZA==",
						},
						AppendAction: envoy_config_core_v3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
					},
				}

				upstreamHeaders = []*envoycore_sk.HeaderValueOption{
					{
						HeaderOption: &envoycore_sk.HeaderValueOption_HeaderSecretRef{
							HeaderSecretRef: &core.ResourceRef{
								Name: "foo",
							},
						},
						Append: &wrappers.BoolValue{
							Value: true,
						},
					},
				}

				expectedResult = []*envoy_config_core_v3.HealthCheck{
					{
						Timeout:            DefaultHealthCheckTimeout,
						Interval:           DefaultHealthCheckInterval,
						HealthyThreshold:   DefaultThreshold,
						UnhealthyThreshold: DefaultThreshold,
					},
				}
				Expect(expectedResult[0].HealthChecker).To(BeNil())
			})

			AfterEach(os.Clearenv)

			translate := func(expectError bool) {
				snap, errs, report := translator.Translate(params, proxy)
				if expectError {
					Expect(errs.Validate()).To(MatchError(ContainSubstring("list did not find secret bar.foo")))
					return
				}
				Expect(errs.Validate()).NotTo(HaveOccurred())
				Expect(snap).NotTo(BeNil())
				Expect(report).To(Equal(validationutils.MakeReport(proxy)))

				clusters := snap.GetResources(types.ClusterTypeV3)
				clusterResource := clusters.Items[UpstreamToClusterName(upstream.Metadata.Ref())]
				cluster = clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)
				Expect(cluster).NotTo(BeNil())
				var msgList []proto.Message
				for _, v := range expectedResult {
					msgList = append(msgList, v)
				}
				Expect(cluster.HealthChecks).To(ConsistOfProtos(msgList...))
			}

			// Checks to ensure that https://github.com/solo-io/gloo/pull/8505 works as expected.
			// It checks whether headerSecretRef and the upstream that the secret is sent to have matching namespaces
			// if configured to do so and the code to do so is shared across both the http && grpc healt check.
			// The test cases have been split between the http and grpc health checks as it relies on shared code,
			// avoids test duplication and to ensure they both work as expected.
			DescribeTable("http health check", func(enforceMatch, secretNamespace string, expectError bool) {
				err := os.Setenv(api_conversion.MatchingNamespaceEnv, enforceMatch)
				Expect(err).NotTo(HaveOccurred())

				params.Snapshot.Secrets[0].Metadata.Namespace = secretNamespace
				expectedResult[0].HealthChecker = &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
						Host: "host",
						Path: "path",
						ServiceNameMatcher: &envoy_type_matcher_v3.StringMatcher{
							MatchPattern: &envoy_type_matcher_v3.StringMatcher_Prefix{
								Prefix: "svc",
							},
						},
						RequestHeadersToAdd:    []*envoy_config_core_v3.HeaderValueOption{},
						RequestHeadersToRemove: []string{},
						CodecClientType:        envoy_type_v3.CodecClientType_HTTP2,
						ExpectedStatuses:       []*envoy_type_v3.Int64Range{},
					},
				}

				upstream.HealthChecks, err = api_conversion.ToGlooHealthCheckList(expectedResult)
				Expect(err).NotTo(HaveOccurred())

				expectedResult[0].GetHttpHealthCheck().RequestHeadersToAdd = expectedHeaders
				upstream.GetHealthChecks()[0].GetHttpHealthCheck().RequestHeadersToAdd = upstreamHeaders
				upstream.GetHealthChecks()[0].GetHttpHealthCheck().RequestHeadersToAdd[0].GetHeaderSecretRef().Namespace = secretNamespace

				translate(expectError)
			},
				Entry("Matching enforced and namespaces match", "true", "gloo-system", false),
				Entry("Matching not enforced and namespaces match", "false", "gloo-system", false))

			DescribeTable("grpc health check", func(enforceMatch, secretNamespace string, expectError bool) {
				err := os.Setenv(api_conversion.MatchingNamespaceEnv, enforceMatch)
				Expect(err).NotTo(HaveOccurred())

				params.Snapshot.Secrets[0].Metadata.Namespace = secretNamespace
				expectedResult[0].HealthChecker = &envoy_config_core_v3.HealthCheck_GrpcHealthCheck_{
					GrpcHealthCheck: &envoy_config_core_v3.HealthCheck_GrpcHealthCheck{
						ServiceName:     "svc",
						Authority:       "authority",
						InitialMetadata: []*envoy_config_core_v3.HeaderValueOption{},
					},
				}

				upstream.HealthChecks, err = api_conversion.ToGlooHealthCheckList(expectedResult)
				Expect(err).NotTo(HaveOccurred())

				expectedResult[0].GetGrpcHealthCheck().InitialMetadata = expectedHeaders
				upstream.GetHealthChecks()[0].GetGrpcHealthCheck().InitialMetadata = upstreamHeaders
				upstream.GetHealthChecks()[0].GetGrpcHealthCheck().InitialMetadata[0].GetHeaderSecretRef().Namespace = secretNamespace

				translate(expectError)
			},
				Entry("Matching not enforced and namespaces don't match", "false", "bar", false),
				Entry("Matching enforced and namespaces don't match", "true", "bar", true))
		})
	})

	Context("circuit breakers", func() {

		It("should NOT translate circuit breakers on upstream", func() {
			translate()
			Expect(cluster.CircuitBreakers).To(BeNil())
		})

		It("should translate circuit breakers on upstream", func() {

			upstream.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &wrappers.UInt32Value{Value: 1},
				MaxPendingRequests: &wrappers.UInt32Value{Value: 2},
				MaxRequests:        &wrappers.UInt32Value{Value: 3},
				MaxRetries:         &wrappers.UInt32Value{Value: 4},
			}

			expectedCircuitBreakers := &envoy_config_cluster_v3.CircuitBreakers{
				Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{
					{
						MaxConnections:     &wrappers.UInt32Value{Value: 1},
						MaxPendingRequests: &wrappers.UInt32Value{Value: 2},
						MaxRequests:        &wrappers.UInt32Value{Value: 3},
						MaxRetries:         &wrappers.UInt32Value{Value: 4},
					},
				},
			}
			translate()

			Expect(cluster.CircuitBreakers).To(MatchProto(expectedCircuitBreakers))
		})

		It("should translate circuit breakers on settings", func() {

			settings.Gloo = &v1.GlooOptions{}
			settings.Gloo.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &wrappers.UInt32Value{Value: 1},
				MaxPendingRequests: &wrappers.UInt32Value{Value: 2},
				MaxRequests:        &wrappers.UInt32Value{Value: 3},
				MaxRetries:         &wrappers.UInt32Value{Value: 4},
			}

			expectedCircuitBreakers := &envoy_config_cluster_v3.CircuitBreakers{
				Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{
					{
						MaxConnections:     &wrappers.UInt32Value{Value: 1},
						MaxPendingRequests: &wrappers.UInt32Value{Value: 2},
						MaxRequests:        &wrappers.UInt32Value{Value: 3},
						MaxRetries:         &wrappers.UInt32Value{Value: 4},
					},
				},
			}
			translate()

			Expect(cluster.CircuitBreakers).To(MatchProto(expectedCircuitBreakers))
		})

		It("should override circuit breakers on upstream", func() {

			settings.Gloo = &v1.GlooOptions{}
			settings.Gloo.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &wrappers.UInt32Value{Value: 11},
				MaxPendingRequests: &wrappers.UInt32Value{Value: 12},
				MaxRequests:        &wrappers.UInt32Value{Value: 13},
				MaxRetries:         &wrappers.UInt32Value{Value: 14},
			}

			upstream.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &wrappers.UInt32Value{Value: 1},
				MaxPendingRequests: &wrappers.UInt32Value{Value: 2},
				MaxRequests:        &wrappers.UInt32Value{Value: 3},
				MaxRetries:         &wrappers.UInt32Value{Value: 4},
			}

			expectedCircuitBreakers := &envoy_config_cluster_v3.CircuitBreakers{
				Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{
					{
						MaxConnections:     &wrappers.UInt32Value{Value: 1},
						MaxPendingRequests: &wrappers.UInt32Value{Value: 2},
						MaxRequests:        &wrappers.UInt32Value{Value: 3},
						MaxRetries:         &wrappers.UInt32Value{Value: 4},
					},
				},
			}
			translate()

			Expect(cluster.CircuitBreakers).To(MatchProto(expectedCircuitBreakers))
		})
	})

	Context("eds", func() {

		It("should translate eds differently with different clusters", func() {
			translate()
			version1 := endpoints.Version
			// change the cluster
			upstream.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxRetries: &wrappers.UInt32Value{Value: 5},
			}
			translate()
			version2 := endpoints.Version
			Expect(version2).ToNot(Equal(version1))
		})
	})

	Context("lds", func() {

		var (
			localUpstream1 *v1.Upstream
			localUpstream2 *v1.Upstream
		)

		BeforeEach(func() {

			Expect(params.Snapshot.Upstreams).To(HaveLen(1))

			buildLocalUpstream := func(descriptors string) *v1.Upstream {
				return &v1.Upstream{
					Metadata: &core.Metadata{
						Name:      "test2",
						Namespace: "gloo-system",
					},
					UpstreamType: &v1.Upstream_Static{
						Static: &v1static.UpstreamSpec{
							ServiceSpec: &v1plugins.ServiceSpec{
								PluginType: &v1plugins.ServiceSpec_Grpc{
									Grpc: &v1grpc.ServiceSpec{
										GrpcServices: []*v1grpc.ServiceSpec_GrpcService{{
											PackageName: "foo",
											ServiceName: "bar",
										}},
										Descriptors: []byte(descriptors),
									},
								},
							},
							Hosts: []*v1static.Host{
								{
									Addr: "Test2",
									Port: 124,
								},
							},
						},
					},
				}
			}

			localUpstream1 = buildLocalUpstream("")
			localUpstream2 = buildLocalUpstream("")

		})

		It("should have same version and http filters when http filters with the same name are added in a different order", func() {
			translate()

			By("get the original version and http filters")

			// get version
			originalVersion := snapshot.GetResources(types.ListenerTypeV3).Version

			// get http filters
			hcmFilter := listener.GetFilterChains()[0].GetFilters()[0]
			typedConfig, err := glooutils.AnyToMessage(hcmFilter.GetConfigType().(*envoy_config_listener_v3.Filter_TypedConfig).TypedConfig)
			Expect(err).NotTo(HaveOccurred())
			originalHttpFilters := typedConfig.(*envoyhttp.HttpConnectionManager).HttpFilters

			By("add the upstreams and compare the new version and http filters")

			// add upstreams with same name
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, localUpstream1)
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, localUpstream2)
			Expect(params.Snapshot.Upstreams).To(HaveLen(3))

			translate()

			// get and compare version
			upstreamsVersion := snapshot.GetResources(types.ListenerTypeV3).Version
			Expect(upstreamsVersion).ToNot(Equal(originalVersion))

			// get and compare http filters
			hcmFilter = listener.GetFilterChains()[0].GetFilters()[0]
			typedConfig, err = glooutils.AnyToMessage(hcmFilter.GetConfigType().(*envoy_config_listener_v3.Filter_TypedConfig).TypedConfig)
			Expect(err).NotTo(HaveOccurred())
			upstreamsHttpFilters := typedConfig.(*envoyhttp.HttpConnectionManager).HttpFilters
			Expect(upstreamsHttpFilters).ToNot(Equal(originalHttpFilters))

			// reset modified global variables
			beforeEach()
			justBeforeEach()

			By("add the upstreams in the opposite order and compare the version and http filters")

			// add upstreams in the opposite order
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, localUpstream2)
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, localUpstream1)
			Expect(params.Snapshot.Upstreams).To(HaveLen(3))

			translate()

			// get and compare version
			flipOrderVersion := snapshot.GetResources(types.ListenerTypeV3).Version
			Expect(flipOrderVersion).To(Equal(upstreamsVersion))

			// get and compare http filters
			hcmFilter = listener.GetFilterChains()[0].GetFilters()[0]
			typedConfig, err = glooutils.AnyToMessage(hcmFilter.GetConfigType().(*envoy_config_listener_v3.Filter_TypedConfig).TypedConfig)
			Expect(err).NotTo(HaveOccurred())
			flipOrderHttpFilters := typedConfig.(*envoyhttp.HttpConnectionManager).HttpFilters
			Expect(flipOrderHttpFilters).To(Equal(upstreamsHttpFilters))
		})

	})

	Context("when handling cluster_header HTTP header name", func() {
		Context("with valid http header", func() {
			BeforeEach(func() {
				routes = []*v1.Route{{
					Name:     "testRouteClusterHeader",
					Matchers: []*matchers.Matcher{matcher},
					Action: &v1.Route_RouteAction{
						RouteAction: &v1.RouteAction{
							Destination: &v1.RouteAction_ClusterHeader{
								ClusterHeader: "test-cluster",
							},
						},
					},
				}}
			})

			It("should translate valid HTTP header name", func() {
				translate()
				route := routeConfiguration.VirtualHosts[0].Routes[0].GetRoute()
				Expect(route).ToNot(BeNil())
				cluster := route.GetClusterHeader()
				Expect(cluster).ToNot(BeNil())
				Expect(cluster).To(Equal("test-cluster"))
			})
		})

		Context("with invalid http header", func() {
			BeforeEach(func() {
				routes = []*v1.Route{{
					Name:     "testRouteClusterHeader",
					Matchers: []*matchers.Matcher{matcher},
					Action: &v1.Route_RouteAction{
						RouteAction: &v1.RouteAction{
							Destination: &v1.RouteAction_ClusterHeader{
								ClusterHeader: "invalid:-cluster",
							},
						},
					},
				}}
			})

			It("should warn about invalid http header name", func() {
				_, _, report := translator.Translate(params, proxy)
				routeReportWarning := report.GetListenerReports()[0].GetHttpListenerReport().GetVirtualHostReports()[0].GetRouteReports()[0].GetWarnings()[0]
				reason := routeReportWarning.GetReason()
				Expect(reason).To(Equal("invalid:-cluster is an invalid HTTP header name"))
			})
		})

	})

	Context("when handling upstream groups", func() {

		var (
			upstream2     *v1.Upstream
			upstreamGroup *v1.UpstreamGroup
		)

		BeforeEach(func() {
			upstream2 = &v1.Upstream{
				Metadata: &core.Metadata{
					Name:      "test2",
					Namespace: "gloo-system",
				},
				UpstreamType: &v1.Upstream_Static{
					Static: &v1static.UpstreamSpec{
						Hosts: []*v1static.Host{
							{
								Addr: "Test2",
								Port: 124,
							},
						},
					},
				},
			}
			upstreamGroup = &v1.UpstreamGroup{
				Metadata: &core.Metadata{
					Name:      "test",
					Namespace: "gloo-system",
				},
				Destinations: []*v1.WeightedDestination{
					{
						Weight: &wrappers.UInt32Value{Value: 1},
						Destination: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: upstream.Metadata.Ref(),
							},
						},
					},
					{
						Weight: &wrappers.UInt32Value{Value: 1},
						Destination: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: upstream2.Metadata.Ref(),
							},
						},
					},
				},
			}
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, upstream2)
			params.Snapshot.UpstreamGroups = v1.UpstreamGroupList{
				upstreamGroup,
			}
			ref := upstreamGroup.Metadata.Ref()
			routes = []*v1.Route{{
				Matchers: []*matchers.Matcher{matcher},
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_UpstreamGroup{
							UpstreamGroup: ref,
						},
					},
				},
			}}
		})

		It("should translate upstream groups", func() {
			translate()

			route := routeConfiguration.VirtualHosts[0].Routes[0].GetRoute()
			Expect(route).ToNot(BeNil())
			clusters := route.GetWeightedClusters()
			Expect(clusters).ToNot(BeNil())
			Expect(clusters.TotalWeight.Value).To(BeEquivalentTo(2))
			Expect(clusters.Clusters).To(HaveLen(2))
			Expect(clusters.Clusters[0].Name).To(Equal(UpstreamToClusterName(upstream.Metadata.Ref())))
			Expect(clusters.Clusters[1].Name).To(Equal(UpstreamToClusterName(upstream2.Metadata.Ref())))
		})

		It("should error on invalid ref in upstream groups", func() {
			upstreamGroup.Destinations[0].Destination.GetUpstream().Name = "notexist"

			_, errs, report := translator.Translate(params, proxy)
			err := errs.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("destination # 1: upstream not found: list did not find upstream gloo-system.notexist"))

			expectedReport := validationutils.MakeReport(proxy)
			expectedReport.ListenerReports[0].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport.VirtualHostReports[0].RouteReports[0].Warnings = []*validation.RouteReport_Warning{
				{
					Type:   validation.RouteReport_Warning_InvalidDestinationWarning,
					Reason: "invalid destination in weighted destination list: *v1.Upstream { gloo-system.notexist } not found",
				},
			}
			Expect(report.ListenerReports[0]).To(Equal(expectedReport.ListenerReports[0]))
		})

		It("should use upstreamGroup's namespace as default if namespace is omitted on upstream destination", func() {
			upstreamGroup.Destinations[0].Destination.GetUpstream().Namespace = ""

			translate()

			clusters := routeConfiguration.VirtualHosts[0].Routes[0].GetRoute().GetWeightedClusters()
			Expect(clusters).ToNot(BeNil())
			Expect(clusters.Clusters).To(HaveLen(2))
			Expect(clusters.Clusters[0].Name).To(Equal(UpstreamToClusterName(upstream.Metadata.Ref())))
			Expect(clusters.Clusters[1].Name).To(Equal(UpstreamToClusterName(upstream2.Metadata.Ref())))
		})
	})

	Context("when handling missing upstream groups", func() {
		BeforeEach(func() {
			metadata := core.Metadata{
				Name:      "missing",
				Namespace: "gloo-system",
			}
			ref := metadata.Ref()

			routes = []*v1.Route{{
				Matchers: []*matchers.Matcher{matcher},
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_UpstreamGroup{
							UpstreamGroup: ref,
						},
					},
				},
			}}
		})

		It("should set a ClusterSpecifier on the referring route", func() {
			snap, _, _ := translator.Translate(params, proxy)
			routes := snap.GetResources(types.RouteTypeV3)
			routesProto := routes.Items["http-listener-routes"]

			routeConfig := routesProto.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
			clusterSpecifier := routeConfig.VirtualHosts[0].Routes[0].GetRoute().GetClusterSpecifier()
			clusterRouteAction := clusterSpecifier.(*envoy_config_route_v3.RouteAction_Cluster)
			Expect(clusterRouteAction.Cluster).To(Equal(""))
		})
	})

	Context("when handling endpoints", func() {
		var (
			claConfiguration *envoy_config_endpoint_v3.ClusterLoadAssignment
			annotations      map[string]string
		)
		BeforeEach(func() {
			claConfiguration = nil
			annotations = map[string]string{"testkey": "testvalue"}

			upstream.UpstreamType = &v1.Upstream_Kube{
				Kube: &v1kubernetes.UpstreamSpec{},
			}
			ref := upstream.Metadata.Ref()
			params.Snapshot.Endpoints = v1.EndpointList{
				{
					Metadata: &core.Metadata{
						Name:        "test",
						Namespace:   "gloo-system",
						Annotations: annotations,
					},
					Upstreams: []*core.ResourceRef{
						ref,
					},
					Address: "1.2.3.4",
					Port:    1234,
				},
			}
		})
		It("should transfer annotations to snapshot", func() {
			translate()

			endpoints := snapshot.GetResources(types.EndpointTypeV3)

			clusterName := getEndpointClusterName(upstream)

			Expect(endpoints.Items).To(HaveKey(clusterName))
			endpointsResource := endpoints.Items[clusterName]
			claConfiguration = endpointsResource.ResourceProto().(*envoy_config_endpoint_v3.ClusterLoadAssignment)
			Expect(claConfiguration).NotTo(BeNil())
			Expect(claConfiguration.ClusterName).To(Equal(clusterName))
			Expect(claConfiguration.Endpoints).To(HaveLen(1))
			Expect(claConfiguration.Endpoints[0].LbEndpoints).To(HaveLen(len(params.Snapshot.Endpoints)))
			filterMetadata := claConfiguration.Endpoints[0].LbEndpoints[0].GetMetadata().GetFilterMetadata()

			Expect(filterMetadata).NotTo(BeNil())
			Expect(filterMetadata).To(HaveKey(SoloAnnotations))
			Expect(filterMetadata[SoloAnnotations].Fields).To(HaveKey("testkey"))
			Expect(filterMetadata[SoloAnnotations].Fields["testkey"].GetStringValue()).To(Equal("testvalue"))
		})
	})

	Context("when handling subsets", func() {
		var (
			claConfiguration *envoy_config_endpoint_v3.ClusterLoadAssignment
		)
		BeforeEach(func() {
			claConfiguration = nil

			upstream.UpstreamType = &v1.Upstream_Kube{
				Kube: &v1kubernetes.UpstreamSpec{
					SubsetSpec: &v1plugins.SubsetSpec{
						Selectors: []*v1plugins.Selector{{
							Keys: []string{
								"testkey",
							},
						}},
					},
				},
			}
			ref := upstream.Metadata.Ref()
			params.Snapshot.Endpoints = v1.EndpointList{
				{
					Metadata: &core.Metadata{
						Name:      "test",
						Namespace: "gloo-system",
						Labels:    map[string]string{"testkey": "testvalue"},
					},
					Upstreams: []*core.ResourceRef{
						ref,
					},
					Address: "1.2.3.4",
					Port:    1234,
				},
			}

			routes = []*v1.Route{{
				Matchers: []*matchers.Matcher{matcher},
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &v1.Destination{
								DestinationType: &v1.Destination_Upstream{
									Upstream: upstream.Metadata.Ref(),
								},
								Subset: &v1.Subset{
									Values: map[string]string{
										"testkey": "testvalue",
									},
								},
							},
						},
					},
				},
			}}

		})

		translateWithEndpoints := func() {
			translate()

			endpoints := snapshot.GetResources(types.EndpointTypeV3)
			clusterName := getEndpointClusterName(upstream)
			Expect(endpoints.Items).To(HaveKey(clusterName))
			endpointsResource := endpoints.Items[clusterName]
			claConfiguration = endpointsResource.ResourceProto().(*envoy_config_endpoint_v3.ClusterLoadAssignment)
			Expect(claConfiguration).NotTo(BeNil())
			Expect(claConfiguration.ClusterName).To(Equal(clusterName))
			Expect(claConfiguration.Endpoints).To(HaveLen(1))
			Expect(claConfiguration.Endpoints[0].LbEndpoints).To(HaveLen(len(params.Snapshot.Endpoints)))
		}

		Context("when happy path", func() {

			It("should transfer labels to envoy", func() {
				translateWithEndpoints()

				endpointMeta := claConfiguration.Endpoints[0].LbEndpoints[0].Metadata
				fields := endpointMeta.FilterMetadata["envoy.lb"].Fields
				Expect(fields).To(HaveKey("testkey"))
				Expect(fields["testkey"]).To(MatchProto(sv("testvalue")))
			})

			It("should add simple subset configuration to cluster", func() {
				translateWithEndpoints()

				Expect(cluster.LbSubsetConfig).ToNot(BeNil())
				Expect(cluster.LbSubsetConfig.FallbackPolicy).To(Equal(envoy_config_cluster_v3.Cluster_LbSubsetConfig_ANY_ENDPOINT))
				Expect(cluster.LbSubsetConfig.SubsetSelectors).To(HaveLen(1))
				Expect(cluster.LbSubsetConfig.SubsetSelectors[0].SingleHostPerSubset).To(BeFalse())
				Expect(cluster.LbSubsetConfig.SubsetSelectors[0].Keys).To(HaveLen(1))
				Expect(cluster.LbSubsetConfig.SubsetSelectors[0].Keys[0]).To(Equal("testkey"))
			})

			It("should add subset configuration with default subset to cluster", func() {
				upstream.UpstreamType.(*v1.Upstream_Kube).Kube.SubsetSpec =
					&v1plugins.SubsetSpec{
						Selectors: []*v1plugins.Selector{{
							SingleHostPerSubset: true,
							Keys: []string{
								"testkey",
							},
						}},
						FallbackPolicy: v1plugins.FallbackPolicy_DEFAULT_SUBSET,
						DefaultSubset: &v1plugins.Subset{Values: map[string]string{
							"testkey": "testvalue",
						}},
					}
				translateWithEndpoints()

				Expect(cluster.LbSubsetConfig).ToNot(BeNil())
				Expect(cluster.LbSubsetConfig.FallbackPolicy).To(Equal(envoy_config_cluster_v3.Cluster_LbSubsetConfig_DEFAULT_SUBSET))
				translatedDefaultSubset := cluster.LbSubsetConfig.DefaultSubset.AsMap()
				expectedVal, ok := translatedDefaultSubset["testkey"]
				Expect(ok).To(BeTrue())
				Expect(expectedVal).To(Equal("testvalue"))
				Expect(cluster.LbSubsetConfig.SubsetSelectors).To(HaveLen(1))
				Expect(cluster.LbSubsetConfig.SubsetSelectors[0].SingleHostPerSubset).To(BeTrue())
			})

			It("should add subset to route", func() {
				translateWithEndpoints()

				metadataMatch := routeConfiguration.VirtualHosts[0].Routes[0].GetRoute().GetMetadataMatch()
				fields := metadataMatch.FilterMetadata["envoy.lb"].Fields
				Expect(fields).To(HaveKey("testkey"))
				Expect(fields["testkey"]).To(MatchProto(sv("testvalue")))
			})
		})

		It("should create empty value if missing labels on the endpoint are provided in the upstream", func() {
			params.Snapshot.Endpoints[0].Metadata.Labels = nil
			translateWithEndpoints()
			endpointMeta := claConfiguration.Endpoints[0].LbEndpoints[0].Metadata
			Expect(endpointMeta).ToNot(BeNil())
			Expect(endpointMeta.FilterMetadata).To(HaveKey("envoy.lb"))
			fields := endpointMeta.FilterMetadata["envoy.lb"].Fields
			Expect(fields).To(HaveKey("testkey"))
			Expect(fields["testkey"]).To(MatchProto(sv("")))
		})

		Context("subset in route doesnt match subset in upstream", func() {

			BeforeEach(func() {
				routes = []*v1.Route{{
					Matchers: []*matchers.Matcher{matcher},
					Action: &v1.Route_RouteAction{
						RouteAction: &v1.RouteAction{
							Destination: &v1.RouteAction_Single{
								Single: &v1.Destination{
									DestinationType: &v1.Destination_Upstream{
										Upstream: (upstream.Metadata.Ref()),
									},
									Subset: &v1.Subset{
										Values: map[string]string{
											"nottestkey": "value",
										},
									},
								},
							},
						},
					},
				}}
			})

			It("should error the route", func() {
				_, errs, report := translator.Translate(params, proxy)
				Expect(errs.Validate()).To(HaveOccurred())
				Expect(errs.Validate().Error()).To(ContainSubstring("route has a subset config, but none of the subsets in the upstream match it"))
				processingErrorName := fmt.Sprintf("%s-route-0-matcher-0", virtualHostName)
				expectedReport := validationutils.MakeReport(proxy)
				expectedReport.ListenerReports[0].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport.VirtualHostReports[0].RouteReports[0].Errors = []*validation.RouteReport_Error{
					{
						Type:   validation.RouteReport_Error_ProcessingError,
						Reason: fmt.Sprintf("route has a subset config, but none of the subsets in the upstream match it. Route Name: %s", processingErrorName),
					},
				}
				Expect(report.ListenerReports[0]).To(Equal(expectedReport.ListenerReports[0]))
			})
		})

		Context("when a route table is missing", func() {

			BeforeEach(func() {
				routes = []*v1.Route{{
					Matchers: []*matchers.Matcher{matcher},
					Action: &v1.Route_RouteAction{
						RouteAction: &v1.RouteAction{
							Destination: &v1.RouteAction_Single{
								Single: &v1.Destination{
									DestinationType: &v1.Destination_Upstream{
										Upstream: &core.ResourceRef{Name: "do", Namespace: "notexist"},
									},
								},
							},
						},
					},
				}}
			})

			It("should warn a route when a destination is missing", func() {
				_, errs, report := translator.Translate(params, proxy)
				Expect(errs.Validate()).NotTo(HaveOccurred())
				Expect(errs.ValidateStrict()).To(HaveOccurred())
				Expect(errs.ValidateStrict().Error()).To(ContainSubstring("*v1.Upstream { notexist.do } not found"))
				expectedReport := validationutils.MakeReport(proxy)
				expectedReport.ListenerReports[0].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport.VirtualHostReports[0].RouteReports[0].Warnings = []*validation.RouteReport_Warning{
					{
						Type:   validation.RouteReport_Warning_InvalidDestinationWarning,
						Reason: "*v1.Upstream { notexist.do } not found",
					},
				}

				Expect(report.ListenerReports[0]).To(Equal(expectedReport.ListenerReports[0]))
			})
		})
	})

	Context("when translating a route that points directly to a service", func() {

		var fakeUsList v1.UpstreamList

		BeforeEach(func() {

			// The kube service that we want to route to
			svc := skkube.NewService("ns-1", "svc-1")
			svc.Spec = k8scorev1.ServiceSpec{
				Ports: []k8scorev1.ServicePort{
					{
						Name:       "port-1",
						Port:       8080,
						TargetPort: intstr.FromInt(80),
					},
					{
						Name:       "port-2",
						Port:       8081,
						TargetPort: intstr.FromInt(8081),
					},
				},
			}
			// These are the "fake" upstreams that represent the above service in the snapshot
			fakeUsList = kubernetes.KubeServicesToUpstreams(context.TODO(), skkube.ServiceList{svc})
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, fakeUsList...)

			// We need to manually add some fake endpoints for the above kubernetes services to the snapshot
			// Normally these would have been discovered by EDS
			params.Snapshot.Endpoints = v1.EndpointList{
				{
					Metadata: &core.Metadata{
						Namespace: "gloo-system",
						Name:      fmt.Sprintf("ep-%v-%v", "192.168.0.1", svc.Spec.Ports[0].Port),
					},
					Port:      uint32(svc.Spec.Ports[0].Port),
					Address:   "192.168.0.1",
					Upstreams: []*core.ResourceRef{(fakeUsList[0].Metadata.Ref())},
				},
				{
					Metadata: &core.Metadata{
						Namespace: "gloo-system",
						Name:      fmt.Sprintf("ep-%v-%v", "192.168.0.2", svc.Spec.Ports[1].Port),
					},
					Port:      uint32(svc.Spec.Ports[1].Port),
					Address:   "192.168.0.2",
					Upstreams: []*core.ResourceRef{(fakeUsList[1].Metadata.Ref())},
				},
			}

			// Configure Proxy to route to the service
			serviceDestination := v1.Destination{
				DestinationType: &v1.Destination_Kube{
					Kube: &v1.KubernetesServiceDestination{
						Ref: &core.ResourceRef{
							Namespace: svc.Namespace,
							Name:      svc.Name,
						},
						Port: uint32(svc.Spec.Ports[0].Port),
					},
				},
			}
			routes = []*v1.Route{{
				Matchers: []*matchers.Matcher{matcher},
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &serviceDestination,
						},
					},
				},
			}}
		})

		It("generates the expected envoy route configuration", func() {
			translate()

			// Clusters have been created for the two "fake" upstreams
			clusters := snapshot.GetResources(types.ClusterTypeV3)
			clusterResource := clusters.Items[UpstreamToClusterName(fakeUsList[0].Metadata.Ref())]
			cluster = clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)
			Expect(cluster).NotTo(BeNil())
			clusterResource = clusters.Items[UpstreamToClusterName(fakeUsList[1].Metadata.Ref())]
			cluster = clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)
			Expect(cluster).NotTo(BeNil())

			// A route to the kube service has been configured
			routes := snapshot.GetResources(types.RouteTypeV3)
			Expect(routes.Items).To(HaveKey("http-listener-routes"))
			routeResource := routes.Items["http-listener-routes"]
			routeConfiguration = routeResource.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
			Expect(routeConfiguration).NotTo(BeNil())
			Expect(routeConfiguration.VirtualHosts).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains[0]).To(Equal("*"))
			Expect(routeConfiguration.VirtualHosts[0].Routes).To(HaveLen(1))
			routeAction, ok := routeConfiguration.VirtualHosts[0].Routes[0].Action.(*envoy_config_route_v3.Route_Route)
			Expect(ok).To(BeTrue())
			clusterAction, ok := routeAction.Route.ClusterSpecifier.(*envoy_config_route_v3.RouteAction_Cluster)
			Expect(ok).To(BeTrue())
			Expect(clusterAction.Cluster).To(Equal(UpstreamToClusterName(fakeUsList[0].Metadata.Ref())))
		})
	})

	Context("when translating a route that points to a Consul service", func() {

		var (
			fakeUsList v1.UpstreamList
			dc         = func(dataCenterName string) string {
				return constants.ConsulDataCenterKeyPrefix + dataCenterName
			}
			tag = func(tagName string) string {
				return constants.ConsulTagKeyPrefix + tagName
			}

			trueValue = &structpb.Value{
				Kind: &structpb.Value_StringValue{
					StringValue: constants.ConsulEndpointMetadataMatchTrue,
				},
			}
			falseValue = &structpb.Value{
				Kind: &structpb.Value_StringValue{
					StringValue: constants.ConsulEndpointMetadataMatchFalse,
				},
			}
		)

		const (
			svcName = "my-consul-svc"

			// Data centers
			east = "east"
			west = "west"

			// Tags
			dev  = "dev"
			prod = "prod"

			yes = constants.ConsulEndpointMetadataMatchTrue
			no  = constants.ConsulEndpointMetadataMatchFalse
		)

		BeforeEach(func() {

			// Metadata for the Consul service that we want to route to
			svc := &consul.ServiceMeta{
				Name:        svcName,
				DataCenters: []string{east, west},
				Tags:        []string{dev, prod},
			}
			// These are the "fake" upstreams that represent the above service in the snapshot
			initialUsList := consul.CreateUpstreamsFromService(svc, nil)
			if len(initialUsList) == 1 {
				fakeUsList = v1.UpstreamList{consul.CreateUpstreamsFromService(svc, nil)[0]}
			} else {
				fakeUsList = v1.UpstreamList{}
			}
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, fakeUsList...)

			// We need to manually add some fake endpoints for the above Consul service
			// Normally these would have been discovered by EDS
			params.Snapshot.Endpoints = v1.EndpointList{
				// 2 prod endpoints, 1 in each data center, 1 dev endpoint in west data center
				{
					Metadata: &core.Metadata{
						Namespace: defaults.GlooSystem,
						Name:      svc.Name + "_1",
						Labels: map[string]string{
							dc(east):  yes,
							dc(west):  no,
							tag(dev):  no,
							tag(prod): yes,
						},
					},
					Port:      1001,
					Address:   "1.0.0.1",
					Upstreams: []*core.ResourceRef{(fakeUsList[0].Metadata.Ref())},
				},
				{
					Metadata: &core.Metadata{
						Namespace: defaults.GlooSystem,
						Name:      svc.Name + "_2",
						Labels: map[string]string{
							dc(east):  no,
							dc(west):  yes,
							tag(dev):  no,
							tag(prod): yes,
						},
					},
					Port:      2001,
					Address:   "2.0.0.1",
					Upstreams: []*core.ResourceRef{(fakeUsList[0].Metadata.Ref())},
				},
				{
					Metadata: &core.Metadata{
						Namespace: defaults.GlooSystem,
						Name:      svc.Name + "_3",
						Labels: map[string]string{
							dc(east):  no,
							dc(west):  yes,
							tag(dev):  yes,
							tag(prod): no,
						},
					},
					Port:      2002,
					Address:   "2.0.0.2",
					Upstreams: []*core.ResourceRef{(fakeUsList[0].Metadata.Ref())},
				},
			}

			// Configure Proxy to route to the service
			serviceDestination := v1.Destination{
				DestinationType: &v1.Destination_Consul{
					Consul: &v1.ConsulServiceDestination{
						ServiceName: svcName,
						Tags:        []string{prod},
						DataCenters: []string{east},
					},
				},
			}
			routes = []*v1.Route{{
				Matchers: []*matchers.Matcher{matcher},
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &serviceDestination,
						},
					},
				},
			}}
		})

		It("generates the expected envoy route configuration", func() {
			translate()

			// A cluster has been created for the "fake" upstream and has the expected subset config
			clusters := snapshot.GetResources(types.ClusterTypeV3)
			clusterResource := clusters.Items[UpstreamToClusterName(fakeUsList[0].Metadata.Ref())]
			cluster = clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)
			Expect(cluster).NotTo(BeNil())
			Expect(cluster.LbSubsetConfig).NotTo(BeNil())
			Expect(cluster.LbSubsetConfig.SubsetSelectors).To(HaveLen(3))
			// Order is important here
			Expect(cluster.LbSubsetConfig.SubsetSelectors).To(ConsistOfProtos(
				&envoy_config_cluster_v3.Cluster_LbSubsetConfig_LbSubsetSelector{
					Keys: []string{dc(east), dc(west)},
				},
				&envoy_config_cluster_v3.Cluster_LbSubsetConfig_LbSubsetSelector{
					Keys: []string{tag(dev), tag(prod)},
				},
				&envoy_config_cluster_v3.Cluster_LbSubsetConfig_LbSubsetSelector{
					Keys: []string{dc(east), dc(west), tag(dev), tag(prod)},
				},
			))

			// A route to the kube service has been configured
			routes := snapshot.GetResources(types.RouteTypeV3)
			Expect(routes.Items).To(HaveKey("http-listener-routes"))
			routeResource := routes.Items["http-listener-routes"]
			routeConfiguration = routeResource.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
			Expect(routeConfiguration).NotTo(BeNil())
			Expect(routeConfiguration.VirtualHosts).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains[0]).To(Equal("*"))
			Expect(routeConfiguration.VirtualHosts[0].Routes).To(HaveLen(1))
			routeAction, ok := routeConfiguration.VirtualHosts[0].Routes[0].Action.(*envoy_config_route_v3.Route_Route)
			Expect(ok).To(BeTrue())

			clusterAction, ok := routeAction.Route.ClusterSpecifier.(*envoy_config_route_v3.RouteAction_Cluster)
			Expect(ok).To(BeTrue())
			Expect(clusterAction.Cluster).To(Equal(UpstreamToClusterName(fakeUsList[0].Metadata.Ref())))

			Expect(routeAction.Route).NotTo(BeNil())
			Expect(routeAction.Route.MetadataMatch).NotTo(BeNil())
			metadata, ok := routeAction.Route.MetadataMatch.FilterMetadata[EnvoyLb]
			Expect(ok).To(BeTrue())

			Expect(metadata.Fields).To(HaveLen(4))
			Expect(metadata.Fields[dc(east)]).To(MatchProto(trueValue))
			Expect(metadata.Fields[dc(west)]).To(MatchProto(falseValue))
			Expect(metadata.Fields[tag(dev)]).To(MatchProto(falseValue))
			Expect(metadata.Fields[tag(prod)]).To(MatchProto(trueValue))
		})
	})

	Context("when translating a route that points to an AWS lambda", func() {

		createLambdaUpstream := func(namespace, name, region string, lambdaFuncs []*aws.LambdaFunctionSpec) *v1.Upstream {
			return &v1.Upstream{
				Metadata: &core.Metadata{
					Name:      name,
					Namespace: namespace,
				},
				DiscoveryMetadata: nil,
				UpstreamType: &v1.Upstream_Aws{
					Aws: &aws.UpstreamSpec{
						SecretRef: &core.ResourceRef{
							Name:      "my-aws-secret",
							Namespace: "my-namespace",
						},
						Region:          region,
						LambdaFunctions: lambdaFuncs,
					},
				},
			}
		}

		BeforeEach(func() {
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams,
				createLambdaUpstream("my-namespace", "lambda-upstream-1", "us-east-1",
					[]*aws.LambdaFunctionSpec{
						{
							LogicalName: "usEast1Lambda1",
						},
						{
							LogicalName: "usEast1Lambda2",
						},
					}),
				createLambdaUpstream("my-namespace", "lambda-upstream-2", "us-east-2",
					[]*aws.LambdaFunctionSpec{
						{
							LogicalName: "usEast2Lambda1",
						},
						{
							LogicalName: "usEast2Lambda2",
						},
					}))

			secret := &v1.Secret{
				Metadata: &core.Metadata{
					Name:      "my-aws-secret",
					Namespace: "my-namespace",
				},
				Kind: &v1.Secret_Aws{
					Aws: &v1.AwsSecret{
						AccessKey: "a",
						SecretKey: "a",
					},
				},
			}

			params.Snapshot.Secrets = v1.SecretList{secret}
		})

		It("has no errors when pointing to a valid lambda", func() {
			validLambdaRoute := &v1.Route{Action: &v1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: &core.ResourceRef{
									Name:      "lambda-upstream-1",
									Namespace: "my-namespace",
								},
							},
							DestinationSpec: &v1.DestinationSpec{
								DestinationType: &v1.DestinationSpec_Aws{
									Aws: &aws.DestinationSpec{
										LogicalName: "usEast1Lambda1",
									},
								},
							},
						},
					},
				}}}

			routes := proxy.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].GetRoutes()
			proxy.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].Routes = append(routes, validLambdaRoute)

			translate()
		})

		It("reports error when pointing to a lambda function that doesn't exist", func() {
			invalidLambdaRoute := &v1.Route{Action: &v1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: &core.ResourceRef{
									Name:      "lambda-upstream-1",
									Namespace: "my-namespace",
								},
							},
							DestinationSpec: &v1.DestinationSpec{
								DestinationType: &v1.DestinationSpec_Aws{
									Aws: &aws.DestinationSpec{
										LogicalName: "nonexistentLambdaFunc",
									},
								},
							},
						},
					},
				}}}

			routes := proxy.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].GetRoutes()
			proxy.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].Routes = append(routes, invalidLambdaRoute)
			_, resourceReport, _ := translator.Translate(params, proxy)
			Expect(resourceReport.Validate()).To(HaveOccurred())
			Expect(resourceReport.Validate().Error()).To(ContainSubstring("a route references nonexistentLambdaFunc AWS lambda which does not exist on the route's upstream"))
		})

		It("reports error when route has Multi Cluster destination and points to at least one lambda function that doesn't exist", func() {
			invalidLambdaRoute := &v1.Route{Action: &v1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Multi{
						Multi: &v1.MultiDestination{
							Destinations: []*v1.WeightedDestination{
								{
									Destination: &v1.Destination{
										DestinationType: &v1.Destination_Upstream{
											Upstream: &core.ResourceRef{
												Name:      "aws-lambda-upstream",
												Namespace: "my-namespace",
											},
										},
										DestinationSpec: &v1.DestinationSpec{
											DestinationType: &v1.DestinationSpec_Aws{
												Aws: &aws.DestinationSpec{
													LogicalName: "nonexistentLambdaFunc",
												},
											},
										},
									},
								},
							},
						},
					},
				}}}

			routes := proxy.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].GetRoutes()
			proxy.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].Routes = append(routes, invalidLambdaRoute)
			_, resourceReport, _ := translator.Translate(params, proxy)
			Expect(resourceReport.Validate()).To(HaveOccurred())
			Expect(resourceReport.Validate().Error()).To(ContainSubstring("a route references nonexistentLambdaFunc AWS lambda which does not exist on the route's upstream"))
		})

	})

	Context("when translating a multi-route action with differing passed weights", func() {

		var (
			multiActionRouteWithNoWeightPassedDest *v1.Route
			multiActionRouteOneDest                *v1.Route
			multiActionRouteZeroAndFiveAsWeights   *v1.Route
			expectedErrorString                    string
			weightedDestFiveWeight                 *v1.WeightedDestination
			weightedDestZeroWeight                 *v1.WeightedDestination
		)

		BeforeEach(func() {
			testUpstream1 := createStaticUpstream("test1", "gloo-system")
			testUpstream2 := createStaticUpstream("test2", "gloo-system")

			weightedDestNoWeightPassed := createWeightedDestination(false, 0, testUpstream1)
			weightedDestZeroWeight = createWeightedDestination(true, 0, testUpstream1)
			weightedDestFiveWeight = createWeightedDestination(true, 5, testUpstream2)

			multiActionRouteOneDest = createMultiActionRoute("OneDest", matcher, []*v1.WeightedDestination{weightedDestFiveWeight})
			multiActionRouteZeroAndFiveAsWeights = createMultiActionRoute("TwoDest", matcher, []*v1.WeightedDestination{weightedDestFiveWeight, weightedDestZeroWeight})
			multiActionRouteWithNoWeightPassedDest = createMultiActionRoute("NoWeightPassedDest", matcher, []*v1.WeightedDestination{weightedDestNoWeightPassed, weightedDestNoWeightPassed})

			expectedErrorString = fmt.Sprintf("Incorrect configuration for Weighted Destination for route - Weighted Destinations require a total weight that is greater than or equal to 1")
		})

		//Positive Tests
		It("Should translate single routes when multiRoute is passed and only one destination is specified", func() {
			proxy.Listeners[0].GetHttpListener().GetVirtualHosts()[0].Routes = []*v1.Route{multiActionRouteOneDest}
			snap, resourceReport, _ := translator.Translate(params, proxy)
			Expect(resourceReport.ValidateStrict()).To(HaveOccurred())

			// A weighted route to the service has been configured
			routes := snap.GetResources(types.RouteTypeV3)
			Expect(routes.Items).To(HaveKey("http-listener-routes"))
			routeResource := routes.Items["http-listener-routes"]
			routeConfiguration = routeResource.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
			Expect(routeConfiguration).NotTo(BeNil())
			Expect(routeConfiguration.VirtualHosts).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains[0]).To(Equal("*"))
			Expect(routeConfiguration.VirtualHosts[0].Routes).To(HaveLen(1))
			routeAction, ok := routeConfiguration.VirtualHosts[0].Routes[0].Action.(*envoy_config_route_v3.Route_Route)
			Expect(ok).To(BeTrue())
			clusterAction, ok := routeAction.Route.ClusterSpecifier.(*envoy_config_route_v3.RouteAction_WeightedClusters)
			Expect(ok).To(BeTrue())

			//DataFromWeightedCluster
			totalWeight := weightedDestFiveWeight.Weight
			expectedClusterName := weightedDestFiveWeight.Destination.GetUpstream().Name + "_" + weightedDestFiveWeight.Destination.GetUpstream().Namespace

			//There is only one route with a weight of 5 so total weight for the cluster should be 5
			Expect(clusterAction.WeightedClusters.TotalWeight.GetValue()).To(Equal(totalWeight.GetValue()))
			clusters := clusterAction.WeightedClusters.Clusters
			Expect(clusters).To(HaveLen(1))
			Expect(clusters[0].Weight.GetValue()).To(Equal(totalWeight.GetValue()))
			Expect(clusters[0].Name).To(Equal(expectedClusterName))
		})

		It("Should translate 0 weight destinations if there are other destinations with weights over 0", func() {
			proxy.Listeners[0].GetHttpListener().GetVirtualHosts()[0].Routes = []*v1.Route{multiActionRouteZeroAndFiveAsWeights}
			snap, resourceReport, _ := translator.Translate(params, proxy)
			Expect(resourceReport.ValidateStrict()).To(HaveOccurred())

			// A weighted route to the service has been configured
			routes := snap.GetResources(types.RouteTypeV3)
			Expect(routes.Items).To(HaveKey("http-listener-routes"))
			routeResource := routes.Items["http-listener-routes"]
			routeConfiguration = routeResource.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
			Expect(routeConfiguration).NotTo(BeNil())
			Expect(routeConfiguration.VirtualHosts).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains[0]).To(Equal("*"))
			Expect(routeConfiguration.VirtualHosts[0].Routes).To(HaveLen(1))
			routeAction, ok := routeConfiguration.VirtualHosts[0].Routes[0].Action.(*envoy_config_route_v3.Route_Route)
			Expect(ok).To(BeTrue())
			clusterAction, ok := routeAction.Route.ClusterSpecifier.(*envoy_config_route_v3.RouteAction_WeightedClusters)
			Expect(ok).To(BeTrue())

			totalWeight := weightedDestFiveWeight.Weight.GetValue() + weightedDestZeroWeight.Weight.GetValue()
			clusterNameFiveWeight := weightedDestFiveWeight.Destination.GetUpstream().Name + "_" + weightedDestFiveWeight.Destination.GetUpstream().Namespace
			clusterNameZeroWeight := weightedDestZeroWeight.Destination.GetUpstream().Name + "_" + weightedDestZeroWeight.Destination.GetUpstream().Namespace

			//There is only one route with a weight of 5 so total weight for the cluster should be 5
			Expect(clusterAction.WeightedClusters.TotalWeight.GetValue()).To(Equal(totalWeight))
			clusters := clusterAction.WeightedClusters.Clusters
			Expect(clusters).To(HaveLen(2))

			for _, c := range clusters {
				switch c.Name {
				case clusterNameFiveWeight:
					Expect(c.Weight.GetValue()).To(Equal(uint32(5)))
				case clusterNameZeroWeight:
					Expect(c.Weight.GetValue()).To(Equal(uint32(0)))
				}
			}
		})

		//Negative Tests
		It("Should report an error when total weight is 0 - nil and 0 weights passed", func() {
			proxy.Listeners[0].GetHttpListener().GetVirtualHosts()[0].Routes = []*v1.Route{multiActionRouteWithNoWeightPassedDest}
			_, errs, _ := translator.Translate(params, proxy)
			Expect(errs.Validate()).To(HaveOccurred())
			Expect(errs.Validate().Error()).To(ContainSubstring(expectedErrorString))
		})

	})

	Context("Route plugin", func() {
		var (
			routePlugin *routePluginMock
		)
		BeforeEach(func() {
			routePlugin = &routePluginMock{}
			registeredPlugins = append(registeredPlugins, routePlugin)
		})

		It("should have the virtual host when processing route", func() {
			hasVHost := false
			routePlugin.ProcessRouteFunc = func(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
				if params.VirtualHost != nil {
					if params.VirtualHost.GetName() == "virt1" {
						hasVHost = true
					}
				}
				return nil
			}

			translate()
			Expect(hasVHost).To(BeTrue())
		})

	})

	Context("EndpointPlugin", func() {
		var (
			endpointPlugin *endpointPluginMock
			upstreamList   v1.UpstreamList
		)
		BeforeEach(func() {
			endpointPlugin = &endpointPluginMock{}
			registeredPlugins = append(registeredPlugins, endpointPlugin)
			upstreamList = params.Snapshot.Upstreams.Clone()
		})

		AfterEach(func() {
			params.Snapshot.Upstreams = upstreamList
		})

		It("should call the endpoint plugin", func() {
			additionalEndpoint := &envoy_config_endpoint_v3.LocalityLbEndpoints{
				Locality: &envoy_config_core_v3.Locality{
					Region: "region",
					Zone:   "a",
				},
				Priority: 10,
			}

			endpointPlugin.ProcessEndpointFunc = func(params plugins.Params, in *v1.Upstream, out *envoy_config_endpoint_v3.ClusterLoadAssignment) error {
				Expect(out.GetEndpoints()).To(HaveLen(1))
				Expect(out.GetClusterName()).To(Equal(UpstreamToClusterName(upstream.Metadata.Ref())))
				Expect(out.GetEndpoints()[0].GetLbEndpoints()).To(HaveLen(1))

				out.Endpoints = append(out.Endpoints, additionalEndpoint)
				return nil
			}

			translate()
			endpointResource := endpoints.Items["test_gloo-system"]
			endpoint := endpointResource.ResourceProto().(*envoy_config_endpoint_v3.ClusterLoadAssignment)
			Expect(endpoint).NotTo(BeNil())
			Expect(endpoint.Endpoints).To(HaveLen(2))
			Expect(endpoint.Endpoints[1]).To(MatchProto(additionalEndpoint))
		})

		It("should call the endpoint plugin with an empty endpoint", func() {
			// Create an empty consul upstream just to get EDS
			emptyUpstream := &v1.Upstream{
				Metadata: &core.Metadata{
					Namespace: "empty_namespace",
					Name:      "empty_name",
				},
				UpstreamType: &v1.Upstream_Consul{
					Consul: &consul2.UpstreamSpec{},
				},
			}
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, emptyUpstream)

			foundEmptyUpstream := false

			endpointPlugin.ProcessEndpointFunc = func(params plugins.Params, in *v1.Upstream, out *envoy_config_endpoint_v3.ClusterLoadAssignment) error {
				if in.Metadata.Name == emptyUpstream.Metadata.Name &&
					in.Metadata.Namespace == emptyUpstream.Metadata.Namespace {
					foundEmptyUpstream = true
				}
				return nil
			}

			translate()
			Expect(foundEmptyUpstream).To(BeTrue())
		})

	})

	Context("Route option on direct response actions", func() {

		BeforeEach(func() {
			routes = []*v1.Route{{
				Matchers: []*matchers.Matcher{matcher},
				Action: &v1.Route_DirectResponseAction{
					DirectResponseAction: &v1.DirectResponseAction{
						Status: 405,
						Body:   "Unsupported HTTP method",
					},
				},
				Options: &v1.RouteOptions{
					HeaderManipulation: &headers.HeaderManipulation{
						ResponseHeadersToAdd: []*headers.HeaderValueOption{{
							Header: &headers.HeaderValue{
								Key:   "client-id",
								Value: "%REQ(client-id)%",
							},
							Append: &wrappers.BoolValue{
								Value: false,
							},
						}},
					},
				},
			}}
		})

		It("should have the virtual host when processing route", func() {
			translate()

			// A route to the kube service has been configured
			routes := snapshot.GetResources(types.RouteTypeV3)
			Expect(routes.Items).To(HaveKey("http-listener-routes"))
			routeResource := routes.Items["http-listener-routes"]
			routeConfiguration = routeResource.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
			Expect(routeConfiguration).NotTo(BeNil())
			Expect(routeConfiguration.VirtualHosts).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains[0]).To(Equal("*"))

			Expect(routeConfiguration.VirtualHosts[0].Routes).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Routes[0].ResponseHeadersToAdd).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Routes[0].ResponseHeadersToAdd[0]).To(MatchProto(
				&envoy_config_core_v3.HeaderValueOption{
					Header: &envoy_config_core_v3.HeaderValue{
						Key:   "client-id",
						Value: "%REQ(client-id)%",
					},
					AppendAction: envoy_config_core_v3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
				},
			))
		})

	})

	Context("TCP", func() {
		It("can properly create a tcp listener", func() {
			translate()
			listeners := snapshot.GetResources(types.ListenerTypeV3).Items
			Expect(listeners).NotTo(HaveLen(0))
			val, found := listeners["tcp-listener"]
			Expect(found).To(BeTrue())
			listener, ok := val.ResourceProto().(*envoy_config_listener_v3.Listener)
			Expect(ok).To(BeTrue())
			Expect(listener.GetName()).To(Equal("tcp-listener"))
			Expect(listener.GetFilterChains()).To(HaveLen(1))
			fc := listener.GetFilterChains()[0]
			Expect(fc.Filters).To(HaveLen(1))
			tcpFilter := fc.Filters[0]
			cfg := tcpFilter.GetTypedConfig()
			Expect(cfg).NotTo(BeNil())
			var typedCfg envoytcp.TcpProxy
			Expect(ParseTypedConfig(tcpFilter, &typedCfg)).NotTo(HaveOccurred())
			clusterSpec := typedCfg.GetCluster()
			Expect(clusterSpec).To(Equal("test_gloo-system"))
			Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
		})
	})

	Context("Hybrid", func() {

		// The number of HttpFilters that we expect to be generated on the HttpConnectionManager by default
		var defaultHttpFilters = 7

		It("can properly create a hybrid listener", func() {
			translate()
			listeners := snapshot.GetResources(types.ListenerTypeV3).Items
			Expect(listeners).NotTo(HaveLen(0))
			val, found := listeners["hybrid-listener"]
			Expect(found).To(BeTrue())
			listener, ok := val.ResourceProto().(*envoy_config_listener_v3.Listener)
			Expect(ok).To(BeTrue())
			Expect(listener.GetName()).To(Equal("hybrid-listener"))
			Expect(listener.GetFilterChains()).To(HaveLen(2))

			// tcp
			tcpFc := listener.GetFilterChains()[0]
			Expect(tcpFc.Filters).To(HaveLen(1))
			tcpFilter := tcpFc.Filters[0]
			tcpCfg := tcpFilter.GetTypedConfig()
			Expect(tcpCfg).NotTo(BeNil())
			var tcpTypedCfg envoytcp.TcpProxy
			Expect(ParseTypedConfig(tcpFilter, &tcpTypedCfg)).NotTo(HaveOccurred())
			clusterSpec := tcpTypedCfg.GetCluster()
			Expect(clusterSpec).To(Equal("test_gloo-system"))
			Expect(listener.GetListenerFilters()).To(HaveLen(1))
			Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))

			// http
			httpFc := listener.GetFilterChains()[1]
			Expect(httpFc.Filters).To(HaveLen(1))
			hcmFilter := httpFc.Filters[0]
			hcmCfg := hcmFilter.GetConfigType()
			Expect(hcmCfg).NotTo(BeNil())
			var hcmTypedCfg envoyhttp.HttpConnectionManager
			Expect(ParseTypedConfig(hcmFilter, &hcmTypedCfg)).NotTo(HaveOccurred())
			Expect(hcmTypedCfg.GetRds()).NotTo(BeNil())
			Expect(hcmTypedCfg.GetRds().RouteConfigName).To(Equal(glooutils.MatchedRouteConfigName(proxy.GetListeners()[2], proxy.GetListeners()[2].GetHybridListener().GetMatchedListeners()[1].GetMatcher())))
			Expect(hcmTypedCfg.GetHttpFilters()).To(HaveLen(defaultHttpFilters))
		})

		It("can properly create a hybrid listeners without unused filters", func() {
			settings.Gloo = &v1.GlooOptions{RemoveUnusedFilters: &wrappers.BoolValue{Value: true}}
			translate()
			listeners := snapshot.GetResources(types.ListenerTypeV3).Items
			Expect(listeners).NotTo(HaveLen(0))
			val, found := listeners["hybrid-listener"]
			Expect(found).To(BeTrue())
			listener, ok := val.ResourceProto().(*envoy_config_listener_v3.Listener)
			Expect(ok).To(BeTrue())
			Expect(listener.GetName()).To(Equal("hybrid-listener"))
			Expect(listener.GetFilterChains()).To(HaveLen(2))

			// tcp
			tcpFc := listener.GetFilterChains()[0]
			Expect(tcpFc.Filters).To(HaveLen(1))
			tcpFilter := tcpFc.Filters[0]
			tcpCfg := tcpFilter.GetTypedConfig()
			Expect(tcpCfg).NotTo(BeNil())
			var tcpTypedCfg envoytcp.TcpProxy
			Expect(ParseTypedConfig(tcpFilter, &tcpTypedCfg)).NotTo(HaveOccurred())
			clusterSpec := tcpTypedCfg.GetCluster()
			Expect(clusterSpec).To(Equal("test_gloo-system"))
			Expect(listener.GetListenerFilters()).To(HaveLen(1))
			Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))

			// http
			httpFc := listener.GetFilterChains()[1]
			Expect(httpFc.Filters).To(HaveLen(1))
			hcmFilter := httpFc.Filters[0]
			hcmCfg := hcmFilter.GetConfigType()
			Expect(hcmCfg).NotTo(BeNil())
			var hcmTypedCfg envoyhttp.HttpConnectionManager
			Expect(ParseTypedConfig(hcmFilter, &hcmTypedCfg)).NotTo(HaveOccurred())
			Expect(hcmTypedCfg.GetRds()).NotTo(BeNil())
			Expect(hcmTypedCfg.GetRds().RouteConfigName).To(Equal(glooutils.MatchedRouteConfigName(proxy.GetListeners()[2], proxy.GetListeners()[2].GetHybridListener().GetMatchedListeners()[1].GetMatcher())))
			Expect(hcmTypedCfg.GetHttpFilters()).To(HaveLen(1)) // only the router filter should be configured
		})

		It("skips listeners with invalid downstream ssl config", func() {
			invalidSslSecretRef := &ssl.SslConfig_SecretRef{
				SecretRef: &core.ResourceRef{
					Name:      "invalid",
					Namespace: "invalid",
				},
			}

			proxyClone := proto.Clone(proxy).(*v1.Proxy)
			proxyClone.GetListeners()[2].GetHybridListener().GetMatchedListeners()[1].SslConfigurations = []*ssl.SslConfig{{
				SslSecrets: invalidSslSecretRef,
			}}

			_, errs, _ := translator.Translate(params, proxyClone)
			Expect(errs.Validate()).To(HaveOccurred())
			Expect(errs.Validate().Error()).To(ContainSubstring("Listener Error: SSLConfigError. Reason: SSL secret not found: list did not find secret"))
		})

	})

	Context("Ssl - cluster", func() {

		var (
			tlsConf *v1.TlsSecret
		)
		BeforeEach(func() {

			tlsConf = &v1.TlsSecret{}
			secret := &v1.Secret{
				Metadata: &core.Metadata{
					Name:      "name",
					Namespace: "namespace",
				},
				Kind: &v1.Secret_Tls{
					Tls: tlsConf,
				},
			}
			ref := secret.Metadata.Ref()
			upstream.SslConfig = &ssl.UpstreamSslConfig{
				SslSecrets: &ssl.UpstreamSslConfig_SecretRef{
					SecretRef: ref,
				},
			}
			params = plugins.Params{
				Ctx: context.Background(),
				Snapshot: &v1snap.ApiSnapshot{
					Secrets:   v1.SecretList{secret},
					Upstreams: v1.UpstreamList{upstream},
				},
			}

		})

		tlsContext := func() *envoyauth.UpstreamTlsContext {
			clusters := snapshot.GetResources(types.ClusterTypeV3)
			clusterResource := clusters.Items[UpstreamToClusterName(upstream.Metadata.Ref())]
			cluster := clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)

			return glooutils.MustAnyToMessage(cluster.TransportSocket.GetTypedConfig()).(*envoyauth.UpstreamTlsContext)
		}

		It("should process an upstream with tls config", func() {

			pk := gloohelpers.PrivateKey()
			cc := gloohelpers.Certificate()

			tlsConf.PrivateKey = pk
			tlsConf.CertChain = cc

			translate()
			Expect(tlsContext()).ToNot(BeNil())
			Expect(tlsContext().CommonTlsContext.TlsCertificates[0].PrivateKey.GetInlineString()).To(Equal(pk))
			Expect(tlsContext().CommonTlsContext.TlsCertificates[0].CertificateChain.GetInlineString()).To(Equal(cc))
		})

		It("should process an upstream with rootca", func() {

			pk := gloohelpers.PrivateKey()
			cc := gloohelpers.Certificate()
			rca := gloohelpers.Certificate()

			tlsConf.PrivateKey = pk
			tlsConf.CertChain = cc
			tlsConf.RootCa = rca

			translate()
			Expect(tlsContext()).ToNot(BeNil())
			Expect(tlsContext().CommonTlsContext.GetValidationContext().TrustedCa.GetInlineString()).To(Equal(rca))
		})

		Context("SslParameters", func() {

			It("should set upstream SslParameters if defined on upstream", func() {
				upstreamSslParameters := &ssl.SslParameters{
					CipherSuites: []string{"AES256-SHA", "AES256-GCM-SHA384"},
				}

				settingsSslParameters := &ssl.SslParameters{
					CipherSuites: []string{"ECDHE-RSA-AES128-SHA"},
				}

				upstream.SslConfig.Parameters = upstreamSslParameters
				upstream.SslConfig.SslSecrets = &ssl.UpstreamSslConfig_SslFiles{
					SslFiles: &ssl.SSLFiles{
						TlsCert: gloohelpers.Certificate(),
						TlsKey:  gloohelpers.PrivateKey(),
					},
				}
				settings.UpstreamOptions = &v1.UpstreamOptions{
					SslParameters: settingsSslParameters,
				}

				translate()
				Expect(tlsContext()).ToNot(BeNil())
				Expect(tlsContext().CommonTlsContext.TlsParams.CipherSuites).To(Equal(upstreamSslParameters.CipherSuites))
			})

			It("should set settings.UpstreamOptions SslParameters if none defined on upstream", func() {
				settingsSslParameters := &ssl.SslParameters{
					CipherSuites: []string{"ECDHE-RSA-AES128-SHA"},
				}

				upstream.SslConfig.Parameters = nil
				upstream.SslConfig.SslSecrets = &ssl.UpstreamSslConfig_SslFiles{
					SslFiles: &ssl.SSLFiles{
						TlsCert: gloohelpers.Certificate(),
						TlsKey:  gloohelpers.PrivateKey(),
					},
				}
				settings.UpstreamOptions = &v1.UpstreamOptions{
					SslParameters: settingsSslParameters,
				}

				translate()
				Expect(tlsContext()).ToNot(BeNil())
				Expect(tlsContext().CommonTlsContext.TlsParams.CipherSuites).To(Equal(settingsSslParameters.CipherSuites))
			})

		})

		Context("failure", func() {
			It("should fail with an upstream with no tls config", func() {
				_, errs, _ := translator.Translate(params, proxy)
				Expect(errs.Validate()).To(HaveOccurred())
			})

			It("should fail with only private key", func() {
				tlsConf.PrivateKey = gloohelpers.PrivateKey()
				_, errs, _ := translator.Translate(params, proxy)
				Expect(errs.Validate()).To(HaveOccurred())
			})

			It("should fail with only cert chain", func() {
				tlsConf.CertChain = gloohelpers.Certificate()
				_, errs, _ := translator.Translate(params, proxy)
				Expect(errs.Validate()).To(HaveOccurred())
			})
		})
	})

	Context("Ssl", func() {

		var (
			listener *envoy_config_listener_v3.Listener
		)

		prepSsl := func(s []*ssl.SslConfig) {
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
				SslConfigurations: s,
			}
			proxy.Listeners = []*v1.Listener{
				httpListener,
			}
		}

		prep := func(s []*ssl.SslConfig) {
			prepSsl(s)
			translate()

			listeners := snapshot.GetResources(types.ListenerTypeV3).Items
			Expect(listeners).To(HaveLen(1))
			val, found := listeners["http-listener"]
			Expect(found).To(BeTrue())
			listener = val.ResourceProto().(*envoy_config_listener_v3.Listener)
		}
		tlsContext := func(fc *envoy_config_listener_v3.FilterChain) *envoyauth.DownstreamTlsContext {
			if fc.TransportSocket == nil {
				return nil
			}
			return glooutils.MustAnyToMessage(fc.TransportSocket.GetTypedConfig()).(*envoyauth.DownstreamTlsContext)
		}
		Context("files", func() {

			It("should translate ssl correctly", func() {
				prep([]*ssl.SslConfig{
					{
						SslSecrets: &ssl.SslConfig_SslFiles{
							SslFiles: &ssl.SSLFiles{
								TlsCert: gloohelpers.Certificate(),
								TlsKey:  gloohelpers.PrivateKey(),
							},
						},
					},
				})
				Expect(listener.GetFilterChains()).To(HaveLen(1))
				fc := listener.GetFilterChains()[0]
				Expect(tlsContext(fc)).NotTo(BeNil())

				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})

			It("should not merge 2 ssl config if they are different", func() {
				cert1, privateKey1 := gloohelpers.GetCerts(gloohelpers.Params{
					Hosts: "gateway-proxy,knative-proxy,ingress-proxy",
					IsCA:  true,
				})
				cert2, privateKey2 := gloohelpers.GetCerts(gloohelpers.Params{
					Hosts: "gateway-proxy,knative-proxy,ingress-proxy",
					IsCA:  true,
				})

				prep([]*ssl.SslConfig{
					{
						SslSecrets: &ssl.SslConfig_SslFiles{
							SslFiles: &ssl.SSLFiles{
								TlsCert: cert1,
								TlsKey:  privateKey1,
							},
						},
						SniDomains: []string{
							"sni1",
						},
					},
					{
						SslSecrets: &ssl.SslConfig_SslFiles{
							SslFiles: &ssl.SSLFiles{
								TlsCert: cert2,
								TlsKey:  privateKey2,
							},
						},
						SniDomains: []string{
							"sni2",
						},
					},
				})

				Expect(listener.GetFilterChains()).To(HaveLen(2))
				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})

			It("should merge 2 ssl config if they are the same", func() {
				prep([]*ssl.SslConfig{
					{
						SslSecrets: &ssl.SslConfig_SslFiles{
							SslFiles: &ssl.SSLFiles{
								TlsCert: gloohelpers.Certificate(),
								TlsKey:  gloohelpers.PrivateKey(),
							},
						},
					},
					{
						SslSecrets: &ssl.SslConfig_SslFiles{
							SslFiles: &ssl.SSLFiles{
								TlsCert: gloohelpers.Certificate(),
								TlsKey:  gloohelpers.PrivateKey(),
							},
						},
					},
				})

				Expect(listener.GetFilterChains()).To(HaveLen(1))
				fc := listener.GetFilterChains()[0]
				Expect(tlsContext(fc)).NotTo(BeNil())
				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})

			It("should reject configs if different FilterChains have identical FilterChainMatches", func() {
				filterChains := []*envoy_config_listener_v3.FilterChain{
					{
						FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
							DestinationPort: &wrappers.UInt32Value{Value: 1},
						},
					},
					{
						FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
							DestinationPort: &wrappers.UInt32Value{Value: 1},
						},
					},
				}
				report := &validation.ListenerReport{}
				CheckForFilterChainConsistency(filterChains, report, listener)
				Expect(report.Errors).NotTo(BeNil())
				Expect(report.Errors).To(HaveLen(1))
				Expect(report.Errors[0].Type).To(Equal(validation.ListenerReport_Error_SSLConfigError))
			})
			It("should combine sni matches", func() {
				prep([]*ssl.SslConfig{
					{
						SslSecrets: &ssl.SslConfig_SslFiles{
							SslFiles: &ssl.SSLFiles{
								TlsCert: gloohelpers.Certificate(),
								TlsKey:  gloohelpers.PrivateKey(),
							},
						},
						SniDomains: []string{"a.com"},
					},
					{
						SslSecrets: &ssl.SslConfig_SslFiles{
							SslFiles: &ssl.SSLFiles{
								TlsCert: gloohelpers.Certificate(),
								TlsKey:  gloohelpers.PrivateKey(),
							},
						},
						SniDomains: []string{"b.com"},
					},
				})

				Expect(listener.GetFilterChains()).To(HaveLen(1))
				fc := listener.GetFilterChains()[0]
				Expect(tlsContext(fc)).NotTo(BeNil())
				cert := tlsContext(fc).GetCommonTlsContext().GetTlsCertificates()[0]
				Expect(cert.GetCertificateChain().GetFilename()).To(Equal(gloohelpers.Certificate()))
				Expect(cert.GetPrivateKey().GetFilename()).To(Equal(gloohelpers.PrivateKey()))
				Expect(fc.FilterChainMatch.ServerNames).To(Equal([]string{"a.com", "b.com"}))
				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})
			It("should combine 1 that has and 1 that doesn't have sni", func() {

				prep([]*ssl.SslConfig{
					{
						SslSecrets: &ssl.SslConfig_SslFiles{
							SslFiles: &ssl.SSLFiles{
								TlsCert: gloohelpers.Certificate(),
								TlsKey:  gloohelpers.PrivateKey(),
							},
						},
					},
					{
						SslSecrets: &ssl.SslConfig_SslFiles{
							SslFiles: &ssl.SSLFiles{
								TlsCert: gloohelpers.Certificate(),
								TlsKey:  gloohelpers.PrivateKey(),
							},
						},
						SniDomains: []string{"b.com"},
					},
				})

				Expect(listener.GetFilterChains()).To(HaveLen(1))
				fc := listener.GetFilterChains()[0]
				Expect(tlsContext(fc)).NotTo(BeNil())
				Expect(fc.FilterChainMatch.ServerNames).To(BeEmpty())
				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})
		})
		Context("secret refs", func() {
			It("should combine sni matches ", func() {

				params.Snapshot.Secrets = append(params.Snapshot.Secrets, &v1.Secret{
					Metadata: &core.Metadata{
						Name:      "solo",
						Namespace: "solo.io",
					},
					Kind: &v1.Secret_Tls{
						Tls: &v1.TlsSecret{
							CertChain:  gloohelpers.Certificate(),
							PrivateKey: gloohelpers.PrivateKey(),
						},
					},
				})

				prep([]*ssl.SslConfig{
					{
						SslSecrets: &ssl.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
						SniDomains: []string{"a.com"},
					},
					{
						SslSecrets: &ssl.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
						SniDomains: []string{"b.com"},
					},
				})

				Expect(listener.GetFilterChains()).To(HaveLen(1))
				fc := listener.GetFilterChains()[0]
				Expect(tlsContext(fc)).NotTo(BeNil())
				cert := tlsContext(fc).GetCommonTlsContext().GetTlsCertificates()[0]
				Expect(cert.GetCertificateChain().GetInlineString()).To(Equal(gloohelpers.Certificate()))
				Expect(cert.GetPrivateKey().GetInlineString()).To(Equal(gloohelpers.PrivateKey()))
				Expect(fc.FilterChainMatch.ServerNames).To(Equal([]string{"a.com", "b.com"}))
				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})
			It("should not combine when not matching", func() {

				cert1, privateKey1 := gloohelpers.GetCerts(gloohelpers.Params{
					Hosts: "gateway-proxy,knative-proxy,ingress-proxy",
					IsCA:  true,
				})
				cert2, privateKey2 := gloohelpers.GetCerts(gloohelpers.Params{
					Hosts: "gateway-proxy,knative-proxy,ingress-proxy",
					IsCA:  true,
				})
				cert3, privateKey3 := gloohelpers.GetCerts(gloohelpers.Params{
					Hosts: "gateway-proxy,knative-proxy,ingress-proxy",
					IsCA:  true,
				})

				params.Snapshot.Secrets = append(params.Snapshot.Secrets, &v1.Secret{
					Metadata: &core.Metadata{
						Name:      "solo",
						Namespace: "solo.io",
					},
					Kind: &v1.Secret_Tls{
						Tls: &v1.TlsSecret{
							CertChain:  cert1,
							PrivateKey: privateKey1,
						},
					},
				}, &v1.Secret{
					Metadata: &core.Metadata{
						Name:      "solo2",
						Namespace: "solo.io",
					},
					Kind: &v1.Secret_Tls{
						Tls: &v1.TlsSecret{
							CertChain:  cert2,
							PrivateKey: privateKey2,
							RootCa:     cert2,
						},
					},
				}, &v1.Secret{
					Metadata: &core.Metadata{
						Name:      "solo", // check same name with different ns
						Namespace: "solo.io2",
					},
					Kind: &v1.Secret_Tls{
						Tls: &v1.TlsSecret{
							CertChain:  cert3,
							PrivateKey: privateKey3,
						},
					},
				})

				prep([]*ssl.SslConfig{
					{
						SslSecrets: &ssl.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
						SniDomains: []string{"a.com"},
					},
					{
						SslSecrets: &ssl.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo2",
								Namespace: "solo.io",
							},
						},
						SniDomains: []string{"b.com"},
					},
					{
						SslSecrets: &ssl.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io2",
							},
						},
						SniDomains: []string{"c.com"},
					},
					{
						Parameters: &ssl.SslParameters{
							MinimumProtocolVersion: ssl.SslParameters_TLSv1_2,
						},
						SslSecrets: &ssl.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io2",
							},
						},
						SniDomains: []string{"d.com"},
					},
					{
						Parameters: &ssl.SslParameters{
							MinimumProtocolVersion: ssl.SslParameters_TLSv1_2,
						},
						SslSecrets: &ssl.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io2",
							},
						},
						SniDomains: []string{"d.com", "e.com"},
					},
				})

				Expect(listener.GetFilterChains()).To(HaveLen(4))
				By("checking first filter chain")
				fc := listener.GetFilterChains()[0]
				Expect(tlsContext(fc)).NotTo(BeNil())
				cert := tlsContext(fc).GetCommonTlsContext().GetTlsCertificates()[0]
				Expect(cert.GetCertificateChain().GetInlineString()).To(Equal(cert1))
				Expect(cert.GetPrivateKey().GetInlineString()).To(Equal(privateKey1))
				Expect(tlsContext(fc).GetCommonTlsContext().GetValidationContext()).To(BeNil())
				Expect(fc.FilterChainMatch.ServerNames).To(Equal([]string{"a.com"}))

				By("checking second filter chain")
				fc = listener.GetFilterChains()[1]
				Expect(tlsContext(fc)).NotTo(BeNil())
				cert = tlsContext(fc).GetCommonTlsContext().GetTlsCertificates()[0]
				Expect(cert.GetCertificateChain().GetInlineString()).To(Equal(cert2))
				Expect(cert.GetPrivateKey().GetInlineString()).To(Equal(privateKey2))
				Expect(tlsContext(fc).GetCommonTlsContext().GetValidationContext().GetTrustedCa().GetInlineString()).To(Equal(cert2))
				Expect(fc.FilterChainMatch.ServerNames).To(Equal([]string{"b.com"}))

				By("checking third filter chain")
				fc = listener.GetFilterChains()[2]
				Expect(tlsContext(fc)).NotTo(BeNil())
				cert = tlsContext(fc).GetCommonTlsContext().GetTlsCertificates()[0]
				Expect(cert.GetCertificateChain().GetInlineString()).To(Equal(cert3))
				Expect(cert.GetPrivateKey().GetInlineString()).To(Equal(privateKey3))
				Expect(tlsContext(fc).GetCommonTlsContext().GetValidationContext()).To(BeNil())
				Expect(fc.FilterChainMatch.ServerNames).To(Equal([]string{"c.com"}))

				By("checking fourth filter chain")
				fc = listener.GetFilterChains()[3]
				Expect(tlsContext(fc)).NotTo(BeNil())
				cert = tlsContext(fc).GetCommonTlsContext().GetTlsCertificates()[0]
				Expect(cert.GetCertificateChain().GetInlineString()).To(Equal(cert3))
				Expect(cert.GetPrivateKey().GetInlineString()).To(Equal(privateKey3))
				Expect(tlsContext(fc).GetCommonTlsContext().GetValidationContext()).To(BeNil())
				Expect(fc.FilterChainMatch.ServerNames).To(Equal([]string{"d.com", "e.com"}))
				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})
			It("should error when different parameters have the same sni domains", func() {

				params.Snapshot.Secrets = append(params.Snapshot.Secrets, &v1.Secret{
					Metadata: &core.Metadata{
						Name:      "solo",
						Namespace: "solo.io",
					},
					Kind: &v1.Secret_Tls{
						Tls: &v1.TlsSecret{
							CertChain:  gloohelpers.Certificate(),
							PrivateKey: gloohelpers.PrivateKey(),
						},
					},
				})

				prepSsl([]*ssl.SslConfig{
					{
						SslSecrets: &ssl.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
						SniDomains: []string{"a.com"},
					},
					{
						Parameters: &ssl.SslParameters{
							MinimumProtocolVersion: ssl.SslParameters_TLSv1_2,
						},
						SslSecrets: &ssl.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
						SniDomains: []string{"a.com"},
					},
				})
				_, errs, _ := translator.Translate(params, proxy)
				proxyKind := resources.Kind(proxy)
				_, reports := errs.Find(proxyKind, proxy.Metadata.Ref())
				Expect(reports.Errors.Error()).To(ContainSubstring("Tried to apply multiple filter chains with the" +
					" same FilterChainMatch {server_names:\"a.com\"}. This is usually caused by overlapping sniDomains or multiple empty sniDomains in virtual services"))
			})
			It("should error when different parameters have no sni domains", func() {

				params.Snapshot.Secrets = append(params.Snapshot.Secrets, &v1.Secret{
					Metadata: &core.Metadata{
						Name:      "solo",
						Namespace: "solo.io",
					},
					Kind: &v1.Secret_Tls{
						Tls: &v1.TlsSecret{
							CertChain:  gloohelpers.Certificate(),
							PrivateKey: gloohelpers.PrivateKey(),
						},
					},
				})

				prepSsl([]*ssl.SslConfig{
					{
						SslSecrets: &ssl.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
					},
					{
						Parameters: &ssl.SslParameters{
							MinimumProtocolVersion: ssl.SslParameters_TLSv1_2,
						},
						SslSecrets: &ssl.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
					},
				})
				_, errs, _ := translator.Translate(params, proxy)
				proxyKind := resources.Kind(proxy)
				_, reports := errs.Find(proxyKind, proxy.Metadata.Ref())
				Expect(reports.Errors.Error()).To(ContainSubstring("Tried to apply multiple filter chains with the" +
					" same FilterChainMatch {}. This is usually caused by overlapping sniDomains or multiple empty sniDomains in virtual services"))
			})
			It("should work when different parameters have different sni domains", func() {

				params.Snapshot.Secrets = append(params.Snapshot.Secrets, &v1.Secret{
					Metadata: &core.Metadata{
						Name:      "solo",
						Namespace: "solo.io",
					},
					Kind: &v1.Secret_Tls{
						Tls: &v1.TlsSecret{
							CertChain:  gloohelpers.Certificate(),
							PrivateKey: gloohelpers.PrivateKey(),
						},
					},
				})

				prep([]*ssl.SslConfig{
					{
						SslSecrets: &ssl.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
						SniDomains: []string{"a.com"},
					},
					{
						Parameters: &ssl.SslParameters{
							MinimumProtocolVersion: ssl.SslParameters_TLSv1_2,
						},
						SslSecrets: &ssl.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
						SniDomains: []string{"b.com"},
					},
				})
				Expect(listener.GetFilterChains()).To(HaveLen(2))
				By("checking first filter chain")
				fc := listener.GetFilterChains()[0]
				Expect(tlsContext(fc)).NotTo(BeNil())
				cert := tlsContext(fc).GetCommonTlsContext().GetTlsCertificates()[0]
				Expect(cert.GetCertificateChain().GetInlineString()).To(Equal(gloohelpers.Certificate()))
				Expect(cert.GetPrivateKey().GetInlineString()).To(Equal(gloohelpers.PrivateKey()))
				params := tlsContext(fc).GetCommonTlsContext().GetTlsParams()
				Expect(params.GetTlsMinimumProtocolVersion().String()).To(Equal("TLS_AUTO"))
				Expect(tlsContext(fc).GetCommonTlsContext().GetValidationContext()).To(BeNil())
				Expect(fc.FilterChainMatch.ServerNames).To(Equal([]string{"a.com"}))
				By("checking second filter chain")
				fc = listener.GetFilterChains()[1]
				Expect(tlsContext(fc)).NotTo(BeNil())
				cert = tlsContext(fc).GetCommonTlsContext().GetTlsCertificates()[0]
				Expect(cert.GetCertificateChain().GetInlineString()).To(Equal(gloohelpers.Certificate()))
				Expect(cert.GetPrivateKey().GetInlineString()).To(Equal(gloohelpers.PrivateKey()))
				params = tlsContext(fc).GetCommonTlsContext().GetTlsParams()
				Expect(params.GetTlsMinimumProtocolVersion().String()).To(Equal("TLSv1_2"))
				Expect(tlsContext(fc).GetCommonTlsContext().GetValidationContext()).To(BeNil())
				Expect(fc.FilterChainMatch.ServerNames).To(Equal([]string{"b.com"}))
				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})
		})
	})

	It("Should report an error for virtual services with empty domains", func() {
		virtualHosts := []*v1.VirtualHost{
			{
				Domains: []string{"*"},
			},
			{
				Name:    "problem-vhost",
				Domains: []string{"", "nonempty-url.com"},
			},
		}
		report := &validation.HttpListenerReport{VirtualHostReports: []*validation.VirtualHostReport{{}, {}}}

		ValidateVirtualHostDomains(virtualHosts, report)

		Expect(report.VirtualHostReports[0].Errors).To(BeEmpty(), "The virtual host with domain * should not have an error")
		Expect(report.VirtualHostReports[1].Errors).NotTo(BeEmpty(), "The virtual host with an empty domain should report errors")
		Expect(report.VirtualHostReports[1].Errors[0].Type).To(Equal(validation.VirtualHostReport_Error_EmptyDomainError), "The error reported for the virtual host with empty domain should be the EmptyDomainError")
	})

	It("clusterSpecifier is set even when there is an error setting the route action", func() {
		proxy.Listeners[0].GetHttpListener().GetVirtualHosts()[0].Routes = []*v1.Route{
			{
				Name:     "testRouteName",
				Matchers: []*matchers.Matcher{matcher},
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Multi{
							Multi: &v1.MultiDestination{
								Destinations: []*v1.WeightedDestination{
									{
										Weight: &wrappers.UInt32Value{Value: 1},
										Destination: &v1.Destination{
											DestinationType: &v1.Destination_Upstream{
												Upstream: &core.ResourceRef{
													Name:      "test",
													Namespace: "gloo-system",
												},
											},
											Subset: &v1.Subset{
												Values: map[string]string{
													"key": "value",
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
		snap, resourceReport, _ := translator.Translate(params, proxy)
		Expect(resourceReport.ValidateStrict()).To(HaveOccurred())
		routes := snap.GetResources(types.RouteTypeV3)
		routesProto := routes.Items["http-listener-routes"]
		routeConfig := routesProto.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
		clusterSpecifier := routeConfig.VirtualHosts[0].Routes[0].GetRoute().GetClusterSpecifier()
		Expect(clusterSpecifier).NotTo(BeNil())
	})

	Context("IgnoreHealthOnHostRemoval", func() {
		DescribeTable("propagates IgnoreHealthOnHostRemoval to Cluster", func(upstreamValue *wrappers.BoolValue, expectedClusterValue bool) {
			// Set the value
			upstream.IgnoreHealthOnHostRemoval = upstreamValue

			snap, errs, report := translator.Translate(params, proxy)
			Expect(errs.Validate()).NotTo(HaveOccurred())
			Expect(snap).NotTo(BeNil())
			Expect(report).To(Equal(validationutils.MakeReport(proxy)))

			clusters := snap.GetResources(types.ClusterTypeV3)
			clusterResource := clusters.Items[UpstreamToClusterName(upstream.Metadata.Ref())]
			cluster = clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)
			Expect(cluster).NotTo(BeNil())
			Expect(cluster.IgnoreHealthOnHostRemoval).To(Equal(expectedClusterValue))
		},
			Entry("When value=true", &wrappers.BoolValue{Value: true}, true),
			Entry("When value=false", &wrappers.BoolValue{Value: false}, false),
			Entry("When value=nil", nil, false))
	})

	Context("PreconnectPolicy", func() {
		DescribeTable("propagates PreconnectPolicy to Cluster",
			func(upstreamValue *v1.PreconnectPolicy, shouldErr bool) {
				// Set the value to test
				upstream.PreconnectPolicy = upstreamValue

				snap, errs, _ := translator.Translate(params, proxy)
				if shouldErr {
					Expect(errs.Validate()).To(HaveOccurred())
					return
				}
				Expect(snap).NotTo(BeNil())
				Expect(errs.Validate()).To(BeNil())

				clusters := snap.GetResources(types.ClusterTypeV3)
				clusterResource := clusters.Items[UpstreamToClusterName(upstream.Metadata.Ref())]
				cluster = clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)
				Expect(cluster).NotTo(BeNil())

				// The underlying types are different but not individual fields
				actualPrecon := cluster.PreconnectPolicy
				if upstreamValue == nil {
					Expect(actualPrecon).To(BeNil())
					return
				}

				Expect(actualPrecon.PerUpstreamPreconnectRatio).To(Equal(upstreamValue.PerUpstreamPreconnectRatio))

				Expect(actualPrecon.PredictivePreconnectRatio).To(Equal(upstreamValue.PredictivePreconnectRatio))
			},
			Entry("When unset", nil, false),
			Entry("When valid perupstream", &v1.PreconnectPolicy{PerUpstreamPreconnectRatio: asDouble(1)}, false),
			Entry("When valid predictive", &v1.PreconnectPolicy{PredictivePreconnectRatio: asDouble(2)}, false),
			Entry("When valid", &v1.PreconnectPolicy{PerUpstreamPreconnectRatio: asDouble(1), PredictivePreconnectRatio: asDouble(2)}, false),
			// invalid based on proto constraints https://github.com/envoyproxy/envoy/blob/353c0a439ef8bc8eb63bf08c2db4fd3fc3778dec/api/envoy/config/cluster/v3/cluster.proto#L724
			Entry("When invalid perupstream", &v1.PreconnectPolicy{PerUpstreamPreconnectRatio: asDouble(0.5)}, true),
			Entry("When both pieces are invalid", &v1.PreconnectPolicy{PerUpstreamPreconnectRatio: asDouble(0.5), PredictivePreconnectRatio: asDouble(1000)}, true),
			Entry("When valid perupstream but invalid predictive", &v1.PreconnectPolicy{PerUpstreamPreconnectRatio: asDouble(1), PredictivePreconnectRatio: asDouble(1000)}, true),
		)
	})

	Context("DnsRefreshRate", func() {
		DescribeTable("Sets DnsRefreshRate on Cluster",
			func(staticUpstream bool, refreshRate *duration.Duration, refreshRateMatcher types2.GomegaMatcher, reportMatcher types2.GomegaMatcher) {
				// By default, the Upstream is configured as a Static Upstream
				if !staticUpstream {
					upstream.UpstreamType = &v1.Upstream_Kube{
						Kube: &v1kubernetes.UpstreamSpec{
							ServiceName:      "service-name",
							ServiceNamespace: "service-ns",
						},
					}
				}
				upstream.DnsRefreshRate = refreshRate

				snap, errs, _ := translator.Translate(params, proxy)
				Expect(snap).NotTo(BeNil(), "Ensure the xds Snapshot is not nil")
				Expect(errs.ValidateStrict()).To(reportMatcher, "Ensure the reports contain the necessary errors/warnings")

				clusters := snap.GetResources(types.ClusterTypeV3)
				clusterResource := clusters.Items[UpstreamToClusterName(upstream.Metadata.Ref())]
				cluster = clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)
				Expect(cluster.GetDnsRefreshRate()).To(refreshRateMatcher)
			},
			Entry("Static, DnsRefreshRate=nil", true, nil, BeNil(), BeNil()),
			Entry("Static, DsnRefreshRate valid", true, &duration.Duration{Seconds: 1}, MatchProto(&duration.Duration{Seconds: 1}), BeNil()),
			Entry("Static, DsnRefreshRate=0", true, &duration.Duration{Seconds: 0}, BeNil(), MatchError(ContainSubstring("dnsRefreshRate was set below minimum requirement"))),
			Entry("Eds, DsnRefreshRate valid", false, &duration.Duration{Seconds: 1}, MatchProto(&duration.Duration{Seconds: 1}), MatchError(ContainSubstring("DnsRefreshRate is only valid with STRICT_DNS or LOGICAL_DNS cluster type"))),
		)
	})

	//TODO: We could split this into a test file for clusters.go
	Context("Protocol Options", func() {
		It("when no value passed in upstream - cluster has default", func() {
			name := "ProtocolOptionsTest2"
			namespace := "gloo-system"
			upstreamNoProtocol := &v1.Upstream{
				Metadata: &core.Metadata{
					Name:      name,
					Namespace: namespace,
				},
				UpstreamType: &v1.Upstream_Static{
					Static: &v1static.UpstreamSpec{
						Hosts: []*v1static.Host{
							{
								Addr: "poTest1",
								Port: 124,
							},
						},
					},
				},
			}
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, upstreamNoProtocol)
			translate()
			clusters := snapshot.GetResources(types.ClusterTypeV3)
			clusterResource := clusters.Items[fmt.Sprintf("%s_%s", name, namespace)]
			Expect(clusterResource).ToNot(BeNil())
			createdCluster := clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)
			Expect(createdCluster).ToNot(BeNil())
			Expect(createdCluster.ProtocolSelection).To(Equal(envoy_config_cluster_v3.Cluster_USE_CONFIGURED_PROTOCOL))
		})

		It("USE_CONFIGURED_PROTOCOL is passed and set on cluster", func() {
			name := "ProtocolOptionsTest2"
			namespace := "gloo-system"

			upstreamConfiguredProtocol := &v1.Upstream{
				Metadata: &core.Metadata{
					Name:      name,
					Namespace: namespace,
				},
				UpstreamType: &v1.Upstream_Static{
					Static: &v1static.UpstreamSpec{
						Hosts: []*v1static.Host{
							{
								Addr: "poTest2",
								Port: 124,
							},
						},
					},
				},
				ProtocolSelection: v1.Upstream_USE_CONFIGURED_PROTOCOL,
			}
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, upstreamConfiguredProtocol)
			translate()
			clusters := snapshot.GetResources(types.ClusterTypeV3)
			clusterResource := clusters.Items[fmt.Sprintf("%s_%s", name, namespace)]
			Expect(clusterResource).ToNot(BeNil())
			createdCluster := clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)
			Expect(createdCluster).ToNot(BeNil())
			Expect(createdCluster.ProtocolSelection).To(Equal(envoy_config_cluster_v3.Cluster_USE_CONFIGURED_PROTOCOL))
		})

		It("USE_DOWNSTREAM_PROTOCOL is passed and set on cluster", func() {
			name := "ProtocolOptionsTest2"
			namespace := "gloo-system"
			upstreamDownstreamProtocol := &v1.Upstream{
				Metadata: &core.Metadata{
					Name:      name,
					Namespace: namespace,
				},
				UpstreamType: &v1.Upstream_Static{
					Static: &v1static.UpstreamSpec{
						Hosts: []*v1static.Host{
							{
								Addr: "poTest3",
								Port: 124,
							},
						},
					},
				},
				ProtocolSelection: v1.Upstream_USE_DOWNSTREAM_PROTOCOL,
			}
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, upstreamDownstreamProtocol)
			translate()
			clusters := snapshot.GetResources(types.ClusterTypeV3)
			clusterResource := clusters.Items[fmt.Sprintf("%s_%s", name, namespace)]
			Expect(clusterResource).ToNot(BeNil())
			createdCluster := clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)
			Expect(createdCluster).ToNot(BeNil())
			Expect(createdCluster.ProtocolSelection).To(Equal(envoy_config_cluster_v3.Cluster_USE_DOWNSTREAM_PROTOCOL))
		})
	})

})

// The endpoint Cluster is now the UpstreamToClusterName-<hash of upstream> to facilitate
// gRPC EDS updates
func getEndpointClusterName(upstream *v1.Upstream) string {
	return fmt.Sprintf("%s-%d", UpstreamToClusterName(upstream.Metadata.Ref()), upstream.MustHash())
}

func sv(s string) *structpb.Value {
	return &structpb.Value{
		Kind: &structpb.Value_StringValue{
			StringValue: s,
		},
	}
}

type routePluginMock struct {
	ProcessRouteFunc func(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error
}

func (p *routePluginMock) Name() string {
	return "route_plugin_mock"
}

func (p *routePluginMock) Init(_ plugins.InitParams) {
}

func (p *routePluginMock) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	return p.ProcessRouteFunc(params, in, out)
}

type endpointPluginMock struct {
	ProcessEndpointFunc func(params plugins.Params, in *v1.Upstream, out *envoy_config_endpoint_v3.ClusterLoadAssignment) error
}

func (e *endpointPluginMock) ProcessEndpoints(params plugins.Params, in *v1.Upstream, out *envoy_config_endpoint_v3.ClusterLoadAssignment) error {
	return e.ProcessEndpointFunc(params, in, out)
}

func (e *endpointPluginMock) Name() string {
	return "endpoint_plugin_mock"
}

func (e *endpointPluginMock) Init(params plugins.InitParams) {
}

func createStaticUpstream(name, namespace string) *v1.Upstream {
	return &v1.Upstream{
		Metadata: &core.Metadata{
			Name:      name,
			Namespace: namespace,
		},
		UpstreamType: &v1.Upstream_Static{
			Static: &v1static.UpstreamSpec{
				Hosts: []*v1static.Host{
					{
						Addr: "Test" + name,
						Port: 124,
					},
				},
			},
		},
	}
}

func createWeightedDestination(isWeightIncluded bool, weight uint32, upstream *v1.Upstream) *v1.WeightedDestination {
	if isWeightIncluded {
		return &v1.WeightedDestination{
			Weight: &wrappers.UInt32Value{Value: weight},
			Destination: &v1.Destination{
				DestinationType: &v1.Destination_Upstream{
					Upstream: &core.ResourceRef{
						Name:      upstream.Metadata.Name,
						Namespace: upstream.Metadata.Namespace,
					},
				},
			},
		}
	}
	return &v1.WeightedDestination{
		Destination: &v1.Destination{
			DestinationType: &v1.Destination_Upstream{
				Upstream: &core.ResourceRef{
					Name:      upstream.Metadata.Name,
					Namespace: upstream.Metadata.Namespace,
				},
			},
		},
	}
}

func createMultiActionRoute(routeName string, matcher *matchers.Matcher, destinations []*v1.WeightedDestination) *v1.Route {
	return &v1.Route{
		Name:     routeName,
		Matchers: []*matchers.Matcher{matcher},
		Action: &v1.Route_RouteAction{
			RouteAction: &v1.RouteAction{
				Destination: &v1.RouteAction_Multi{
					Multi: &v1.MultiDestination{
						Destinations: destinations,
					},
				},
			},
		},
	}
}

func asDouble(v float64) *wrappers.DoubleValue {
	return &wrappers.DoubleValue{Value: v}
}
