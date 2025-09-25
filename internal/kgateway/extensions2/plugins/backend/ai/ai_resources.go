package ai

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/trafficpolicy"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

const (
	extProcUDSClusterName = "ai_ext_proc_uds_cluster"
	extProcUDSSocketPath  = "@kgateway-ai-sock"
	waitFilterName        = "io.kgateway.wait"
)

func GetAIAdditionalResources() ([]*envoyclusterv3.Cluster, error) {
	// This env var can be used to test the ext-proc filter locally.
	// On linux this should be set to `172.17.0.1` and on mac to `host.docker.internal`
	// Note: Mac doesn't work yet because it needs to be a DNS cluster
	// The port can be whatever you want.
	// When running the ext-proc filter locally, you also need to set
	// `LISTEN_ADDR` to `0.0.0.0:PORT`. Where port is the same port as above.
	listenAddr := strings.Split(os.Getenv(trafficpolicy.AiListenAddr), ":")

	var ep *envoyendpointv3.LbEndpoint
	if len(listenAddr) == 2 {
		port, _ := strconv.Atoi(listenAddr[1])
		ep = &envoyendpointv3.LbEndpoint{
			HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
				Endpoint: &envoyendpointv3.Endpoint{
					Address: &envoycorev3.Address{
						Address: &envoycorev3.Address_SocketAddress{
							SocketAddress: &envoycorev3.SocketAddress{
								Address: listenAddr[0],
								PortSpecifier: &envoycorev3.SocketAddress_PortValue{
									PortValue: uint32(port), //nolint:gosec // G115: port validated to be in range 1-65535 by kubebuilder
								},
							},
						},
					},
				},
			},
		}
	} else {
		ep = &envoyendpointv3.LbEndpoint{
			HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
				Endpoint: &envoyendpointv3.Endpoint{
					Address: &envoycorev3.Address{
						Address: &envoycorev3.Address_Pipe{
							Pipe: &envoycorev3.Pipe{
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
					Http2ProtocolOptions: &envoycorev3.Http2ProtocolOptions{},
				},
			},
		},
	}
	http2ProtocolOptionsAny, err := utils.MessageToAny(http2ProtocolOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to convert http2 protocol options to any: %w", err)
	}
	udsCluster := &envoyclusterv3.Cluster{
		Name: extProcUDSClusterName,
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
			Type: envoyclusterv3.Cluster_STATIC,
		},
		TypedExtensionProtocolOptions: map[string]*anypb.Any{
			"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": http2ProtocolOptionsAny,
		},
		LoadAssignment: &envoyendpointv3.ClusterLoadAssignment{
			ClusterName: extProcUDSClusterName,
			Endpoints: []*envoyendpointv3.LocalityLbEndpoints{
				{
					LbEndpoints: []*envoyendpointv3.LbEndpoint{
						ep,
					},
				},
			},
		},
	}
	// Add UDS cluster for the ext-proc filter
	return []*envoyclusterv3.Cluster{udsCluster}, nil
}
