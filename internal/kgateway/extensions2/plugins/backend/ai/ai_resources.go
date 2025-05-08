package ai

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

const (
	extProcUDSClusterName = "ai_ext_proc_uds_cluster"
	extProcUDSSocketPath  = "@kgateway-ai-sock"
	waitFilterName        = "io.kgateway.wait"
)

func GetAIAdditionalResources(ctx context.Context) []*envoy_config_cluster_v3.Cluster {
	// This env var can be used to test the ext-proc filter locally.
	// On linux this should be set to `172.17.0.1` and on mac to `host.docker.internal`
	// Note: Mac doesn't work yet because it needs to be a DNS cluster
	// The port can be whatever you want.
	// When running the ext-proc filter locally, you also need to set
	// `LISTEN_ADDR` to `0.0.0.0:PORT`. Where port is the same port as above.
	// TODO: clean up and centralize the processing of env vars (https://github.com/kgateway-dev/kgateway/issues/10721
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

	http2ProtocolOptions := &envoy_upstreams_v3.HttpProtocolOptions{
		UpstreamProtocolOptions: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
					Http2ProtocolOptions: &envoy_config_core_v3.Http2ProtocolOptions{},
				},
			},
		},
	}
	http2ProtocolOptionsAny, err := utils.MessageToAny(http2ProtocolOptions)
	if err != nil {
		slog.Error("error converting http2 protocol options to any", "error", err)
		return nil
	}
	udsCluster := &envoy_config_cluster_v3.Cluster{
		Name: extProcUDSClusterName,
		ClusterDiscoveryType: &envoy_config_cluster_v3.Cluster_Type{
			Type: envoy_config_cluster_v3.Cluster_STATIC,
		},
		TypedExtensionProtocolOptions: map[string]*anypb.Any{
			"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": http2ProtocolOptionsAny,
		},
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
