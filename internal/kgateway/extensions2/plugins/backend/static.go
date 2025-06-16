package backend

import (
	"fmt"
	"net/netip"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

func processStatic(in *v1alpha1.StaticBackend, out *envoy_config_cluster_v3.Cluster) error {
	var hostname string
	out.ClusterDiscoveryType = &envoy_config_cluster_v3.Cluster_Type{
		Type: envoy_config_cluster_v3.Cluster_STATIC,
	}
	for _, host := range in.Hosts {
		if host.Host == "" {
			return fmt.Errorf("addr cannot be empty for host")
		}
		if host.Port == 0 {
			return fmt.Errorf("port cannot be empty for host")
		}

		_, err := netip.ParseAddr(host.Host)
		if err != nil {
			// can't parse ip so this is a dns hostname.
			// save the first hostname for use with sni
			if hostname == "" {
				hostname = host.Host
			}
		}

		if out.GetLoadAssignment() == nil {
			out.LoadAssignment = &envoy_config_endpoint_v3.ClusterLoadAssignment{
				ClusterName: out.GetName(),
				Endpoints:   []*envoy_config_endpoint_v3.LocalityLbEndpoints{{}},
			}
		}

		healthCheckConfig := &envoy_config_endpoint_v3.Endpoint_HealthCheckConfig{
			Hostname: host.Host,
		}

		out.GetLoadAssignment().GetEndpoints()[0].LbEndpoints = append(out.GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints(),
			&envoy_config_endpoint_v3.LbEndpoint{
				//	Metadata: getMetadata(params.Ctx, spec, host),
				HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
					Endpoint: &envoy_config_endpoint_v3.Endpoint{
						Hostname: host.Host,
						Address: &envoy_config_core_v3.Address{
							Address: &envoy_config_core_v3.Address_SocketAddress{
								SocketAddress: &envoy_config_core_v3.SocketAddress{
									Protocol: envoy_config_core_v3.SocketAddress_TCP,
									Address:  host.Host,
									PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
										PortValue: uint32(host.Port),
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
		out.ClusterDiscoveryType = &envoy_config_cluster_v3.Cluster_Type{
			Type: envoy_config_cluster_v3.Cluster_STRICT_DNS,
		}

		// do we still need this?
		//		// fix issue where ipv6 addr cannot bind
		//		out.DnsLookupFamily = envoy_config_cluster_v3.Cluster_V4_ONLY
	}
	return nil
}

func processEndpointsStatic(_ *v1alpha1.StaticBackend) *ir.EndpointsForBackend {
	return nil
}
