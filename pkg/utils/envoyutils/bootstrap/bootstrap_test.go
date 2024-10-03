package bootstrap

import (
	"log"

	envoy_config_bootstrap_v3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_filters_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	anypb "github.com/golang/protobuf/ptypes/any"
	"github.com/solo-io/gloo/pkg/utils/protoutils"
	envoytransformation "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Static bootstrap generation", func() {
	Context("Util functions", func() {
		var (
			routedCluster map[string]struct{}
			listeners     []*envoy_config_listener_v3.Listener
			routes        []*envoy_config_route_v3.RouteConfiguration
		)
		BeforeEach(func() {
			routedCluster = make(map[string]struct{})
			listeners = make([]*envoy_config_listener_v3.Listener, 10)
			routes = []*envoy_config_route_v3.RouteConfiguration{{
				Name: "foo-routes",
				VirtualHosts: []*envoy_config_route_v3.VirtualHost{
					{
						Name:    "placeholder_host",
						Domains: []string{"*"},
						Routes: []*envoy_config_route_v3.Route{
							{
								Action: &envoy_config_route_v3.Route_Route{
									Route: &envoy_config_route_v3.RouteAction{
										ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{
											Cluster: "foo",
										},
									},
								},
								Name: "foo-route",
							},
							{
								Action: &envoy_config_route_v3.Route_Route{
									Route: &envoy_config_route_v3.RouteAction{
										ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{
											Cluster: "bar",
										},
									},
								},
								Name: "bar-route",
							},
						},
					},
				},
			},
			}
		})
		Context("extractRoutedClustersFromListeners", func() {
			BeforeEach(func() {
			})
			FIt("errors if bad hcm", func() {
				// This case should never happen, but we purport to handle it so we test it here.

				// Create an *Any from a struct that's not a HCM
				notAnHcmAny, err := utils.MessageToAny(&envoy_config_listener_v3.Listener{
					Name:                  "oops-not-hcm",
					UseOriginalDst:        &wrapperspb.BoolValue{Value: true},
					BypassOverloadManager: true,
				})

				// Set the type to be that of HCM
				notAnHcmAny.TypeUrl = "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"

				Expect(err).NotTo(HaveOccurred())
				l := &envoy_config_listener_v3.Listener{
					Name:    "fake-listener",
					Address: &envoy_config_core_v3.Address{},
					FilterChains: []*envoy_config_listener_v3.FilterChain{{
						FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{},
						Filters: []*envoy_config_listener_v3.Filter{{
							Name: wellknown.HTTPConnectionManager,
							ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
								TypedConfig: notAnHcmAny,
							},
						}},
					}},
				}
				listeners = append(listeners, l)
				Expect(extractRoutedClustersFromListeners(routedCluster, listeners, routes)).To(HaveOccurred())
			})
			It("does not error if no hcm", func() {
				l := &envoy_config_listener_v3.Listener{
					Name:    "fake-listener",
					Address: &envoy_config_core_v3.Address{},
					FilterChains: []*envoy_config_listener_v3.FilterChain{{
						FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{},
						Filters:          []*envoy_config_listener_v3.Filter{},
					}},
				}
				listeners = append(listeners, l)
				Expect(extractRoutedClustersFromListeners(routedCluster, listeners, routes)).NotTo(HaveOccurred())
				Expect(routedCluster).To(BeEmpty())
			})
			It("extracts a single happy cluster", func() {
				hcmAny, err := utils.MessageToAny(&envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{
					StatPrefix: "placeholder",
					RouteSpecifier: &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager_Rds{
						Rds: &envoy_extensions_filters_network_http_connection_manager_v3.Rds{
							RouteConfigName: "foo-routes",
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				l := &envoy_config_listener_v3.Listener{
					Name:    "fake-listener",
					Address: &envoy_config_core_v3.Address{},
					FilterChains: []*envoy_config_listener_v3.FilterChain{{
						FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{},
						Filters: []*envoy_config_listener_v3.Filter{{
							Name: wellknown.HTTPConnectionManager,
							ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
								TypedConfig: hcmAny,
							},
						}},
					}},
				}
				listeners = append(listeners, l)
				Expect(extractRoutedClustersFromListeners(routedCluster, listeners, routes)).NotTo(HaveOccurred())
				Expect(routedCluster).To(HaveKey("foo"))
			})
		})
		It("convertToStaticClusters", func() {
		})
		It("addBlackholeClusters", func() {
		})
		It("getHcmForFilterChain", func() {
		})
		It("findTargetedClusters", func() {
		})
		It("setStaticRouteConfig", func() {
		})
	})
	Context("From Filter", func() {
		It("produces correct bootstrap", func() {
			Skip("TODO")
			inTransformation := &envoytransformation.RouteTransformations{
				ClearRouteCache: true,
				Transformations: []*envoytransformation.RouteTransformations_RouteTransformation{
					{
						Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
							RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{ClearRouteCache: true},
						},
					},
				},
			}

			filterName := "transformation"
			actual, err := FromFilter(filterName, inTransformation)
			Expect(err).NotTo(HaveOccurred())

			expectedBootstrap := &envoy_config_bootstrap_v3.Bootstrap{
				Node: &envoy_config_core_v3.Node{
					Id:      "validation-node-id",
					Cluster: "validation-cluster",
				},
				StaticResources: &envoy_config_bootstrap_v3.Bootstrap_StaticResources{
					Listeners: []*envoy_config_listener_v3.Listener{{
						Name: "placeholder_listener",
						Address: &envoy_config_core_v3.Address{
							Address: &envoy_config_core_v3.Address_SocketAddress{SocketAddress: &envoy_config_core_v3.SocketAddress{
								Address:       "0.0.0.0",
								PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{PortValue: 8081},
							}},
						},
						FilterChains: []*envoy_config_listener_v3.FilterChain{
							{
								Name: "placeholder_filter_chain",
								Filters: []*envoy_config_listener_v3.Filter{
									{
										Name: wellknown.HTTPConnectionManager,
										ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
											TypedConfig: func() *anypb.Any {
												hcmAny, err := utils.MessageToAny(&envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{
													StatPrefix: "placeholder",
													RouteSpecifier: &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager_RouteConfig{
														RouteConfig: &envoy_config_route_v3.RouteConfiguration{
															VirtualHosts: []*envoy_config_route_v3.VirtualHost{
																{
																	Name:    "placeholder_host",
																	Domains: []string{"*"},
																	TypedPerFilterConfig: map[string]*anypb.Any{
																		filterName: {
																			TypeUrl: "type.googleapis.com/envoy.api.v2.filter.http.RouteTransformations",
																			Value: func() []byte {
																				tformany, err := utils.MessageToAny(inTransformation)
																				Expect(err).NotTo(HaveOccurred())
																				return tformany.GetValue()
																			}(),
																		},
																	},
																},
															},
														},
													},
												})
												Expect(err).NotTo(HaveOccurred())
												return hcmAny
											}(),
										},
									},
								},
							},
						},
					}},
				},
			}

			var actualBootstrap *envoy_config_bootstrap_v3.Bootstrap

			log.Println(actual)
			err = protoutils.UnmarshalBytesAllowUnknown([]byte(actual), actualBootstrap)
			// err = (&jsonpb.Unmarshaler{
			// 	AllowUnknownFields: true,
			// 	AnyResolver:        nil,
			// }).UnmarshalString(actual, actualBootstrap)
			// err = jsonpb.Unmarshal(bytes.NewBuffer([]byte(actual)), actualBootstrap)
			Expect(err).NotTo(HaveOccurred())

			Expect(proto.Equal(expectedBootstrap, actualBootstrap)).To(BeTrue())
		})
	})
})
