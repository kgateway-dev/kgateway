package backend

import (
	"fmt"
	"net/netip"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoydnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dns/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

const dnsClusterExtensionName = "envoy.clusters.dns"

// StaticIr is the internal representation of a static backend.
type StaticIr struct {
	// +noKrtEquals
	clusterType envoyclusterv3.Cluster_DiscoveryType
	// +noKrtEquals
	clusterTypeConfig *anypb.Any
	// +noKrtEquals
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
			proto.Equal(a.clusterTypeConfig, b.clusterTypeConfig) &&
			proto.Equal(a.loadAssignment, b.loadAssignment)
	})
}

func buildStaticIr(in *kgateway.StaticBackend) (*StaticIr, error) {
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
		dnsClusterConfig, err := utils.MessageToAny(&envoydnsv3.DnsCluster{})
		if err != nil {
			return nil, err
		}
		ir.clusterTypeConfig = dnsClusterConfig
	}

	return ir, nil
}

// processStatic applies the static IR to the envoy cluster.
func processStatic(ir *StaticIr, out *envoyclusterv3.Cluster) {
	if ir.clusterTypeConfig != nil {
		out.ClusterDiscoveryType = &envoyclusterv3.Cluster_ClusterType{
			ClusterType: &envoyclusterv3.Cluster_CustomClusterType{
				Name:        dnsClusterExtensionName,
				TypedConfig: proto.Clone(ir.clusterTypeConfig).(*anypb.Any),
			},
		}
	} else {
		out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{
			Type: ir.clusterType,
		}
	}

	if ir.loadAssignment != nil {
		// clone needed to avoid adding cluster name to original object in the IR.
		out.LoadAssignment = proto.Clone(ir.loadAssignment).(*envoyendpointv3.ClusterLoadAssignment)
		out.LoadAssignment.ClusterName = out.GetName()
	}
}
