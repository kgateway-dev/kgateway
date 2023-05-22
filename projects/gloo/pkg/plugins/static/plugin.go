package static

import (
	"fmt"
	"net"
	"net/url"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	proxyproto "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/proxy_protocol/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	pbgostruct "github.com/golang/protobuf/ptypes/struct"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	upstream_proxy_protocol "github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils/upstreamproxyprotocol"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/errors"
)

var (
	_ plugins.Plugin         = new(plugin)
	_ plugins.UpstreamPlugin = new(plugin)
)

const (
	// TODO: make solo-projects use this constant
	TransportSocketMatchKey = "envoy.transport_socket_match"

	proxyProtocolUpstreamClusterName = "envoy.extensions.transport_sockets.proxy_protocol.v3.ProxyProtocolUpstreamTransport"
	UpstreamProxySocketName          = "envoy.transport_sockets.upstream_proxy_protocol"

	AdvancedHttpCheckerName = "io.solo.health_checkers.advanced_http"
	PathFieldName           = "path"
	MethodFieldName         = "method"
	ExtensionName           = "static"
)

type plugin struct {
	settings *v1.Settings
}

func NewPlugin() plugins.Plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Resolve(u *v1.Upstream) (*url.URL, error) {
	staticSpec, ok := u.GetUpstreamType().(*v1.Upstream_Static)
	if !ok {
		return nil, nil
	}
	if len(staticSpec.Static.GetHosts()) == 0 {
		return nil, errors.Errorf("must provide at least 1 host in static spec")
	}

	return url.Parse(fmt.Sprintf("tcp://%v:%v", staticSpec.Static.GetHosts()[0].GetAddr(), staticSpec.Static.GetHosts()[0].GetPort()))
}

