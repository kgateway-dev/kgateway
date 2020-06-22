package pluginutils

import (
	envoycluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
)

func EnvoySingleEndpointLoadAssignment(out *envoycluster.Cluster, address string, port uint32) {
	out.LoadAssignment = &envoyendpoint.ClusterLoadAssignment{
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
	}
}
