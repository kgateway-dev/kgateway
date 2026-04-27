package backend

import (
	"fmt"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

// InternalListenerIr is the internal representation of an internal-listener backend.
type InternalListenerIr struct {
	listenerPort int32
}

// Equals checks if two InternalListenerIr objects are equal.
func (u *InternalListenerIr) Equals(other *InternalListenerIr) bool {
	if u == nil && other == nil {
		return true
	}
	if u == nil || other == nil {
		return false
	}
	return u.listenerPort == other.listenerPort
}

// buildInternalListenerIr converts the kgateway API type to the IR.
func buildInternalListenerIr(in *kgateway.InternalListenerBackend) (*InternalListenerIr, error) {
	if in.ListenerPort <= 0 || in.ListenerPort > 65535 {
		return nil, fmt.Errorf("internalListener.listenerPort must be between 1 and 65535, got %d", in.ListenerPort)
	}
	return &InternalListenerIr{listenerPort: in.ListenerPort}, nil
}

// processInternalListener applies the internal listener IR to the Envoy cluster.
// It creates a STATIC cluster whose single endpoint address is an EnvoyInternalAddress
// pointing at the named internal listener (e.g. "listener~8081").
func processInternalListener(ir *InternalListenerIr, out *envoyclusterv3.Cluster) error {
	if ir == nil {
		return errors.New("ir is nil")
	}

	// The server_listener_name must match the xDS listener name produced by
	// GenerateListenerNameFromPort (pkg/kgateway/translator/listener/gateway_listener_translator.go).
	serverListenerName := fmt.Sprintf("listener~%d", ir.listenerPort)

	out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{
		Type: envoyclusterv3.Cluster_STATIC,
	}
	out.LoadAssignment = &envoyendpointv3.ClusterLoadAssignment{
		ClusterName: out.GetName(),
		Endpoints: []*envoyendpointv3.LocalityLbEndpoints{
			{
				LbEndpoints: []*envoyendpointv3.LbEndpoint{
					{
						HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
							Endpoint: &envoyendpointv3.Endpoint{
								Address: &envoycorev3.Address{
									Address: &envoycorev3.Address_EnvoyInternalAddress{
										EnvoyInternalAddress: &envoycorev3.EnvoyInternalAddress{
											AddressNameSpecifier: &envoycorev3.EnvoyInternalAddress_ServerListenerName{
												ServerListenerName: serverListenerName,
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

	return nil
}
