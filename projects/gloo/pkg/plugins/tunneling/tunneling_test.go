package tunneling_test

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/tunneling"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Plugin", func() {

	var (
		params                plugins.Params
		inRouteConfigurations []*envoy_config_route_v3.RouteConfiguration
		inClusters            []*envoy_config_cluster_v3.Cluster

		us = &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      "http-proxy-upstream",
				Namespace: "gloo-system",
			},
			SslConfig:         nil,
			HttpProxyHostname: &wrappers.StringValue{Value: "host.com:443"},
		}
	)

	BeforeEach(func() {

		params = plugins.Params{
			Snapshot: &v1.ApiSnapshot{
				Upstreams: []*v1.Upstream{us},
			},
		}

		inRouteConfigurations = []*envoy_config_route_v3.RouteConfiguration{
			{
				Name: "listener-::-11082-routes",
				VirtualHosts: []*envoy_config_route_v3.VirtualHost{
					{
						Name:    "gloo-system_vs",
						Domains: []string{"*"},
						Routes: []*envoy_config_route_v3.Route{
							{
								Match: &envoy_config_route_v3.RouteMatch{
									PathSpecifier: &envoy_config_route_v3.RouteMatch_Prefix{
										Prefix: "/",
									},
								},
								Action: &envoy_config_route_v3.Route_Route{
									Route: &envoy_config_route_v3.RouteAction{
										ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{
											Cluster: translator.UpstreamToClusterName(us.Metadata.Ref()),
										},
									},
								},
							},
						},
					},
				},
			},
		}

		inClusters = []*envoy_config_cluster_v3.Cluster{
			{
				Name: "http_proxy",
				LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "http_proxy",
					Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
						{
							LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
								{
									HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
										Endpoint: &envoy_config_endpoint_v3.Endpoint{
											Address: &envoy_config_core_v3.Address{
												Address: &envoy_config_core_v3.Address_SocketAddress{
													SocketAddress: &envoy_config_core_v3.SocketAddress{
														Address: "192.168.0.1",
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
	})

	It("should update resources properly", func() {
		p := tunneling.NewPlugin()

		originalCluster := inRouteConfigurations[0].GetVirtualHosts()[0].GetRoutes()[0].GetRoute().GetCluster()

		generatedClusters, _, _, generatedListeners, err := p.GeneratedResources(params, inClusters, nil, inRouteConfigurations, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(generatedClusters).ToNot(BeNil())
		Expect(generatedListeners).ToNot(BeNil())

		// follow the new request path through envoy

		// step 1. original route now routes to generated cluster
		modifiedRouteCluster := inRouteConfigurations[0].GetVirtualHosts()[0].GetRoutes()[0].GetRoute().GetCluster()
		Expect(modifiedRouteCluster).To(Equal(generatedClusters[0].GetName()), "old route should now route to generated self tcp cluster")

		// step 2. generated self tcp cluster should pipe to in memory tcp listener
		selfClusterPipe := generatedClusters[0].GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetPipe()
		selfListenerPipe := generatedListeners[0].GetAddress().GetPipe()
		Expect(selfClusterPipe).To(Equal(selfListenerPipe), "we should be routing to ourselves")

		// step 3. generated listener encapsulates tcp data in HTTP CONNECT and sends to the original destination
		generatedTcpConfig := generatedListeners[0].GetFilterChains()[0].GetFilters()[0].GetTypedConfig()
		typedTcpConfig := utils.MustAnyToMessage(generatedTcpConfig).(*envoytcp.TcpProxy)
		Expect(typedTcpConfig.GetCluster()).To(Equal(originalCluster), "should forward to original destination")
	})

})