func (p *plugin) Init(params plugins.InitParams) {
	p.settings = params.Settings
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {
	staticSpec, ok := in.GetUpstreamType().(*v1.Upstream_Static)
	if !ok {
		// not ours
		return nil
	}

	spec := staticSpec.Static
	var foundSslPort bool
	var hostname string

	out.ClusterDiscoveryType = &envoy_config_cluster_v3.Cluster_Type{
		Type: envoy_config_cluster_v3.Cluster_STATIC,
	}
	for _, host := range spec.GetHosts() {
		if host.GetAddr() == "" {
			return errors.Errorf("addr cannot be empty for host")
		}
		if host.GetPort() == 0 {
			return errors.Errorf("port cannot be empty for host")
		}
		if host.GetPort() == 443 {
			foundSslPort = true
		}
		ip := net.ParseIP(host.GetAddr())
		if ip == nil {
			// can't parse ip so this is a dns hostname.
			// save the first hostname for use with sni
			if hostname == "" {
				hostname = host.GetAddr()
			}
		}

		if out.GetLoadAssignment() == nil {
			out.LoadAssignment = &envoy_config_endpoint_v3.ClusterLoadAssignment{
				ClusterName: out.GetName(),
				Endpoints:   []*envoy_config_endpoint_v3.LocalityLbEndpoints{{}},
			}
		}

		healthCheckConfig := &envoy_config_endpoint_v3.Endpoint_HealthCheckConfig{
			Hostname: host.GetAddr(),
		}

		if (in.GetHealthChecks() != nil) &&
			(len(in.GetHealthChecks()) > 0) &&
			(in.GetHealthChecks()[0].GetHttpHealthCheck().GetHost() != "") {

			// The discerning reader may ask the question "Why are we hardcoding this to use the 0th healthcheck?"
			// This was done with two assumptions:
			// 		1) no one _currently_ uses this feature.  It was previously _always_ hardcoded to be a loopback call to host.GetAddr()
			//		2) looking through our field engineering scripts/customer use-cases, _right now_, it appears that customers are using only a single top-level healthcheck
			healthCheckConfig = &envoy_config_endpoint_v3.Endpoint_HealthCheckConfig{
				Hostname: in.GetHealthChecks()[0].GetHttpHealthCheck().GetHost(),
			}
		}

		out.GetLoadAssignment().GetEndpoints()[0].LbEndpoints = append(out.GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints(),
			&envoy_config_endpoint_v3.LbEndpoint{
				Metadata: getMetadata(spec, host),
				HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
					Endpoint: &envoy_config_endpoint_v3.Endpoint{
						Hostname: host.GetAddr(),
						Address: &envoy_config_core_v3.Address{
							Address: &envoy_config_core_v3.Address_SocketAddress{
								SocketAddress: &envoy_config_core_v3.SocketAddress{
									Protocol: envoy_config_core_v3.SocketAddress_TCP,
									Address:  host.GetAddr(),
									PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
										PortValue: host.GetPort(),
									},
								},
							},
						},
						HealthCheckConfig: healthCheckConfig,
					},
				},
				LoadBalancingWeight: host.GetLoadBalancingWeight(),
			})
	}

	// if host port is 443 or if the user wants it, we will use TLS
	if spec.GetUseTls().GetValue() || (spec.GetUseTls() == nil && foundSslPort) {
		// tell envoy to use TLS to connect to this upstream
		// TODO: support client certificates
		if out.GetTransportSocket() == nil {
			commonTlsContext, err := utils.GetCommonTlsContextFromUpstreamOptions(p.settings.GetUpstreamOptions())
			if err != nil {
				return err
			}
			tlsContext := &envoyauth.UpstreamTlsContext{
				CommonTlsContext: commonTlsContext,
				// TODO(yuval-k): Add verification context
				Sni: hostname,
			}
			typedConfig, err := utils.MessageToAny(tlsContext)
			if err != nil {
				return err
			}
			out.TransportSocket = &envoy_config_core_v3.TransportSocket{
				Name:       wellknown.TransportSocketTls,
				ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
			}
		}
	}
	if out.GetTransportSocket() != nil {
		// we may have to operate on a different pointer so lets store this for now
		topTS := out.GetTransportSocket()
		shouldMutate := false
		// we have a transport socket and its not tls which was the former intention here
		// given this we attempt to parse and mutate
		if out.GetTransportSocket().GetName() == upstream_proxy_protocol.UpstreamProxySocketName {
			// strip it we will reset it
			tcMsg, err := utils.AnyToMessage(out.GetTransportSocket().GetTypedConfig())
			if err != nil {
				return err
			}

			typedTransport, ok := tcMsg.(*proxyproto.ProxyProtocolUpstreamTransport)
			if !ok {
				return fmt.Errorf("static upstream cannot convert transport socket: %v", tcMsg)
			}
			topTS = typedTransport.GetTransportSocket()

			cfg, err := utils.AnyToMessage(topTS.GetTypedConfig())
			if err != nil {
				return errors.Wrapf(err, "static upstream cannot convert transport socket: %v", tcMsg)
			}

			_, ok = cfg.(*envoyauth.UpstreamTlsContext)
			shouldMutate = ok

		}
		if shouldMutate {
			for _, host := range spec.GetHosts() {

				sniname := sniAddr(spec, host)
				if sniname == "" {
					continue
				}
				ts, err := mutateSni(topTS, sniname)
				if err != nil {
					return err
				}

				if in.GetProxyProtocolVersion() != nil {
					// reinstate the proxy protocol as we may wipe it out when we mutate the sni
					newTs, err := upstream_proxy_protocol.WrapWithPProtocol(ts, in.GetProxyProtocolVersion().GetValue())
					if err != nil {
						return err
					}
					ts = newTs
				}

				out.TransportSocketMatches = append(out.GetTransportSocketMatches(), &envoy_config_cluster_v3.Cluster_TransportSocketMatch{
					Name:            name(spec, host),
					Match:           metadataMatch(spec, host),
					TransportSocket: ts,
				})
			}
		}
	}

	// the upstream has a DNS name. We need Envoy to resolve the DNS name
	if hostname != "" {
		// set the type to strict dns
		out.ClusterDiscoveryType = &envoy_config_cluster_v3.Cluster_Type{
			Type: envoy_config_cluster_v3.Cluster_STRICT_DNS,
		}

		// fix issue where ipv6 addr cannot bind
		out.DnsLookupFamily = envoy_config_cluster_v3.Cluster_V4_ONLY
	}

	return nil
}

