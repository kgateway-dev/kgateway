package static

import (
	"fmt"
	"net"
	"net/url"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	pbgostruct "github.com/golang/protobuf/ptypes/struct"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/errors"
)

const (
	// TODO: make solo-projects use this constant
	TransportSocketMatchKey = "envoy.transport_socket_match"

	AdvancedHttpCheckerName = "io.solo.health_checkers.advanced_http"
	PathFieldName           = "path"
	MethodFieldName         = "method"
)

var _ plugins.Plugin = new(plugin)
var _ plugins.UpstreamPlugin = new(plugin)

type plugin struct {
	settings *v1.Settings
}

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
	p.settings = params.Settings
	return nil
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {
	staticSpec, ok := in.UpstreamType.(*v1.Upstream_Static)
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
			out.LoadAssignment = &envoy_config_endpoint_v3.ClusterLoadAssignment{
				ClusterName: out.Name,
				Endpoints:   []*envoy_config_endpoint_v3.LocalityLbEndpoints{{}},
			}
		}

		out.LoadAssignment.Endpoints[0].LbEndpoints = append(out.LoadAssignment.Endpoints[0].LbEndpoints,
			&envoy_config_endpoint_v3.LbEndpoint{
				Metadata: getMetadata(spec, host),
				HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
					Endpoint: &envoy_config_endpoint_v3.Endpoint{
						Hostname: host.Addr,
						Address: &envoy_config_core_v3.Address{
							Address: &envoy_config_core_v3.Address_SocketAddress{
								SocketAddress: &envoy_config_core_v3.SocketAddress{
									Protocol: envoy_config_core_v3.SocketAddress_TCP,
									Address:  host.Addr,
									PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
										PortValue: host.Port,
									},
								},
							},
						},
						HealthCheckConfig: &envoy_config_endpoint_v3.Endpoint_HealthCheckConfig{
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
			commonTlsContext, err := utils.GetCommonTlsContextFromUpstreamOptions(p.settings.GetUpstreamOptions())
			if err != nil {
				return err
			}
			tlsContext := &envoyauth.UpstreamTlsContext{
				CommonTlsContext: commonTlsContext,
				// TODO(yuval-k): Add verification context
				Sni: hostname,
			}
			out.TransportSocket = &envoy_config_core_v3.TransportSocket{
				Name:       wellknown.TransportSocketTls,
				ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: utils.MustMessageToAny(tlsContext)},
			}
		}
	}
	if out.TransportSocket != nil {
		for _, host := range spec.Hosts {
			sniname := sniAddr(spec, host)
			if sniname == "" {
				continue
			}
			ts, err := mutateSni(out.TransportSocket, sniname)
			if err != nil {
				return err
			}
			out.TransportSocketMatches = append(out.TransportSocketMatches, &envoy_config_cluster_v3.Cluster_TransportSocketMatch{
				Name:            name(spec, host),
				Match:           metadataMatch(spec, host),
				TransportSocket: ts,
			})
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

	copy.ConfigType = &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: utils.MustMessageToAny(typedCfg)}

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
		meta.FilterMetadata[TransportSocketMatchKey] = metadataMatch(spec, in)
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
	if meta.FilterMetadata[AdvancedHttpCheckerName] == nil {
		meta.FilterMetadata[AdvancedHttpCheckerName] = &pbgostruct.Struct{}
	}
	if meta.FilterMetadata[AdvancedHttpCheckerName].Fields == nil {
		meta.FilterMetadata[AdvancedHttpCheckerName].Fields = map[string]*pbgostruct.Value{}
	}

	meta.FilterMetadata[AdvancedHttpCheckerName].Fields[fieldKey] = &pbgostruct.Value{
		Kind: &pbgostruct.Value_StringValue{
			StringValue: value,
		},
	}
}

func name(spec *v1static.UpstreamSpec, in *v1static.Host) string {
	return fmt.Sprintf("%s;%s:%d", sniAddr(spec, in), in.Addr, in.Port)
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
