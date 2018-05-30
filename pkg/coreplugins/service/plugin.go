package service

import (
	"net"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins"
)

type Plugin struct {
	sslUpstreams map[string]bool
}

func NewPlugin() *Plugin {
	return &Plugin{
		sslUpstreams: make(map[string]bool),
	}
}

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
	var hostname string
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
			// for sni
			hostname = host.Addr
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
		if (spec.TLS != nil && *spec.TLS) || (spec.TLS == nil && foundSslPort) {
			// cache the name of this upstream, we need to enable automatic host rewrite on routes
			p.sslUpstreams[in.Name] = true

			// tell envoy to use TLS to connect to this upstream
			// TODO: support client certificates
			out.TlsContext = &envoyauth.UpstreamTlsContext{
				Sni: hostname,
			}
		}
	}
	return nil
}

// need to enable automatic host rewrite on routes to SSL upstreams
func (p *Plugin) ProcessRoute(_ *plugins.RoutePluginParams, in *v1.Route, out *envoyroute.Route) error {
	upstreamNames := destinationUpstreams(in)
	for _, usName := range upstreamNames {
		if _, ok := p.sslUpstreams[usName]; !ok {
			continue
		}
		// this is a route to one of our ssl upstreams
		// enable auto host rewrite
		out.Action.(*envoyroute.Route_Route).Route.HostRewriteSpecifier = &envoyroute.RouteAction_AutoHostRewrite{
			AutoHostRewrite: &types.BoolValue{
				Value: true,
			},
		}
		// one is good enough
		break
	}
	return nil
}

func destinationUpstreams(route *v1.Route) []string {
	switch {
	case route.SingleDestination != nil:
		return []string{destinationUpstream(route.SingleDestination)}
	case route.MultipleDestinations != nil:
		var destinationUpstreams []string
		for _, dest := range route.MultipleDestinations {
			destinationUpstreams = append(destinationUpstreams, destinationUpstream(dest.Destination))
		}
		return destinationUpstreams
	}
	panic("invalid route")
}

func destinationUpstream(dest *v1.Destination) string {
	switch dest := dest.DestinationType.(type) {
	case *v1.Destination_Upstream:
		return dest.Upstream.Name
	case *v1.Destination_Function:
		return dest.Function.UpstreamName
	}
	panic("invalid destination")
}

// just use HttpFilters to clear cache
func (p *Plugin) HttpFilters(params *plugins.FilterPluginParams) []plugins.StagedFilter {
	p.sslUpstreams = make(map[string]bool)
	return nil
}