func mutateSni(in *envoy_config_core_v3.TransportSocket, sni string) (*envoy_config_core_v3.TransportSocket, error) {
	copy := *in

	// copy the sni
	cfg, err := utils.AnyToMessage(copy.GetTypedConfig())
	if err != nil {
		return nil, err
	}

	typedCfg, ok := cfg.(*envoyauth.UpstreamTlsContext)
	if !ok {
		return nil, errors.Errorf("unknown tls config type: %T", cfg)
	}
	typedCfg.Sni = sni
	typedConfig, err := utils.MessageToAny(typedCfg)
	if err != nil {
		return nil, err
	}
	copy.ConfigType = &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig}

	return &copy, nil
}

func sniAddr(spec *v1static.UpstreamSpec, in *v1static.Host) string {
	if in.GetSniAddr() != "" {
		return in.GetSniAddr()
	}
	if spec.GetAutoSniRewrite() == nil || spec.GetAutoSniRewrite().GetValue() {
		return in.GetAddr()
	}
	return ""
}

func getMetadata(spec *v1static.UpstreamSpec, in *v1static.Host) *envoy_config_core_v3.Metadata {
	if in == nil {
		return nil
	}
	var meta *envoy_config_core_v3.Metadata
	sniaddr := sniAddr(spec, in)
	if sniaddr != "" {
		if meta == nil {
			meta = &envoy_config_core_v3.Metadata{FilterMetadata: map[string]*pbgostruct.Struct{}}
		}
		meta.GetFilterMetadata()[TransportSocketMatchKey] = metadataMatch(spec, in)
	}

	if path := in.GetHealthCheckConfig().GetPath(); path != "" {
		setMetadataField(meta, PathFieldName, path)
	}
	if method := in.GetHealthCheckConfig().GetMethod(); method != "" {
		setMetadataField(meta, MethodFieldName, method)
	}
	return meta
}

func setMetadataField(meta *envoy_config_core_v3.Metadata, fieldKey, value string) {
	if meta == nil {
		meta = &envoy_config_core_v3.Metadata{FilterMetadata: map[string]*pbgostruct.Struct{}}
	}
	if meta.GetFilterMetadata()[AdvancedHttpCheckerName] == nil {
		meta.GetFilterMetadata()[AdvancedHttpCheckerName] = &pbgostruct.Struct{}
	}
	if meta.GetFilterMetadata()[AdvancedHttpCheckerName].GetFields() == nil {
		meta.GetFilterMetadata()[AdvancedHttpCheckerName].Fields = map[string]*pbgostruct.Value{}
	}

	meta.GetFilterMetadata()[AdvancedHttpCheckerName].GetFields()[fieldKey] = &pbgostruct.Value{
		Kind: &pbgostruct.Value_StringValue{
			StringValue: value,
		},
	}
}

func name(spec *v1static.UpstreamSpec, in *v1static.Host) string {
	return fmt.Sprintf("%s;%s:%d", sniAddr(spec, in), in.GetAddr(), in.GetPort())
}

func metadataMatch(spec *v1static.UpstreamSpec, in *v1static.Host) *pbgostruct.Struct {
	return &pbgostruct.Struct{
		Fields: map[string]*pbgostruct.Value{
			name(spec, in): {
				Kind: &pbgostruct.Value_BoolValue{
					BoolValue: true,
				},
			},
		},
	}
}
