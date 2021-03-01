package tunneling

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

func NewPlugin() *Plugin {
	return &Plugin{}
}

var _ plugins.Plugin = new(Plugin)
var _ plugins.ResourceGeneratorPlugin = new(Plugin)

//TODO(kdorosh) make sure upgradeable

type Plugin struct {
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) GeneratedResources(params plugins.Params,
	inClusters []*envoy_config_cluster_v3.Cluster,
	inEndpoints []*envoy_config_endpoint_v3.ClusterLoadAssignment,
	inRouteConfigurations []*envoy_config_route_v3.RouteConfiguration,
	inListeners []*envoy_config_listener_v3.Listener,
) ([]*envoy_config_cluster_v3.Cluster, []*envoy_config_endpoint_v3.ClusterLoadAssignment, []*envoy_config_route_v3.RouteConfiguration, []*envoy_config_listener_v3.Listener, error) {

	var generatedClusters []*envoy_config_cluster_v3.Cluster
	var generatedListeners []*envoy_config_listener_v3.Listener

	upstreams := params.Snapshot.Upstreams

	// find all the route config that points to upstreams with tunneling
	for _, rtConfig := range inRouteConfigurations {
		for _, vh := range rtConfig.VirtualHosts {
			for _, rt := range vh.Routes {
				rtAction := rt.GetRoute()
				if cluster := rtAction.GetCluster(); cluster != "" {

					ref, err := translator.ClusterToUpstreamRef(cluster)
					if err != nil {
						// return what we have so far, so that any modified input resources can still route
						// successfully to their generated targets
						return generatedClusters, nil, nil, generatedListeners, nil
					}

					us, err := upstreams.Find(ref.GetNamespace(), ref.GetName())
					if err != nil {
						// return what we have so far, so that any modified input resources can still route
						// successfully to their generated targets
						return generatedClusters, nil, nil, generatedListeners, nil
					}

					tunnelingHostname := us.GetHttpProxyHostname()
					if tunnelingHostname.GetValue() == "" {
						continue
					}

					selfCluster := "solo_io_generated_self_cluster_" + cluster
					selfPipe := "@/" + cluster

					// update the old cluster to route to ourselves first
					rtAction.ClusterSpecifier = &envoy_config_route_v3.RouteAction_Cluster{Cluster: selfCluster}

					var originalTransportSocket *envoy_config_core_v3.TransportSocket
					for _, inCluster := range inClusters {
						if inCluster.Name == cluster && inCluster.TransportSocket != nil {
							tmp := *inCluster.TransportSocket
							originalTransportSocket = &tmp
							// we copy the transport socket to the generated cluster.
							// the generated cluster will use upstream TLS context to leverage TLS,
							// and when we encapsulate in HTTP Connect the tcp data being proxied will
							// be secured (thus we don't need the original transport socket metadata here)
							inCluster.TransportSocket = nil
							inCluster.TransportSocketMatches = nil
						}
					}

					generatedClusters = append(generatedClusters, &envoy_config_cluster_v3.Cluster{
						ClusterDiscoveryType: &envoy_config_cluster_v3.Cluster_Type{
							Type: envoy_config_cluster_v3.Cluster_STATIC,
						},
						ConnectTimeout:  &duration.Duration{Seconds: 5},
						Name:            selfCluster,
						TransportSocket: originalTransportSocket,
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
					})

					cfg := &envoytcp.TcpProxy{
						StatPrefix:       "soloioTcpStats" + cluster,
						TunnelingConfig:  &envoytcp.TcpProxy_TunnelingConfig{Hostname: tunnelingHostname.GetValue()},
						ClusterSpecifier: &envoytcp.TcpProxy_Cluster{Cluster: cluster}, // route to original target
					}

					generatedListeners = append(generatedListeners, &envoy_config_listener_v3.Listener{
						Name: "solo_io_generated_self_listener_" + cluster,
						Address: &envoy_config_core_v3.Address{
							Address: &envoy_config_core_v3.Address_Pipe{
								Pipe: &envoy_config_core_v3.Pipe{
									Path: selfPipe,
								},
							},
						},
						FilterChains: []*envoy_config_listener_v3.FilterChain{
							{
								Filters: []*envoy_config_listener_v3.Filter{
									{
										Name: "tcp",
										ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
											TypedConfig: utils.MustMessageToAny(cfg),
										},
									},
								},
							},
						},
					})
				}
			}
		}
	}

	return generatedClusters, nil, nil, generatedListeners, nil
}
