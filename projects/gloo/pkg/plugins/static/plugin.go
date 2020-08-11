package static

import (
	"net"

	pbgostruct "github.com/golang/protobuf/ptypes/struct"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	"github.com/envoyproxy/go-control-plane/pkg/wellknown"

	"fmt"
	"net/url"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyendpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/errors"
)

const (
	HttpPathCheckerName = "io.solo.health_checkers.http_path"
	PathFieldName       = "path"
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
				Metadata: getMetadata(host),
				HostIdentifier: &envoyendpoint.LbEndpoint_Endpoint{
					Endpoint: &envoyendpoint.Endpoint{
						Hostname: host.Addr,
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
						HealthCheckConfig: &envoyendpoint.Endpoint_HealthCheckConfig{
							Hostname: host.Addr,
						},
					},
				},
			})
	}

	// if host port is 443 or if the user wants it, we will use TLS
	if spec.UseTls || foundSslPort {
		// tell envoy to use TLS to connect to this upstream
		// TODO: support client certificates
		if out.TransportSocket == nil {
			tlsContext := &envoyauth.UpstreamTlsContext{
				// TODO(yuval-k): Add verification context
				Sni: hostname,
			}
			out.TransportSocket = &envoycore.TransportSocket{
				Name:       wellknown.TransportSocketTls,
				ConfigType: &envoycore.TransportSocket_TypedConfig{TypedConfig: utils.MustMessageToAny(tlsContext)},
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

func getMetadata(in *v1static.Host) *envoycore.Metadata {
	if in == nil {
		return nil
	}
	if in.GetHealthCheckConfig().GetPath() != "" {
		return &envoycore.Metadata{
			FilterMetadata: map[string]*pbgostruct.Struct{
				HttpPathCheckerName: {
					Fields: map[string]*pbgostruct.Value{
						PathFieldName: {
							Kind: &pbgostruct.Value_StringValue{
								StringValue: in.GetHealthCheckConfig().GetPath(),
							},
						},
					},
				},
			},
		}
	}
	return nil
}
