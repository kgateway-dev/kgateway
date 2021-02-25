package tunneling_test

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/tunneling"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Plugin", func() {

	var (
		params                plugins.Params
		inRouteConfigurations []*envoy_config_route_v3.RouteConfiguration

		us = &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      "http-proxy-upstream",
				Namespace: "gloo-system",
			},
			SslConfig:         nil,
			HttpProxyHostname: "host.com:443",
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
	})

	It("should update listener properly", func() {
		p := tunneling.NewPlugin()
		generatedClusters, _, _, generatedListeners, err := p.GeneratedResources(params, nil, nil, inRouteConfigurations, nil)
		Expect(err).ToNot(HaveOccurred())
		selfCluster := "solo_io_generated_self_cluster_http-proxy-upstream_gloo-system"
		selfPipe := "@/" + translator.UpstreamToClusterName(us.Metadata.Ref())
		Expect(inRouteConfigurations[0].VirtualHosts[0].Routes[0].GetRoute().GetCluster()).To(Equal(selfCluster))
		Expect(generatedClusters).To(Equal([]*envoy_config_cluster_v3.Cluster{
			{
				Name: selfCluster,
				LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: selfCluster,
					Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
						{
							LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
								{
									HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
										Endpoint: &envoy_config_endpoint_v3.Endpoint{
											Address: &envoy_config_core_v3.Address{
												Address: &envoy_config_core_v3.Address_Pipe{
													Pipe: &envoy_config_core_v3.Pipe{
														Path: selfPipe,
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
		}))
		Expect(generatedListeners).To(BeNil())
	})

})
