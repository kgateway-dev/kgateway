package pluginutils

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyendpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
)

func EnvoySingleEndpointLoadAssignment(out *envoyapi.Cluster, address string, port uint32) {
	out.LoadAssignment = &envoyapi.ClusterLoadAssignment{
		ClusterName: out.Name,
		Endpoints: []*envoyendpoint.LocalityLbEndpoints{
			{
				LbEndpoints: []*envoyendpoint.LbEndpoint{
					{
						HostIdentifier: &envoyendpoint.LbEndpoint_Endpoint{
							Endpoint: EnvoyEndpoint(address, port),
						},
					},
				},
			},
		},
	}
}

func EnvoyEndpoint(address string, port uint32) *envoyendpoint.Endpoint {
	return &envoyendpoint.Endpoint{
		Address: &envoycore.Address{
			Address: &envoycore.Address_SocketAddress{
				SocketAddress: &envoycore.SocketAddress{
					Address: address,
					PortSpecifier: &envoycore.SocketAddress_PortValue{
						PortValue: port,
					},
				},
			},
		},
		Hostname: address,
		HealthCheckConfig: &envoyendpoint.Endpoint_HealthCheckConfig{
			Hostname: address,
		},
	}
}
