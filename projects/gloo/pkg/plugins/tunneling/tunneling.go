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

type Plugin struct {
}

func (p *Plugin) Init(_ plugins.InitParams) error {
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
		for _, vh := range rtConfig.GetVirtualHosts() {
			for _, rt := range vh.GetRoutes() {
				rtAction := rt.GetRoute()
				// we do not handle the weighted cluster or cluster header cases
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

					tunnelingHostname := us.GetHttpProxyHostname().GetValue()
					if tunnelingHostname == "" {
						continue
					}

					selfCluster := "solo_io_generated_self_cluster_" + cluster
					selfPipe := "@/" + cluster // use an in-memory pipe to ourselves (only works on linux)

					// update the old cluster to route to ourselves first
					rtAction.ClusterSpecifier = &envoy_config_route_v3.RouteAction_Cluster{Cluster: selfCluster}

					var originalTransportSocket *envoy_config_core_v3.TransportSocket
					for _, inCluster := range inClusters {
						if inCluster.GetName() == cluster {
							originalTransportSocket = inCluster.TransportSocket
							// we copy the transport socket to the generated cluster.
							// the generated cluster will use upstream TLS context to leverage TLS origination;
							// when we encapsulate in HTTP Connect the tcp data being proxied will
							// be encrypted (thus we don't need the original transport socket metadata here)
							inCluster.TransportSocket = nil
							inCluster.TransportSocketMatches = nil
							break
						}
					}

					generatedClusters = append(generatedClusters, generateSelfCluster(selfCluster, selfPipe, originalTransportSocket))
					generatedListeners = append(generatedListeners, generateForwardingTcpListener(cluster, selfPipe, tunnelingHostname))
				}
			}
		}
	}

	return generatedClusters, nil, nil, generatedListeners, nil
}

// the initial route is updated to route to this generated cluster, which routes envoy back to itself (to the
// generated TCP listener, which forwards to the original destination)
//
// the purpose of doing this is to allow both the HTTP Connection Manager filter and TCP filter to run.
// the HTTP Connection Manager runs to allow route-level matching on HTTP parameters (such as request path),
// but then we forward the bytes as raw TCP to the HTTP Connect proxy (which can only be done on a TCP listener)
func generateSelfCluster(selfCluster, selfPipe string, originalTransportSocket *envoy_config_core_v3.TransportSocket) *envoy_config_cluster_v3.Cluster {
	return &envoy_config_cluster_v3.Cluster{
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
	}
}

// the generated cluster routes to this generated listener, which forwards TCP traffic to an HTTP Connect proxy
func generateForwardingTcpListener(cluster, selfPipe, tunnelingHostname string) *envoy_config_listener_v3.Listener {
	cfg := &envoytcp.TcpProxy{
		StatPrefix:       "soloioTcpStats" + cluster,
		TunnelingConfig:  &envoytcp.TcpProxy_TunnelingConfig{Hostname: tunnelingHostname},
		ClusterSpecifier: &envoytcp.TcpProxy_Cluster{Cluster: cluster}, // route to original target
	}

	return &envoy_config_listener_v3.Listener{
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
	}
}
