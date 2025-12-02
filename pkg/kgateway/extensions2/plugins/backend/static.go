package backend

import (
	"fmt"
	"net/netip"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

// StaticIr is the internal representation of a static backend.
type StaticIr struct {
	clusterType    envoyclusterv3.Cluster_DiscoveryType
	loadAssignment *envoyendpointv3.ClusterLoadAssignment
}

// Equals checks if two StaticIr objects are equal.
func (u *StaticIr) Equals(other any) bool {
	otherStatic, ok := other.(*StaticIr)
	if !ok {
		return false
	}
	return cmputils.CompareWithNils(u, otherStatic, func(a, b *StaticIr) bool {
		return a.clusterType == b.clusterType &&
			proto.Equal(a.loadAssignment, b.loadAssignment)
	})
}

func buildStaticIr(in *v1alpha1.StaticBackend) (*StaticIr, error) {
	ir := &StaticIr{
		clusterType: envoyclusterv3.Cluster_STATIC,
	}

	var hostname string
	for _, host := range in.Hosts {
		if host.Host == "" {
			return nil, fmt.Errorf("addr cannot be empty for host")
		}
		if host.Port == 0 {
			return nil, fmt.Errorf("port cannot be empty for host")
		}

		_, err := netip.ParseAddr(host.Host)
		if err != nil {
			// can't parse ip so this is a dns hostname.
			// save the first hostname for use with sni
			if hostname == "" {
				hostname = host.Host
			}
		}

		if ir.loadAssignment == nil {
			ir.loadAssignment = &envoyendpointv3.ClusterLoadAssignment{
				Endpoints: []*envoyendpointv3.LocalityLbEndpoints{{}},
			}
		}

		healthCheckConfig := &envoyendpointv3.Endpoint_HealthCheckConfig{
			Hostname: host.Host,
		}

		ir.loadAssignment.GetEndpoints()[0].LbEndpoints = append(ir.loadAssignment.GetEndpoints()[0].GetLbEndpoints(),
			&envoyendpointv3.LbEndpoint{
				//	Metadata: getMetadata(params.Ctx, spec, host),
				HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
					Endpoint: &envoyendpointv3.Endpoint{
						Hostname: host.Host,
						Address: &envoycorev3.Address{
							Address: &envoycorev3.Address_SocketAddress{
								SocketAddress: &envoycorev3.SocketAddress{
									Protocol: envoycorev3.SocketAddress_TCP,
									Address:  host.Host,
									PortSpecifier: &envoycorev3.SocketAddress_PortValue{
										PortValue: uint32(host.Port), //nolint:gosec // G115: Gateway API PortNumber is int32 with validation 1-65535, always safe
									},
								},
							},
						},
						HealthCheckConfig: healthCheckConfig,
					},
				},
				//				LoadBalancingWeight: host.GetLoadBalancingWeight(),
			})
	}

	// the upstream has a DNS name. We need Envoy to resolve the DNS name
	if hostname != "" {
		// set the type to strict dns
		ir.clusterType = envoyclusterv3.Cluster_STRICT_DNS

		// do we still need this?
		//		// fix issue where ipv6 addr cannot bind
		//		out.DnsLookupFamily = envoyclusterv3.Cluster_V4_ONLY
	}

	return ir, nil
}

// processStatic applies the static IR to the envoy cluster.
func processStatic(ir *StaticIr, out *envoyclusterv3.Cluster) {
	out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{
		Type: ir.clusterType,
	}

	if ir.loadAssignment != nil {
		// clone needed to avoid adding cluster name to original object in the IR.
		out.LoadAssignment = proto.Clone(ir.loadAssignment).(*envoyendpointv3.ClusterLoadAssignment)
		out.LoadAssignment.ClusterName = out.GetName()
	}
}

func processEndpointsStatic(_ *v1alpha1.StaticBackend) *ir.EndpointsForBackend {
	return nil
}
