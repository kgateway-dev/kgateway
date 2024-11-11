package istio_automtls

import (
	"fmt"
	"strconv"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	socketsRaw "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/raw_buffer/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	_struct "github.com/golang/protobuf/ptypes/struct"
	"github.com/solo-io/gloo/projects/gloo/constants"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
)

var (
	_ plugins.Plugin         = new(plugin)
	_ plugins.UpstreamPlugin = new(plugin)
)

const (
	ExtensionName = "istio_automtls"
)

type plugin struct {
	settings *v1.Settings

	// Note: When enableIstioSidecarOnGateway is enabled, eds will not add the istio pod labels to the endpoint and
	// automtls will not generate the endpoint metadata to match the transport socket.
	enableIstioSidecarOnGateway bool
}

func NewPlugin(enableIstioSidecarOnGateway bool) *plugin {
	return &plugin{
		enableIstioSidecarOnGateway: enableIstioSidecarOnGateway,
	}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {
	p.settings = params.Settings
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {
	var socketmatches []*envoy_config_cluster_v3.Cluster_TransportSocketMatch

	sslConfig := in.GetSslConfig()
	// Istio automtls will only be applied when:
	// 1) automtls is enabled on the settings
	// 2) the upstream has not disabled auto mtls
	// 3) the upstream has no sslConfig
	if p.settings.GetGloo().GetIstioOptions().GetEnableAutoMtls().GetValue() && !in.GetDisableIstioAutoMtls().GetValue() && sslConfig == nil {
		// Istio automtls config is not applied if istio integration is disabled on the helm chart.
		// When istio integration is disabled via istioSds.enabled=false, there is no sds or istio-proxy sidecar present
		if !p.settings.GetGloo().GetIstioOptions().GetEnableIntegration().GetValue() {
			contextutils.LoggerFrom(params.Ctx).Error("Istio integration must be enabled to use auto mTLS. Enable integration with istioIntegration.enabled=true")
		} else {
			// Note: If enableIstioSidecarOnGateway is enabled, Istio automtls will not be able to generate the endpoint
			// metadata from the Pod to match the transport socket match. We will still translate the transport socket match
			// configuration. enableIstioSidecarOnGateway should be removed as part of: https://github.com/solo-io/solo-projects/issues/5743
			if p.enableIstioSidecarOnGateway {
				contextutils.LoggerFrom(params.Ctx).Warn("Istio sidecar injection (istioIntegration.enableIstioSidecarOnGateway) should be disabled for Istio automtls mode")
			}

			sni := buildSni(in)

			socketmatches = []*envoy_config_cluster_v3.Cluster_TransportSocketMatch{
				// add istio mtls match
				createIstioMatch(sni),
				// plaintext match. Note: this needs to come after the tlsMode-istio match
				createDefaultIstioMatch(),
			}
		}
		out.TransportSocketMatches = socketmatches
	}
	return nil
}

func createIstioMatch(sni string) *envoy_config_cluster_v3.Cluster_TransportSocketMatch {
	istioMtlsTransportSocketMatch := &_struct.Struct{
		Fields: map[string]*_struct.Value{
			constants.TLSModeLabelShortname: {Kind: &_struct.Value_StringValue{StringValue: constants.IstioMutualTLSModeLabel}},
		},
	}

	sslSds := &tlsv3.UpstreamTlsContext{
		Sni: sni,
		CommonTlsContext: &tlsv3.CommonTlsContext{
			AlpnProtocols: []string{"istio"},
			TlsParams:     &tlsv3.TlsParameters{},
			ValidationContextType: &tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
				ValidationContextSdsSecretConfig: &tlsv3.SdsSecretConfig{
					Name: constants.IstioValidationContext,
					SdsConfig: &envoy_config_core_v3.ConfigSource{
						ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
						ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
							ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
								// Istio sets this to skip the node identifier in later discovery requests
								SetNodeOnFirstMessageOnly: true,
								ApiType:                   envoy_config_core_v3.ApiConfigSource_GRPC,
								TransportApiVersion:       envoy_config_core_v3.ApiVersion_V3,
								GrpcServices: []*envoy_config_core_v3.GrpcService{
									{
										TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{ClusterName: constants.SdsClusterName},
										},
									},
								},
							},
						},
					},
				},
			},
			TlsCertificateSdsSecretConfigs: []*tlsv3.SdsSecretConfig{
				{
					Name: constants.IstioCertSecret,
					SdsConfig: &envoy_config_core_v3.ConfigSource{
						ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
						ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
							ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
								ApiType: envoy_config_core_v3.ApiConfigSource_GRPC,
								// Istio sets this to skip the node identifier in later discovery requests
								SetNodeOnFirstMessageOnly: true,
								TransportApiVersion:       envoy_config_core_v3.ApiVersion_V3,
								GrpcServices: []*envoy_config_core_v3.GrpcService{
									{
										TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
												ClusterName: constants.SdsClusterName,
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

	typedConfig, _ := utils.MessageToAny(sslSds)
	transportSocket := &envoy_config_core_v3.TransportSocket{
		Name:       wellknown.TransportSocketTls,
		ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
	}

	return &envoy_config_cluster_v3.Cluster_TransportSocketMatch{
		Name:            fmt.Sprintf("%s-%s", constants.TLSModeLabelShortname, constants.IstioMutualTLSModeLabel),
		Match:           istioMtlsTransportSocketMatch,
		TransportSocket: transportSocket,
	}
}

func createDefaultIstioMatch() *envoy_config_cluster_v3.Cluster_TransportSocketMatch {
	// Based on Istio's default match https://github.com/istio/istio/blob/fa321ebd2a1186325788b0f461aa9f36a1a8d90e/pilot/pkg/xds/filters/filters.go#L78
	typedConfig, _ := utils.MessageToAny(&socketsRaw.RawBuffer{})
	rawBufferTransportSocket := &envoy_config_core_v3.TransportSocket{
		Name:       wellknown.TransportSocketRawBuffer,
		ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
	}

	return &envoy_config_cluster_v3.Cluster_TransportSocketMatch{
		Name:            fmt.Sprintf("%s-disabled", constants.TLSModeLabelShortname),
		Match:           &_struct.Struct{},
		TransportSocket: rawBufferTransportSocket,
	}
}

func buildSni(us *v1.Upstream) string {
	if us.GetUpstreamType() == nil {
		return ""
	}
	switch us := us.GetUpstreamType().(type) {
	case *v1.Upstream_Kube:
		return buildDNSSrvSubsetKey(
			svcFQDN(
				us.Kube.GetServiceName(),
				us.Kube.GetServiceNamespace(),
				"cluster.local", // TODO we need a setting like Istio has for trustDomain
			),
			us.Kube.GetServicePort(),
		)
	case *v1.Upstream_Static:
		if len(us.Static.GetHosts()) > 0 {
			// static upstreams use the first host
			host := us.Static.GetHosts()[0]

			// if SNI address is set, use it directly
			if host.GetSniAddr() != "" {
				return host.GetSniAddr()
			}

			// otherwise build istio DNSSrv style
			return buildDNSSrvSubsetKey(
				host.GetAddr(),
				host.GetPort(),
			)
		}
	default:
	}
	return ""
}

// buildDNSSrvSubsetKey mirrors a similarly named function in Istio.
// Istio auto-passthrough gateways expect this value for the SNI.
// We also expect gloo mesh to tell Istio to match the virtual destination SNI
// but route to the backing Service's cluster via EnvoyFilter.
func buildDNSSrvSubsetKey(hostname string, port uint32) string {
	return "outbound" + "_." + strconv.Itoa(int(port)) + "_._." + string(hostname)
}

func svcFQDN(name, ns, trustDomain string) string {
	return fmt.Sprintf("%s.%s.svc.%s", name, ns, trustDomain)
}
