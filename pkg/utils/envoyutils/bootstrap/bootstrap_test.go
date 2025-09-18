package bootstrap

import (
	"context"

	envoytransformation "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"

	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_filters_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	hcmTypeUrl = "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"
)

var _ = Describe("Static bootstrap generation", func() {
	var (
		listeners []*envoylistenerv3.Listener
		clusters  []*envoyclusterv3.Cluster
		routes    []*envoyroutev3.RouteConfiguration
		endpoints []*envoyendpointv3.ClusterLoadAssignment
	)
	BeforeEach(func() {
		listeners = []*envoylistenerv3.Listener{}
		clusters = []*envoyclusterv3.Cluster{{
			Name: "foo",
			EdsClusterConfig: &envoyclusterv3.Cluster_EdsClusterConfig{
				ServiceName: "foo-eds",
			},
		}}
		routes = []*envoyroutev3.RouteConfiguration{{
			Name: "foo-routes",
			VirtualHosts: []*envoyroutev3.VirtualHost{
				{
					Name:    "placeholder_host",
					Domains: []string{"*"},
					Routes: []*envoyroutev3.Route{
						{
							Action: &envoyroutev3.Route_Route{
								Route: &envoyroutev3.RouteAction{
									ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{
										Cluster: "foo",
									},
								},
							},
							Name: "foo-route",
						},
						{
							Action: &envoyroutev3.Route_Route{
								Route: &envoyroutev3.RouteAction{
									ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{
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
		endpoints = []*envoyendpointv3.ClusterLoadAssignment{{
			ClusterName: "foo-eds",
		}}
	})
	Context("Util functions", func() {
		var (
			routedCluster map[string]struct{}
		)
		BeforeEach(func() {
			routedCluster = make(map[string]struct{})
		})
		Context("extractRoutedClustersFromListeners", func() {
			It("does not error if no hcm", func() {
				l := &envoylistenerv3.Listener{
					Name:    "fake-listener",
					Address: &envoycorev3.Address{},
					FilterChains: []*envoylistenerv3.FilterChain{{
						FilterChainMatch: &envoylistenerv3.FilterChainMatch{},
						Filters:          []*envoylistenerv3.Filter{},
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
				l := &envoylistenerv3.Listener{
					Name:    "fake-listener",
					Address: &envoycorev3.Address{},
					FilterChains: []*envoylistenerv3.FilterChain{{
						FilterChainMatch: &envoylistenerv3.FilterChainMatch{},
						Filters: []*envoylistenerv3.Filter{{
							Name: wellknown.HTTPConnectionManager,
							ConfigType: &envoylistenerv3.Filter_TypedConfig{
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
		Context("convertToStaticClusters", func() {
			BeforeEach(func() {
				routedCluster = map[string]struct{}{"foo": struct{}{}, "bar": struct{}{}}
				clusters = []*envoyclusterv3.Cluster{{
					Name: "foo",
					EdsClusterConfig: &envoyclusterv3.Cluster_EdsClusterConfig{
						ServiceName: "foo-eds",
					},
				}}
				endpoints = []*envoyendpointv3.ClusterLoadAssignment{{
					ClusterName: "foo-eds",
				}}
			})
			It("converts and removes from routedCluster", func() {
				Expect(clusters[0].GetLoadAssignment()).To(BeNil())
				convertToStaticClusters(routedCluster, clusters, endpoints)
				Expect(routedCluster).To(HaveKey("bar"))
				Expect(routedCluster).NotTo(HaveKey("foo"))
				Expect(clusters).To(HaveLen(1))
				Expect(clusters[0].GetLoadAssignment()).NotTo(BeNil())
				Expect(clusters[0].GetLoadAssignment().GetClusterName()).To(Equal("foo-eds"))
			})
		})
		Context("addBlackholeClusters", func() {
			BeforeEach(func() {
				routedCluster = map[string]struct{}{"bar": struct{}{}}
				clusters = []*envoyclusterv3.Cluster{{
					Name: "foo",
					EdsClusterConfig: &envoyclusterv3.Cluster_EdsClusterConfig{
						ServiceName: "foo-eds",
					},
				}}
				endpoints = []*envoyendpointv3.ClusterLoadAssignment{{
					ClusterName: "foo-eds",
				}}
			})
			It("adds blackhole clusters for missing values", func() {
				clusters = addBlackholeClusters(routedCluster, clusters)
				Expect(clusters).To(HaveLen(2))
				Expect(clusters[1].GetName()).To(Equal("bar"))
				Expect(clusters[1].GetLoadAssignment().GetClusterName()).To(Equal("bar"))
			})
		})
		Context("getHcmForFilterChain", func() {
			It("gets the HCM", func() {
				hcmAny, err := utils.MessageToAny(&envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{
					StatPrefix: "placeholder",
					RouteSpecifier: &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager_Rds{
						Rds: &envoy_extensions_filters_network_http_connection_manager_v3.Rds{
							RouteConfigName: "foo-routes",
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				fc := &envoylistenerv3.FilterChain{
					FilterChainMatch: &envoylistenerv3.FilterChainMatch{},
					Filters: []*envoylistenerv3.Filter{{
						Name: wellknown.HTTPConnectionManager,
						ConfigType: &envoylistenerv3.Filter_TypedConfig{
							TypedConfig: hcmAny,
						},
					}},
				}
				hcm, filter, err := getHcmForFilterChain(fc)
				Expect(err).NotTo(HaveOccurred())
				Expect(hcm.StatPrefix).To(Equal("placeholder"))
				Expect(filter.GetTypedConfig().GetTypeUrl()).To(Equal(hcmTypeUrl))
			})
		})
		Context("findTargetedClusters", func() {
			It("finds clusters targeted by routes", func() {
				findTargetedClusters(routes[0], routedCluster)
				Expect(routedCluster).To(HaveLen(2))
				Expect(routedCluster).To(HaveKey("foo"))
				Expect(routedCluster).To(HaveKey("bar"))
			})
		})
		Context("setStaticRouteConfig", func() {
			It("sets the route config as static and mutates the filter", func() {
				f := &envoylistenerv3.Filter{}
				hcm := &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{}
				Expect(setStaticRouteConfig(f, hcm, routes[0])).NotTo(HaveOccurred())
				Expect(hcm.GetRouteConfig().GetName()).To(Equal(routes[0].GetName()))
				Expect(f.GetTypedConfig().GetTypeUrl()).To(Equal(hcmTypeUrl))
			})
		})
	})
	Context("From Filter", func() {
		It("produces correct bootstrap", func() {
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

			expectedBootstrap := &envoybootstrapv3.Bootstrap{
				Node: &envoycorev3.Node{
					Id:      "validation-node-id",
					Cluster: "validation-cluster",
				},
				StaticResources: &envoybootstrapv3.Bootstrap_StaticResources{
					Listeners: []*envoylistenerv3.Listener{{
						Name: "placeholder_listener",
						Address: &envoycorev3.Address{
							Address: &envoycorev3.Address_SocketAddress{SocketAddress: &envoycorev3.SocketAddress{
								Address:       "0.0.0.0",
								PortSpecifier: &envoycorev3.SocketAddress_PortValue{PortValue: 8081},
							}},
						},
						FilterChains: []*envoylistenerv3.FilterChain{
							{
								Name: "placeholder_filter_chain",
								Filters: []*envoylistenerv3.Filter{
									{
										Name: wellknown.HTTPConnectionManager,
										ConfigType: &envoylistenerv3.Filter_TypedConfig{
											TypedConfig: func() *anypb.Any {
												hcmAny, err := utils.MessageToAny(&envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{
													StatPrefix: "placeholder",
													RouteSpecifier: &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager_RouteConfig{
														RouteConfig: &envoyroutev3.RouteConfiguration{
															VirtualHosts: []*envoyroutev3.VirtualHost{
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

			actualBootstrap := &envoybootstrapv3.Bootstrap{}

			err = protojson.Unmarshal([]byte(actual), actualBootstrap)
			Expect(err).NotTo(HaveOccurred())

			Expect(proto.Equal(expectedBootstrap, actualBootstrap)).To(BeTrue())
		})
	})
	Context("From Snapshot", func() {
		var (
			snap *fakeSnapshot
		)
		BeforeEach(func() {
			snap = &fakeSnapshot{
				m: map[string]envoycache.Resources{
					envoyresource.ListenerType: envoycache.NewResources("", []types.Resource{}),
					envoyresource.ClusterType:  envoycache.NewResources("", []types.Resource{}),
					envoyresource.RouteType:    envoycache.NewResources("", []types.Resource{}),
					envoyresource.EndpointType: envoycache.NewResources("", []types.Resource{}),
				},
			}
			hcmAny, err := utils.MessageToAny(&envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{
				StatPrefix: "placeholder",
				RouteSpecifier: &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager_Rds{
					Rds: &envoy_extensions_filters_network_http_connection_manager_v3.Rds{
						RouteConfigName: "foo-routes",
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			listeners = append(listeners, &envoylistenerv3.Listener{
				Name: "placeholder_listener",
				Address: &envoycorev3.Address{
					Address: &envoycorev3.Address_SocketAddress{SocketAddress: &envoycorev3.SocketAddress{
						Address:       "0.0.0.0",
						PortSpecifier: &envoycorev3.SocketAddress_PortValue{PortValue: 8081},
					}},
				},
				FilterChains: []*envoylistenerv3.FilterChain{{
					Name: "placeholder_filter_chain",
					Filters: []*envoylistenerv3.Filter{{
						Name: wellknown.HTTPConnectionManager,
						ConfigType: &envoylistenerv3.Filter_TypedConfig{
							TypedConfig: hcmAny,
						},
					}},
				}},
			})
		})

		// types.SecretTypeV3 are omitted due to not being converted from snapshot into bootstrap.
		JustBeforeEach(func() {
			for _, l := range listeners {
				snap.m[envoyresource.ListenerType].Items[l.GetName()] = types.ResourceWithTTL{Resource: l}
			}
			for _, c := range clusters {
				snap.m[envoyresource.ClusterType].Items[c.GetName()] = types.ResourceWithTTL{Resource: c}
			}
			for _, r := range routes {
				snap.m[envoyresource.RouteType].Items[r.GetName()] = types.ResourceWithTTL{Resource: r}
			}
			for _, e := range endpoints {
				snap.m[envoyresource.EndpointType].Items[e.GetClusterName()] = types.ResourceWithTTL{Resource: e}
			}
		})
		It("produces correct bootstrap", func() {

			actual, err := FromSnapshot(context.Background(), snap)
			Expect(err).NotTo(HaveOccurred())

			expectedBootstrap := &envoybootstrapv3.Bootstrap{
				Node: &envoycorev3.Node{
					Id:      "validation-node-id",
					Cluster: "validation-cluster",
				},
				StaticResources: &envoybootstrapv3.Bootstrap_StaticResources{
					Clusters: []*envoyclusterv3.Cluster{
						{
							Name: "foo",
							ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
								Type: envoyclusterv3.Cluster_STRICT_DNS,
							},
							LoadAssignment: &envoyendpointv3.ClusterLoadAssignment{
								ClusterName: "foo-eds",
							},
						},
						{
							Name: "bar",
							ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
								Type: envoyclusterv3.Cluster_STATIC,
							},
							LoadAssignment: &envoyendpointv3.ClusterLoadAssignment{
								ClusterName: "bar",
							},
						},
					},
					Listeners: []*envoylistenerv3.Listener{{
						Name: "placeholder_listener",
						Address: &envoycorev3.Address{
							Address: &envoycorev3.Address_SocketAddress{SocketAddress: &envoycorev3.SocketAddress{
								Address:       "0.0.0.0",
								PortSpecifier: &envoycorev3.SocketAddress_PortValue{PortValue: 8081},
							}},
						},
						FilterChains: []*envoylistenerv3.FilterChain{
							{
								Name: "placeholder_filter_chain",
								Filters: []*envoylistenerv3.Filter{
									{
										Name: wellknown.HTTPConnectionManager,
										ConfigType: &envoylistenerv3.Filter_TypedConfig{
											TypedConfig: func() *anypb.Any {
												hcmAny, err := utils.MessageToAny(&envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{
													StatPrefix: "placeholder",
													RouteSpecifier: &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager_RouteConfig{
														RouteConfig: &envoyroutev3.RouteConfiguration{
															Name: "foo-routes",
															VirtualHosts: []*envoyroutev3.VirtualHost{
																{
																	Name:    "placeholder_host",
																	Domains: []string{"*"},
																	Routes: []*envoyroutev3.Route{
																		{
																			Name: "foo-route",
																			Action: &envoyroutev3.Route_Route{
																				Route: &envoyroutev3.RouteAction{
																					ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{
																						Cluster: "foo",
																					},
																				},
																			},
																		},
																		{
																			Name: "bar-route",
																			Action: &envoyroutev3.Route_Route{
																				Route: &envoyroutev3.RouteAction{
																					ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{
																						Cluster: "bar",
																					},
																				},
																			},
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

			actualBootstrap := &envoybootstrapv3.Bootstrap{}

			err = protojson.Unmarshal([]byte(actual), actualBootstrap)
			Expect(err).NotTo(HaveOccurred())

			Expect(proto.Equal(expectedBootstrap, actualBootstrap)).To(BeTrue())
		})
	})
})

type fakeSnapshot struct {
	m map[string]envoycache.Resources
}

func (f *fakeSnapshot) GetResources(typ string) map[string]types.Resource {
	if res, ok := f.m[typ]; ok {
		withoutTTL := make(map[string]types.Resource, len(res.Items))
		for k, v := range res.Items {
			withoutTTL[k] = v.Resource
		}

		return withoutTTL
	}
	panic("unknown resources type" + typ)
}

func (f *fakeSnapshot) GetResourcesAndTTL(typ string) map[string]types.ResourceWithTTL {
	panic("not implemented")
}

// GetVersion should return the current version of the resource indicated
// by typeURL. The version string that is returned is opaque and should
// only be compared for equality.
func (f *fakeSnapshot) GetVersion(typeURL string) string {
	panic("not implemented") // TODO: Implement
}

// ConstructVersionMap is a hint that a delta watch will soon make a
// call to GetVersionMap. The snapshot should construct an internal
// opaque version string for each collection of resource types.
func (f *fakeSnapshot) ConstructVersionMap() error {
	panic("not implemented") // TODO: Implement
}

// GetVersionMap returns a map of resource name to resource version for
// all the resources of type indicated by typeURL.
func (f *fakeSnapshot) GetVersionMap(typeURL string) map[string]string {
	panic("not implemented") // TODO: Implement
}
