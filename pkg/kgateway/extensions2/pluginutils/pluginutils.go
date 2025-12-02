package pluginutils

import (
	"fmt"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

func EnvoySingleEndpointLoadAssignment(out *envoyclusterv3.Cluster, address string, port uint32) {
	out.LoadAssignment = &envoyendpointv3.ClusterLoadAssignment{
		ClusterName: out.GetName(),
		Endpoints: []*envoyendpointv3.LocalityLbEndpoints{
			{
				LbEndpoints: []*envoyendpointv3.LbEndpoint{
					{
						HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
							Endpoint: EnvoyEndpoint(address, port),
						},
					},
				},
			},
		},
	}
}

func EnvoyEndpoint(address string, port uint32) *envoyendpointv3.Endpoint {
	return &envoyendpointv3.Endpoint{
		Address: &envoycorev3.Address{
			Address: &envoycorev3.Address_SocketAddress{
				SocketAddress: &envoycorev3.SocketAddress{
					Address: address,
					PortSpecifier: &envoycorev3.SocketAddress_PortValue{
						PortValue: port,
					},
				},
			},
		},
	}
}

func SetExtensionProtocolOptions(out *envoyclusterv3.Cluster, filterName string, protoext proto.Message) error {
	protoextAny, err := utils.MessageToAny(protoext)
	if err != nil {
		return fmt.Errorf("converting extension %s protocol options to struct: %w", filterName, err)
	}
	if out.GetTypedExtensionProtocolOptions() == nil {
		out.TypedExtensionProtocolOptions = make(map[string]*anypb.Any)
	}

	out.GetTypedExtensionProtocolOptions()[filterName] = protoextAny
	return nil
}
