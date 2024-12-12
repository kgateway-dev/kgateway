package istio

import (
	"context"
	"fmt"
	"strconv"

	structpb "github.com/golang/protobuf/ptypes/struct"
	"google.golang.org/protobuf/types/known/anypb"
	"k8s.io/apimachinery/pkg/runtime/schema"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	sockets_raw_buffer "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/raw_buffer/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/solo-io/gloo/projects/gateway2/extensions2/common"
	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gloo/constants"
	"github.com/solo-io/go-utils/contextutils"
	corev1 "k8s.io/api/core/v1"
)

var (
	VirtualIstioGK = schema.GroupKind{
		Group: "istioplugin",
		Kind:  "istioplugin",
	}
)

func NewPlugin(ctx context.Context, commoncol common.CommonCollections) extensionsplug.Plugin {
	p := plugin{
		enabledIstioIntegration: true,
		enabledAutoMTLS:         true,
	}
	return extensionsplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			VirtualIstioGK: {
				Name:            "istio",
				ProcessUpstream: p.processUpstream,
				GlobalPolicies: func(attachmentPoints extensionsplug.AttachmentPoints) ir.PolicyIR {
					return struct{}{}
				},
			},
		},
	}
}

type plugin struct {
	enabledIstioIntegration     bool
	enabledAutoMTLS             bool
	enableIstioSidecarOnGateway bool
}

func isDisabledForUpstream(upstream ir.Upstream) bool {
	// return in.GetDisableIstioAutoMtls().GetValue()

	// TODO: implement this; we can do it by checking annotations?
	return false
}

// we don't have a good way of know if we have ssl on the upstream, so check cluster instead
func doesClusterHaveSslConfigPresent(out *envoy_config_cluster_v3.Cluster) bool {
	// TODO: implement this
	return false
}

func (p plugin) processUpstream(ctx context.Context, _ ir.PolicyIR, in ir.Upstream, out *envoy_config_cluster_v3.Cluster) {
	var socketmatches []*envoy_config_cluster_v3.Cluster_TransportSocketMatch

	// Istio automtls will only be applied when:
	// 1) automtls is enabled on the settings
	// 2) the upstream has not disabled auto mtls
	// 3) the upstream has no sslConfig
	//if p.settings.GetGloo().GetIstioOptions().GetEnableAutoMtls().GetValue() && !in.GetDisableIstioAutoMtls().GetValue() && sslConfig == nil {
	if p.enabledAutoMTLS && !isDisabledForUpstream(in) && !doesClusterHaveSslConfigPresent(out) {
		// Istio automtls config is not applied if istio integration is disabled on the helm chart.
		// When istio integration is disabled via istioSds.enabled=false, there is no sds or istio-proxy sidecar present
		if !p.enabledIstioIntegration {
			contextutils.LoggerFrom(ctx).Desugar().Error("Istio integration must be enabled to use auto mTLS. Enable integration with istioIntegration.enabled=true")
		} else {
			// Note: If enableIstioSidecarOnGateway is enabled, Istio automtls will not be able to generate the endpoint
			// metadata from the Pod to match the transport socket match. We will still translate the transport socket match
			// configuration. enableIstioSidecarOnGateway should be removed as part of: https://github.com/solo-io/solo-projects/issues/5743
			if p.enableIstioSidecarOnGateway {
				contextutils.LoggerFrom(ctx).Desugar().Warn("Istio sidecar injection (istioIntegration.enableIstioSidecarOnGateway) should be disabled for Istio automtls mode")
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
}

func createIstioMatch(sni string) *envoy_config_cluster_v3.Cluster_TransportSocketMatch {
	istioMtlsTransportSocketMatch := &structpb.Struct{
		Fields: map[string]*structpb.Value{
			constants.TLSModeLabelShortname: {Kind: &structpb.Value_StringValue{StringValue: constants.IstioMutualTLSModeLabel}},
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

	typedConfig, _ := anypb.New(sslSds)
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
	typedConfig, _ := anypb.New(&sockets_raw_buffer.RawBuffer{})
	rawBufferTransportSocket := &envoy_config_core_v3.TransportSocket{
		Name:       wellknown.TransportSocketRawBuffer,
		ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
	}

	return &envoy_config_cluster_v3.Cluster_TransportSocketMatch{
		Name:            fmt.Sprintf("%s-disabled", constants.TLSModeLabelShortname),
		Match:           &structpb.Struct{},
		TransportSocket: rawBufferTransportSocket,
	}
}

func buildSni(upstream ir.Upstream) string {

	switch us := upstream.Obj.(type) {
	case *corev1.Service:
		return buildDNSSrvSubsetKey(
			svcFQDN(
				us.Name,
				us.Namespace,
				"cluster.local", // TODO we need a setting like Istio has for trustDomain
			),
			uint32(upstream.Port),
		)
	default:
		if upstream.Port != 0 && upstream.CanonicalHostname != "" {
			return buildDNSSrvSubsetKey(
				upstream.CanonicalHostname,
				uint32(upstream.Port),
			)
		}
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
