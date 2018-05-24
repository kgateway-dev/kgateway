package service

import (
	"net"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins"
)

type Plugin struct{}

const (
	// define Upstream type name
	UpstreamTypeService = "service"
)

func (p *Plugin) GetDependencies(_ *v1.Config) *plugins.Dependencies {
	return nil
}

func (p *Plugin) ProcessUpstream(_ *plugins.UpstreamPluginParams, in *v1.Upstream, out *envoyapi.Cluster) error {
	if in.Type != UpstreamTypeService {
		return nil
	}
	// decode does validation for us
	spec, err := DecodeUpstreamSpec(in.Spec)
	if err != nil {
		return errors.Wrap(err, "invalid service upstream spec")
	}
	var foundSslPort bool
	var addr string
	for _, host := range spec.Hosts {
		if host.Addr == "" {
			return errors.New("addr cannot be empty for host")
		}
		if host.Port == 0 {
			return errors.New("port cannot be empty for host")
		}
		if host.Port == 443 {
			foundSslPort = true
		}
		ip := net.ParseIP(host.Addr)
		if ip != nil {
			out.Type = envoyapi.Cluster_STATIC
		} else {
			out.Type = envoyapi.Cluster_STRICT_DNS
		}
		out.Hosts = append(out.Hosts, &envoycore.Address{
			Address: &envoycore.Address_SocketAddress{
				SocketAddress: &envoycore.SocketAddress{
					Protocol: envoycore.TCP,
					Address:  host.Addr,
					PortSpecifier: &envoycore.SocketAddress_PortValue{
						PortValue: host.Port,
					},
				},
			},
		})
		// fix issue where ipv6 addr cannot bind
		if !spec.EnableIPv6 {
			out.DnsLookupFamily = envoyapi.Cluster_V4_ONLY
		}
		// if host port is 443 && spec.TLS == nil we will use TLS
		// or if the user wants it
		if (spec.TLS != nil && *spec.TLS) || foundSslPort {
			out.TlsContext = &envoyauth.UpstreamTlsContext{
				Sni: hostname,
			}
		}
	}
	return nil
}
