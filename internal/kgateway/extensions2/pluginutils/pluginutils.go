package pluginutils

import (
	"fmt"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

func EnvoySingleEndpointLoadAssignment(out *envoy_config_cluster_v3.Cluster, address string, port uint32) {
	out.LoadAssignment = &envoy_config_endpoint_v3.ClusterLoadAssignment{
		ClusterName: out.GetName(),
		Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
			{
				LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
					{
						HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
							Endpoint: EnvoyEndpoint(address, port),
						},
					},
				},
			},
		},
	}
}

func EnvoyEndpoint(address string, port uint32) *envoy_config_endpoint_v3.Endpoint {
	return &envoy_config_endpoint_v3.Endpoint{
		Address: &envoy_config_core_v3.Address{
			Address: &envoy_config_core_v3.Address_SocketAddress{
				SocketAddress: &envoy_config_core_v3.SocketAddress{
					Address: address,
					PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
						PortValue: port,
					},
				},
			},
		},
	}
}

func SetExtensionProtocolOptions(out *envoy_config_cluster_v3.Cluster, filterName string, protoext proto.Message) error {
	protoextAny, err := utils.MessageToAny(protoext)
	if err != nil {
		return errors.Wrapf(err, "converting extension %s protocol options to struct", filterName)
	}
	if out.GetTypedExtensionProtocolOptions() == nil {
		out.TypedExtensionProtocolOptions = make(map[string]*anypb.Any)
	}

	out.GetTypedExtensionProtocolOptions()[filterName] = protoextAny
	return nil
}

// BackendToEnvoyCluster converts a name namespaced backendref and creates a valid envoy cluster name
func BackendToEnvoyCluster(backendRef *gwv1.BackendRef) string {
	// For non-namespaced resources, return only name
	if backendRef.Namespace == nil {
		return string(backendRef.Name)
	}

	// Don't use dots in the name as it messes up prometheus stats
	return fmt.Sprintf("%s_%s", backendRef.Name, *backendRef.Namespace)
}
