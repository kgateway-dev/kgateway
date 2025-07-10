// The "sandwich" pattern allows any L7 Proxy to act as a Waypoint without
// implementing mTLS or CONNECT/HBONE. Instead, the L7 Waypoint gets these
// features directly from the zTunnel L4 mesh.
//
// The zTunnel will pass the following information over PROXY protocol:
//
// * Original source IP/port (standard PROXY)
// * Original destination IP/port (standard PROXY)
// * Authenticated client identity from terminated mTLS. (TLV header 0xD0)
//
// In the future, we may expand this protocol to pass the target service's
// logical hostname over a TLV to avoid relying on service VIPs which may
// be cluster-local.
//
// The zTunnel is informed to send the PROXY data at the start of the TCP stream
// using the annotation: "ambient.istio.io/waypoint-inbound-binding" on either
// the GatewayClass or Gateway resource.
//
// This plugin will add a `ListenerFilter` and a `NetworkFilter` to _every_
// chain on the listener to handle the "sandwich" pattern in Istio ambient.
// The result is that normal Envoy semantics will apply, such as matching the
// restored source/destination addresses in FilterChainMatches or validating
// the client identity in RBAC filters (both NetworkFilter and HTTPFilter) using
// the filter state key that is standard in Istio (io.istio.peer_principal).
//
// PROXY Protocol spec: https://www.haproxy.org/download/1.8/doc/proxy-protocol.txt
// Waypoint Binding annotation: https://github.com/istio/api/blob/bccd18b8afa7ba1fbcc1263d7ecb814922b5358a/annotation/annotations.yaml#L507
// Original one-pager: https://docs.google.com/document/d/1MFPLVTatJI4Z9a88bSo9pZ8z9ftweHUYXpH7zdrXU94/
package sandwich

import (
	"context"
	"time"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	sfsvalue "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/common/set_filter_state/v3"
	proxy_protocol "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/proxy_protocol/v3"
	sfsnetwork "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/set_filter_state/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"k8s.io/apimachinery/pkg/runtime/schema"

	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"

	"istio.io/istio/pilot/pkg/util/protoconv"
)

func NewPlugin() extensionsplug.Plugin {
	return extensionsplug.Plugin{
		ContributesPolicies: extensionsplug.ContributesPolicies{
			SandwichedInboundGK: extensionsplug.PolicyPlugin{
				Name: "sandwich",
				NewGatewayTranslationPass: func(ctx context.Context, tctx ir.GwTranslationCtx, reporter reports.Reporter) ir.ProxyTranslationPass {
					// TODO we could read the waypoint-inbound-binding annotation here and set isSandwiched = true
					// instead of using a policy set by translator?
					return &sandwichedTranslationPass{
						reporter: reporter,
					}
				},
			},
		},
	}
}

var SandwichedInboundGK = schema.GroupKind{
	Group: "internal.kgateway.dev",
	Kind:  "SandwichedInboundPolicy",
}

// SandwichedInboundPolicy is a marker that indicates that this proxy
// expects to receive PROXY Protocol based on Istio's sandwiching capabilities
// that pass metadata from the zTunnel including:
// * 0xD0 - authenticated client identity
// * (potentially more in the future like service hostname)
type SandwichedInboundPolicy struct{}

var _ ir.PolicyIR = SandwichedInboundPolicy{}

func (w SandwichedInboundPolicy) CreationTime() time.Time {
	// not a real policy, no creation time
	return time.Time{}
}

func (w SandwichedInboundPolicy) Equals(in any) bool {
	// no fields, always equal
	return true
}

type sandwichedTranslationPass struct {
	ir.UnimplementedProxyTranslationPass
	reporter reports.Reporter
	// isSandwiched is marked true when we process the listener
	// so that we add the FilterChain level network filters
	isSandwiched bool
}

var _ ir.ProxyTranslationPass = &sandwichedTranslationPass{}

// ApplyListenerPlugin adds a ProxyProtocol ListenerFilter that
// 1. Overrides source and destination addresses to be what the zTunnel saw.
// 2. Grabs the ProxyProtocolPeerTLV (0xD0) used to propagate the client identity validated by zTunnel.
func (s *sandwichedTranslationPass) ApplyListenerPlugin(ctx context.Context, pCtx *ir.ListenerContext, out *listenerv3.Listener) {
	_, ok := pCtx.Policy.(SandwichedInboundPolicy)
	if !ok {
		return
	}

	out.ListenerFilters = append(out.GetListenerFilters(), ProxyProtocolTLV)
	s.isSandwiched = true

	return
}

// NetworkFilters adds the ProxyProtocolTLVAuthorityNetworkFilter which makes
// the identity validated by zTunnel readable from Istio RBAC filters.
// It does this by passing the TLV from PROXY Protocol into filter_state that
// Istio's RBAC will read from.
func (s *sandwichedTranslationPass) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	if !s.isSandwiched {
		return nil, nil
	}

	return []plugins.StagedNetworkFilter{
		{
			Filter: ProxyProtocolTLVAuthorityNetworkFilter,
			Stage:  plugins.BeforeStage(plugins.AuthZStage),
		},
	}, nil
}

// ProxyProtocolPeerTLV is where zTunnel will pass along the validated client
// identity in TLV headers. Ultimately consumed in RBAC filters.
// https://github.com/istio/ztunnel/blob/7f147c658f034de263f975f36c2e9a1fac01e89b/src/proxy.rs#L488
const ProxyProtocolPeerTLV uint32 = 0xD0

// ProxyProtocolTLV is a listener filter that extracts the principal validated
// by zTunnel and puts it into dynamic metadata.
var (
	ProxyProtocolTLV = &listenerv3.ListenerFilter{
		Name: wellknown.ProxyProtocol,
		ConfigType: &listenerv3.ListenerFilter_TypedConfig{
			TypedConfig: protoconv.MessageToAny(&proxy_protocol.ProxyProtocol{
				Rules: []*proxy_protocol.ProxyProtocol_Rule{{
					TlvType: ProxyProtocolPeerTLV,
					OnTlvPresent: &proxy_protocol.ProxyProtocol_KeyValuePair{
						Key: "peer_principal",
					},
				}},
				AllowRequestsWithoutProxyProtocol: false,
			}),
		},
	}

	// ProxyProtocolTLVAuthorityNetworkFilter takes the identity from dynamic
	// metadata and moves it to the well-known filter state Istio uses in RBAC.
	// This allows re-using Istio's control plane as a library to implement
	// AuthorizationPolicy support.
	ProxyProtocolTLVAuthorityNetworkFilter = &listenerv3.Filter{
		Name: "proxy_protocol_authority",
		ConfigType: &listenerv3.Filter_TypedConfig{
			TypedConfig: protoconv.MessageToAny(&sfsnetwork.Config{
				OnNewConnection: []*sfsvalue.FilterStateValue{{
					Key: &sfsvalue.FilterStateValue_ObjectKey{
						ObjectKey: "io.istio.peer_principal",
					},
					FactoryKey: "envoy.string",
					Value: &sfsvalue.FilterStateValue_FormatString{
						FormatString: &core.SubstitutionFormatString{
							Format: &core.SubstitutionFormatString_TextFormatSource{
								TextFormatSource: &core.DataSource{
									Specifier: &core.DataSource_InlineString{
										InlineString: "%DYNAMIC_METADATA(envoy.filters.listener.proxy_protocol:peer_principal)%",
									},
								},
							},
						},
					},
					SharedWithUpstream: sfsvalue.FilterStateValue_ONCE,
				}},
			}),
		},
	}
)
