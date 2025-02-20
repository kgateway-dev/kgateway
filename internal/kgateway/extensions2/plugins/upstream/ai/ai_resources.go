package ai

import (
	"os"
	"strconv"
	"strings"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
)

const (
	extProcUDSClusterName = "ai_ext_proc_uds_cluster"
	extProcUDSSocketPath  = "@kgateway-ai-sock"
	waitFilterName        = "io.kgateway.wait"
)

func GetAIAdditionalResources() []*envoy_config_cluster_v3.Cluster {
	// This env var can be used to test the ext-proc filter locally.
	// On linux this should be set to `172.17.0.1` and on mac to `host.docker.internal`
	// Note: Mac doesn't work yet because it needs to be a DNS cluster
	// The port can be whatever you want.
	// When running the ext-proc filter locally, you also need to set
	// `LISTEN_ADDR` to `0.0.0.0:PORT`. Where port is the same port as above.
	listenAddr := strings.Split(os.Getenv("AI_PLUGIN_LISTEN_ADDR"), ":")

	var ep *envoy_config_endpoint_v3.LbEndpoint
	if len(listenAddr) == 2 {
		port, _ := strconv.Atoi(listenAddr[1])
		ep = &envoy_config_endpoint_v3.LbEndpoint{
			HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
				Endpoint: &envoy_config_endpoint_v3.Endpoint{
					Address: &envoy_config_core_v3.Address{
						Address: &envoy_config_core_v3.Address_SocketAddress{
							SocketAddress: &envoy_config_core_v3.SocketAddress{
								Address: listenAddr[0],
								PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
									PortValue: uint32(port),
								},
							},
						},
					},
				},
			},
		}
	} else {
		ep = &envoy_config_endpoint_v3.LbEndpoint{
			HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
				Endpoint: &envoy_config_endpoint_v3.Endpoint{
					Address: &envoy_config_core_v3.Address{
						Address: &envoy_config_core_v3.Address_Pipe{
							Pipe: &envoy_config_core_v3.Pipe{
								Path: extProcUDSSocketPath,
							},
						},
					},
				},
			},
		}
	}
	udsCluster := &envoy_config_cluster_v3.Cluster{
		Name: extProcUDSClusterName,
		ClusterDiscoveryType: &envoy_config_cluster_v3.Cluster_Type{
			Type: envoy_config_cluster_v3.Cluster_STATIC,
		},
		Http2ProtocolOptions: &envoy_config_core_v3.Http2ProtocolOptions{},
		LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
			ClusterName: extProcUDSClusterName,
			Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
				{
					LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
						ep,
					},
				},
			},
		},
	}
	// Add UDS cluster for the ext-proc filter
	return []*envoy_config_cluster_v3.Cluster{udsCluster}
}
