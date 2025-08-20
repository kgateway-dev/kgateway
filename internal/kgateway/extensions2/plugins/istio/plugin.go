package istio

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	sockets_raw_buffer "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/raw_buffer/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	ourwellknown "github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

var VirtualIstioGK = schema.GroupKind{
	Group: "istioplugin",
	Kind:  "istioplugin",
}

type IstioSettings struct {
	EnableAutoMtls bool
}

func (i IstioSettings) ResourceName() string {
	return "istio-settings"
}

// in case multiple policies attached to the same resource, we sort by policy creation time.
func (i IstioSettings) CreationTime() time.Time {
	// settings always created at the same time
	return time.Time{}
}

func (i IstioSettings) Equals(in any) bool {
	s, ok := in.(IstioSettings)
	if !ok {
		return false
	}
	return i == s
}

var _ ir.PolicyIR = &IstioSettings{}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
	p := istioPlugin{}

	// TODO: if plumb settings from gw class; then they should be in the new translation pass
	// the problem is that they get applied to an upstream, and currently we don't have access to the gateway
	// when translating upstreams. if we want we can add the gateway to the context of PerClientProcessUpstream
	istioSettings := IstioSettings{
		EnableAutoMtls: commoncol.Settings.EnableIstioAutoMtls,
	}

	return extensionsplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			VirtualIstioGK: {
				Name:           "istio",
				ProcessBackend: p.processBackend,
				GlobalPolicies: func(_ krt.HandlerContext) ir.PolicyIR {
					// return static settings which do not change post istioPlugin creation
					return istioSettings
				},
			},
		},
	}
}

type istioPlugin struct{}

func isDisabledForUpstream(_ ir.BackendObjectIR) bool {
	// return in.GetDisableIstioAutoMtls().GetValue()

	// TODO: implement this; we can do it by checking annotations?
	return false
}

// we don't have a good way of know if we have ssl on the upstream, so check cluster instead
// this could be a problem if the policy that adds ssl runs after this one.
// so we need to think about how's best to handle this.
func doesClusterHaveSslConfigPresent(_ *envoyclusterv3.Cluster) bool {
	// TODO: implement this
	return false
}

func (p istioPlugin) processBackend(ctx context.Context, ir ir.PolicyIR, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
	var socketmatches []*envoyclusterv3.Cluster_TransportSocketMatch

	st, ok := ir.(IstioSettings)
	if !ok {
		return
	}
	// Istio automtls will only be applied when:
	// 1) automtls is enabled on the settings
	// 2) the upstream has not disabled auto mtls
	// 3) the upstream has no sslConfig
	if st.EnableAutoMtls && !isDisabledForUpstream(in) && !doesClusterHaveSslConfigPresent(out) {
		sni := buildSni(in)

		socketmatches = []*envoyclusterv3.Cluster_TransportSocketMatch{
			// add istio mtls match
			createIstioMatch(sni),
			// plaintext match. Note: this needs to come after the tlsMode-istio match
			createDefaultIstioMatch(),
		}
		// append the transport socket matches for the Istio integration the cluster
		out.TransportSocketMatches = append(out.GetTransportSocketMatches(), socketmatches...)
	}
}

func createIstioMatch(sni string) *envoyclusterv3.Cluster_TransportSocketMatch {
	istioMtlsTransportSocketMatch := &structpb.Struct{
		Fields: map[string]*structpb.Value{
			ourwellknown.TLSModeLabelShortname: {Kind: &structpb.Value_StringValue{StringValue: ourwellknown.IstioMutualTLSModeLabel}},
		},
	}

	sslSds := &envoytlsv3.UpstreamTlsContext{
		Sni: sni,
		CommonTlsContext: &envoytlsv3.CommonTlsContext{
			AlpnProtocols: []string{"istio"},
			TlsParams:     &envoytlsv3.TlsParameters{},
			ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
				ValidationContextSdsSecretConfig: &envoytlsv3.SdsSecretConfig{
					Name: ourwellknown.IstioValidationContext,
					SdsConfig: &envoycorev3.ConfigSource{
						ResourceApiVersion: envoycorev3.ApiVersion_V3,
						ConfigSourceSpecifier: &envoycorev3.ConfigSource_ApiConfigSource{
							ApiConfigSource: &envoycorev3.ApiConfigSource{
								// Istio sets this to skip the node identifier in later discovery requests
								SetNodeOnFirstMessageOnly: true,
								ApiType:                   envoycorev3.ApiConfigSource_GRPC,
								TransportApiVersion:       envoycorev3.ApiVersion_V3,
								GrpcServices: []*envoycorev3.GrpcService{
									{
										TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{ClusterName: ourwellknown.SdsClusterName},
										},
									},
								},
							},
						},
					},
				},
			},
			TlsCertificateSdsSecretConfigs: []*envoytlsv3.SdsSecretConfig{
				{
					Name: ourwellknown.IstioCertSecret,
					SdsConfig: &envoycorev3.ConfigSource{
						ResourceApiVersion: envoycorev3.ApiVersion_V3,
						ConfigSourceSpecifier: &envoycorev3.ConfigSource_ApiConfigSource{
							ApiConfigSource: &envoycorev3.ApiConfigSource{
								ApiType: envoycorev3.ApiConfigSource_GRPC,
								// Istio sets this to skip the node identifier in later discovery requests
								SetNodeOnFirstMessageOnly: true,
								TransportApiVersion:       envoycorev3.ApiVersion_V3,
								GrpcServices: []*envoycorev3.GrpcService{
									{
										TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
												ClusterName: ourwellknown.SdsClusterName,
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
	transportSocket := &envoycorev3.TransportSocket{
		Name:       wellknown.TransportSocketTls,
		ConfigType: &envoycorev3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
	}

	return &envoyclusterv3.Cluster_TransportSocketMatch{
		Name:            fmt.Sprintf("%s-%s", ourwellknown.TLSModeLabelShortname, ourwellknown.IstioMutualTLSModeLabel),
		Match:           istioMtlsTransportSocketMatch,
		TransportSocket: transportSocket,
	}
}

func createDefaultIstioMatch() *envoyclusterv3.Cluster_TransportSocketMatch {
	// Based on Istio's default match https://github.com/istio/istio/blob/fa321ebd2a1186325788b0f461aa9f36a1a8d90e/pilot/pkg/xds/filters/filters.go#L78
	typedConfig, _ := utils.MessageToAny(&sockets_raw_buffer.RawBuffer{})
	rawBufferTransportSocket := &envoycorev3.TransportSocket{
		Name:       wellknown.TransportSocketRawBuffer,
		ConfigType: &envoycorev3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
	}

	return &envoyclusterv3.Cluster_TransportSocketMatch{
		Name:            fmt.Sprintf("%s-disabled", ourwellknown.TLSModeLabelShortname),
		Match:           &structpb.Struct{},
		TransportSocket: rawBufferTransportSocket,
	}
}

func buildSni(upstream ir.BackendObjectIR) string {
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
