package tunneling_test

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/tunneling"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/skv2/test/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"google.golang.org/protobuf/proto"
)

const (
	httpProxyHostname string = "host.com:443"
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
			HttpProxyHostname: &wrappers.StringValue{Value: httpProxyHostname},
		}
	)

	BeforeEach(func() {

		params = plugins.Params{
			Snapshot: &v1snap.ApiSnapshot{
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
								Name: "testroute",
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

		// use UpstreamToClusterName to emulate a real translation loop.
		clusterName := translator.UpstreamToClusterName(us.Metadata.Ref())
		inClusters = []*envoy_config_cluster_v3.Cluster{
			{
				Name: clusterName,
				LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: clusterName,
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

	Context("UpstreamTlsContext", func() {
		BeforeEach(func() {
			// add an UpstreamTlsContext
			cfg, err := utils.MessageToAny(&envoyauth.UpstreamTlsContext{
				CommonTlsContext: &envoyauth.CommonTlsContext{},
				Sni:              httpProxyHostname,
			})
			Expect(err).ToNot(HaveOccurred())
			inClusters[0].TransportSocket = &envoy_config_core_v3.TransportSocket{
				Name: "",
				ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{
					TypedConfig: cfg,
				},
			}

			inRoute := inRouteConfigurations[0].VirtualHosts[0].Routes[0]

			// update route input with duplicate route, the duplicate points to the same upstream as existing
			dupRoute := proto.Clone(inRoute).(*envoy_config_route_v3.Route)
			dupRoute.Name = dupRoute.Name + "-duplicate"
			dupRoute.Action = &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{
						Cluster: translator.UpstreamToClusterName(us.Metadata.Ref()),
					},
				},
			}
			inRouteConfigurations[0].VirtualHosts[0].Routes = append(inRouteConfigurations[0].VirtualHosts[0].Routes, dupRoute)
		})

		It("should allow multiple routes to same upstream", func() {
			p := tunneling.NewPlugin()
			generatedClusters, _, _, _, err := p.GeneratedResources(params, inClusters, nil, inRouteConfigurations, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(generatedClusters).To(HaveLen(1), "should generate a single cluster for the upstream")
			Expect(generatedClusters[0].GetTransportSocket()).ToNot(BeNil())
		})
	})
	Context("multiple routes and clusters", func() {

		BeforeEach(func() {

			usCopy := &v1.Upstream{}
			us.DeepCopyInto(usCopy)
			usCopy.Metadata.Name = usCopy.Metadata.Name + "-copy"

			// update snapshot with copied upstream, pointing to same HTTP proxy. copied upstream only has different name
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, usCopy)

			// update route input with duplicate route, the copy points to the cluster correlating to the copied upstream
			inRoute := inRouteConfigurations[0].VirtualHosts[0].Routes[0]
			cpRoute := proto.Clone(inRoute).(*envoy_config_route_v3.Route)
			cpRoute.Name = cpRoute.Name + "-copy"
			cpRoute.Action = &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{
						Cluster: translator.UpstreamToClusterName(usCopy.Metadata.Ref()),
					},
				},
			}
			inRouteConfigurations[0].VirtualHosts[0].Routes = append(inRouteConfigurations[0].VirtualHosts[0].Routes, cpRoute)

			// update cluster input such that copied cluster's name matches the copied upstream
			inCluster := inClusters[0]
			cpCluster := proto.Clone(inCluster).(*envoy_config_cluster_v3.Cluster)
			cpCluster.Name = translator.UpstreamToClusterName(usCopy.Metadata.Ref())
			inClusters = append(inClusters, cpCluster)
		})

		It("should namespace generated clusters, avoiding duplicates", func() {
			p := tunneling.NewPlugin()

			generatedClusters, _, _, _, err := p.GeneratedResources(params, inClusters, nil, inRouteConfigurations, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(generatedClusters).To(HaveLen(2), "should generate a cluster for each input route")

			Expect(generatedClusters[0].Name).ToNot(Equal(generatedClusters[1].Name), "should not have name collisions")
			Expect(generatedClusters[0].LoadAssignment.ClusterName).ToNot(Equal(generatedClusters[1].LoadAssignment.ClusterName), "should not route to same cluster")
			cluster1Pipe := generatedClusters[0].LoadAssignment.Endpoints[0].LbEndpoints[0].GetEndpoint().GetAddress().GetPipe().GetPath()
			cluster2Pipe := generatedClusters[1].LoadAssignment.Endpoints[0].LbEndpoints[0].GetEndpoint().GetAddress().GetPipe().GetPath()
			Expect(cluster1Pipe).ToNot(BeEmpty(), "generated cluster should be in-memory pipe to self")
			Expect(cluster2Pipe).ToNot(BeEmpty(), "generated cluster should be in-memory pipe to self")
			Expect(cluster1Pipe).ToNot(Equal(cluster2Pipe), "should not reuse the same pipe to same generated tcp listener")

			// wipe the fields we expect to be different
			generatedClusters[0].Name = ""
			generatedClusters[0].LoadAssignment.ClusterName = ""
			generatedClusters[0].LoadAssignment.Endpoints[0].LbEndpoints[0] = nil
			generatedClusters[1].Name = ""
			generatedClusters[1].LoadAssignment.ClusterName = ""
			generatedClusters[1].LoadAssignment.Endpoints[0].LbEndpoints[0] = nil

			Expect(generatedClusters[0]).To(matchers.MatchProto(generatedClusters[1]), "generated clusters should be identical, barring name, clustername, endpoints")
		})

		It("should namespace generated listeners, avoiding duplicates", func() {
			p := tunneling.NewPlugin()

			_, _, _, generatedListeners, err := p.GeneratedResources(params, inClusters, nil, inRouteConfigurations, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(generatedListeners).To(HaveLen(2), "should generate a listener for each input route")

			listener1Pipe := generatedListeners[0].GetAddress().GetPipe().GetPath()
			listener2Pipe := generatedListeners[1].GetAddress().GetPipe().GetPath()
			Expect(listener1Pipe).ToNot(BeEmpty(), "generated listener should be in-memory pipe to self")
			Expect(listener2Pipe).ToNot(BeEmpty(), "generated listener should be in-memory pipe to self")

			Expect(listener1Pipe).ToNot(Equal(listener2Pipe), "should not reuse the same pipe to same generated tcp listener")
			Expect(generatedListeners[0].Name).ToNot(Equal(generatedListeners[1].Name), "should not have name collisions")

			listener1TcpCfg := utils.MustAnyToMessage(generatedListeners[0].FilterChains[0].Filters[0].GetTypedConfig()).(*envoytcp.TcpProxy)
			listener2TcpCfg := utils.MustAnyToMessage(generatedListeners[1].FilterChains[0].Filters[0].GetTypedConfig()).(*envoytcp.TcpProxy)

			Expect(listener1TcpCfg.GetStatPrefix()).ToNot(Equal(listener2TcpCfg.GetStatPrefix()), "should not reuse the same stats prefix on generated tcp listeners")

			// wipe the fields we expect to be different
			generatedListeners[0].Name = ""
			generatedListeners[0].Address = nil
			generatedListeners[0].FilterChains[0].Filters[0].ConfigType = nil
			generatedListeners[1].Name = ""
			generatedListeners[1].Address = nil
			generatedListeners[1].FilterChains[0].Filters[0].ConfigType = nil

			Expect(generatedListeners[0]).To(matchers.MatchProto(generatedListeners[1]), "generated listeners should be identical, barring name, address, and tcp stats prefix")
		})
	})

})
