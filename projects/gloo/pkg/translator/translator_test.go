package translator_test

import (
	"context"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	envoycluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	types "github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	staticplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Translator", func() {
	var (
		settings   *v1.Settings
		translator Translator
		upstream   *v1.Upstream
		proxy      *v1.Proxy
		params     plugins.Params
		matcher    *v1.Matcher

		cluster             *envoyapi.Cluster
		listener            *envoyapi.Listener
		hcm_cfg             *envoyhttp.HttpConnectionManager
		route_configuration *envoyapi.RouteConfiguration
	)

	BeforeEach(func() {
		cluster = nil
		settings = &v1.Settings{}
		tplugins := []plugins.Plugin{
			staticplugin.NewPlugin(),
		}
		translator = NewTranslator(tplugins, settings)

		upname := core.Metadata{
			Name:      "test",
			Namespace: "gloo-system",
		}
		upstream = &v1.Upstream{
			Metadata: upname,
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
				Upstreams: v1.UpstreamsByNamespace{
					"gloo-system": v1.UpstreamList{
						upstream,
					},
				},
			},
		}
		matcher = &v1.Matcher{
			PathSpecifier: &v1.Matcher_Prefix{
				Prefix: "/",
			},
		}
		proxy = &v1.Proxy{
			Metadata: upname,
			Listeners: []*v1.Listener{{
				Name:        "listener",
				BindAddress: "127.0.0.1",
				BindPort:    80,
				ListenerType: &v1.Listener_HttpListener{
					HttpListener: &v1.HttpListener{
						VirtualHosts: []*v1.VirtualHost{{
							Name:    "virt1",
							Domains: []string{"*"},
							Routes: []*v1.Route{{
								Matcher: matcher,
								Action: &v1.Route_RouteAction{
									RouteAction: &v1.RouteAction{
										Destination: &v1.RouteAction_Single{
											Single: &v1.Destination{
												Upstream: upname.Ref(),
											},
										},
									},
								},
							}},
						}},
					},
				},
			}},
		}
	})
	translate := func() {

		snap, errs, err := translator.Translate(params, proxy)
		Expect(err).NotTo(HaveOccurred())
		Expect(errs.Validate()).NotTo(HaveOccurred())
		Expect(snap).NotTo(BeNil())

		clusters := snap.GetResources(xds.ClusterType)
		clusterResource := clusters.Items[UpstreamToClusterName(upstream.Metadata.Ref())]
		cluster = clusterResource.ResourceProto().(*envoyapi.Cluster)
		Expect(cluster).NotTo(BeNil())

		listeners := snap.GetResources(xds.ListenerType)

		listenerResource := listeners.Items["listener"]
		listener = listenerResource.ResourceProto().(*envoyapi.Listener)
		Expect(listener).NotTo(BeNil())

		hcm_filter := listener.FilterChains[0].Filters[0]
		hcm_cfg = &envoyhttp.HttpConnectionManager{}
		err = ParseConfig(&hcm_filter, hcm_cfg)
		Expect(err).NotTo(HaveOccurred())

		routes := snap.GetResources(xds.RouteType)

		Expect(routes.Items).To(HaveKey("listener-routes"))
		routeResource := routes.Items["listener-routes"]
		route_configuration = routeResource.ResourceProto().(*envoyapi.RouteConfiguration)
		Expect(route_configuration).NotTo(BeNil())

	}

	Context("route header match", func() {
		It("should translate header matcher with no value to a PresentMatch", func() {

			matcher.Headers = []*v1.HeaderMatcher{
				{
					Name: "test",
				},
			}
			translate()
			headermatch := route_configuration.VirtualHosts[0].Routes[0].Match.Headers[0]
			Expect(headermatch.Name).To(Equal("test"))
			presentmatch := headermatch.GetPresentMatch()
			Expect(presentmatch).To(BeTrue())
		})

		It("should translate header matcher with value to exact match", func() {

			matcher.Headers = []*v1.HeaderMatcher{
				{
					Name:  "test",
					Value: "testvalue",
				},
			}
			translate()

			headermatch := route_configuration.VirtualHosts[0].Routes[0].Match.Headers[0]
			Expect(headermatch.Name).To(Equal("test"))
			exactmatch := headermatch.GetExactMatch()
			Expect(exactmatch).To(Equal("testvalue"))
		})

		It("should translate header matcher with regex becomes regex match", func() {

			matcher.Headers = []*v1.HeaderMatcher{
				{
					Name:  "test",
					Value: "testvalue",
					Regex: true,
				},
			}
			translate()

			headermatch := route_configuration.VirtualHosts[0].Routes[0].Match.Headers[0]
			Expect(headermatch.Name).To(Equal("test"))
			regex := headermatch.GetRegexMatch()
			Expect(regex).To(Equal("testvalue"))
		})

	})

	Context("circuit breakers", func() {

		It("should NOT translate circuit breakers on upstream", func() {
			translate()
			Expect(cluster.CircuitBreakers).To(BeNil())
		})

		It("should translate circuit breakers on upstream", func() {

			upstream.UpstreamSpec.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &types.UInt32Value{Value: 1},
				MaxPendingRequests: &types.UInt32Value{Value: 2},
				MaxRequests:        &types.UInt32Value{Value: 3},
				MaxRetries:         &types.UInt32Value{Value: 4},
			}

			expectedCircuitBreakers := &envoycluster.CircuitBreakers{
				Thresholds: []*envoycluster.CircuitBreakers_Thresholds{
					{
						MaxConnections:     &types.UInt32Value{Value: 1},
						MaxPendingRequests: &types.UInt32Value{Value: 2},
						MaxRequests:        &types.UInt32Value{Value: 3},
						MaxRetries:         &types.UInt32Value{Value: 4},
					},
				},
			}
			translate()

			Expect(cluster.CircuitBreakers).To(BeEquivalentTo(expectedCircuitBreakers))
		})

		It("should translate circuit breakers on settings", func() {

			settings.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &types.UInt32Value{Value: 1},
				MaxPendingRequests: &types.UInt32Value{Value: 2},
				MaxRequests:        &types.UInt32Value{Value: 3},
				MaxRetries:         &types.UInt32Value{Value: 4},
			}

			expectedCircuitBreakers := &envoycluster.CircuitBreakers{
				Thresholds: []*envoycluster.CircuitBreakers_Thresholds{
					{
						MaxConnections:     &types.UInt32Value{Value: 1},
						MaxPendingRequests: &types.UInt32Value{Value: 2},
						MaxRequests:        &types.UInt32Value{Value: 3},
						MaxRetries:         &types.UInt32Value{Value: 4},
					},
				},
			}
			translate()

			Expect(cluster.CircuitBreakers).To(BeEquivalentTo(expectedCircuitBreakers))
		})

		It("should override circuit breakers on upstream", func() {

			settings.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &types.UInt32Value{Value: 11},
				MaxPendingRequests: &types.UInt32Value{Value: 12},
				MaxRequests:        &types.UInt32Value{Value: 13},
				MaxRetries:         &types.UInt32Value{Value: 14},
			}

			upstream.UpstreamSpec.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &types.UInt32Value{Value: 1},
				MaxPendingRequests: &types.UInt32Value{Value: 2},
				MaxRequests:        &types.UInt32Value{Value: 3},
				MaxRetries:         &types.UInt32Value{Value: 4},
			}

			expectedCircuitBreakers := &envoycluster.CircuitBreakers{
				Thresholds: []*envoycluster.CircuitBreakers_Thresholds{
					{
						MaxConnections:     &types.UInt32Value{Value: 1},
						MaxPendingRequests: &types.UInt32Value{Value: 2},
						MaxRequests:        &types.UInt32Value{Value: 3},
						MaxRetries:         &types.UInt32Value{Value: 4},
					},
				},
			}
			translate()

			Expect(cluster.CircuitBreakers).To(BeEquivalentTo(expectedCircuitBreakers))
		})

	})
})
