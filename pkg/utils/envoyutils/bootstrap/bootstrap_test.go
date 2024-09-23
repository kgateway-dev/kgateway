package bootstrap_test

import (
	"log"

	envoy_config_bootstrap_v3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_filters_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	anypb "github.com/golang/protobuf/ptypes/any"
	. "github.com/solo-io/gloo/pkg/utils/envoyutils/bootstrap"
	"github.com/solo-io/gloo/pkg/utils/protoutils"
	envoytransformation "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"google.golang.org/protobuf/proto"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Static bootstrap generation", func() {
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
