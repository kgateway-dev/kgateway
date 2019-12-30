package static

import (
	"net"

	"fmt"
	"net/url"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyendpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/errors"
)

type plugin struct{}

func NewPlugin() plugins.Plugin {
	return &plugin{}
}

func (p *plugin) Resolve(u *v1.Upstream) (*url.URL, error) {
	staticSpec, ok := u.UpstreamType.(*v1.Upstream_Static)
	if !ok {
		return nil, nil
	}
	if len(staticSpec.Static.Hosts) == 0 {
		return nil, errors.Errorf("must provide at least 1 host in static spec")
	}

	return url.Parse(fmt.Sprintf("tcp://%v:%v", staticSpec.Static.Hosts[0].Addr, staticSpec.Static.Hosts[0].Port))
}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {
	staticSpec, ok := in.UpstreamType.(*v1.Upstream_Static)
	if !ok {
		// not ours
		return nil
	}

	spec := staticSpec.Static
	var foundSslPort bool
	var hostname string

	out.ClusterDiscoveryType = &envoyapi.Cluster_Type{
		Type: envoyapi.Cluster_STATIC,
	}
	for _, host := range spec.Hosts {
		if host.Addr == "" {
			return errors.Errorf("addr cannot be empty for host")
		}
		if host.Port == 0 {
			return errors.Errorf("port cannot be empty for host")
		}
		if host.Port == 443 {
			foundSslPort = true
		}
		ip := net.ParseIP(host.Addr)
		if ip == nil {
			// can't parse ip so this is a dns hostname.
			// save the first hostname for use with sni
			if hostname == "" {
				hostname = host.Addr
			}
		}

		if out.LoadAssignment == nil {
			out.LoadAssignment = &envoyapi.ClusterLoadAssignment{
				ClusterName: out.Name,
				Endpoints:   []*envoyendpoint.LocalityLbEndpoints{{}},
			}
		}

		out.LoadAssignment.Endpoints[0].LbEndpoints = append(out.LoadAssignment.Endpoints[0].LbEndpoints,
			&envoyendpoint.LbEndpoint{
				HostIdentifier: &envoyendpoint.LbEndpoint_Endpoint{
					Endpoint: &envoyendpoint.Endpoint{
						Address: &envoycore.Address{
							Address: &envoycore.Address_SocketAddress{
								SocketAddress: &envoycore.SocketAddress{
									Protocol: envoycore.SocketAddress_TCP,
									Address:  host.Addr,
									PortSpecifier: &envoycore.SocketAddress_PortValue{
										PortValue: host.Port,
									},
								},
							},
						},
					},
				},
			})
	}

	// if host port is 443 or if the user wants it, we will use TLS
	if spec.UseTls || foundSslPort {
		// tell envoy to use TLS to connect to this upstream
		// TODO: support client certificates
		if out.TlsContext == nil {
			out.TlsContext = &envoyauth.UpstreamTlsContext{
				Sni: hostname,
			}
		}
	}

	// the upstream has a DNS name. We need Envoy to resolve the DNS name
	if hostname != "" {
		// set the type to strict dns
		out.ClusterDiscoveryType = &envoyapi.Cluster_Type{
			Type: envoyapi.Cluster_STRICT_DNS,
		}

		// fix issue where ipv6 addr cannot bind
		out.DnsLookupFamily = envoyapi.Cluster_V4_ONLY
	}

	return nil
}
