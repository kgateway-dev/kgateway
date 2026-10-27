package backendtlspolicy

import (
	"context"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyproxyprotocolv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/proxy_protocol/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/backendconfigpolicy"
	translatorutils "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/utils"
	kgwellknown "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const httpProtocolOptionsKey = "envoy.extensions.upstreams.http.v3.HttpProtocolOptions"

// processBaseCluster lets Envoy negotiate HTTP/2 or HTTP/1.1 via ALPN on
// clusters that originate TLS because of a BackendTLSPolicy and whose backend
// does not declare an app protocol. Without this, such clusters always speak
// HTTP/1.1, which breaks backends that only accept HTTP/2 over TLS (e.g. gRPC).
//
// It runs as a ProcessBaseCluster hook, after every ProcessBackend hook, so the
// decision does not depend on the order in which policy plugins ran. The
// cluster is left untouched when:
//   - the backend declares an app protocol (HTTP/1.1, HTTP/2 and WebSocket are explicit),
//   - the cluster uses transport socket matches (e.g. Istio auto mTLS, which
//     includes a plaintext match that cannot negotiate ALPN),
//   - the final transport socket is not TLS (possibly wrapped in proxy protocol),
//   - an upstream HTTP version was already chosen, e.g. by BackendConfigPolicy
//     http1ProtocolOptions with at least one field set. An empty HTTP/1.1
//     config does not count as a choice: it is indistinguishable from the
//     placeholder BackendConfigPolicy adds alongside commonHttpProtocolOptions.
//
// BackendConfigPolicy leaves its http2ProtocolOptions to this hook for backends without an app protocol
// that have a BackendTLSPolicy attached, so a warning is logged here when they cannot be applied.
func processBaseCluster(_ krt.HandlerContext, _ context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
	if in.AppProtocol != ir.DefaultAppProtocol || len(in.AttachedPolicies.Policies[backendTlsPolicyGroupKind.GroupKind()]) == 0 {
		return
	}
	if !enableALPN(in, out) && backendconfigpolicy.Http2ProtocolOptions(in) != nil {
		logger.Warn("can't apply http2 protocol options: backend does not negotiate HTTP/2 via ALPN", "backend", in.GetName())
	}
}

// enableALPN lets the cluster negotiate HTTP/2 or HTTP/1.1 via ALPN, and reports whether it did.
func enableALPN(in ir.BackendObjectIR, out *envoyclusterv3.Cluster) bool {
	if !hasBackendTLSPolicy(in) {
		return false
	}
	if len(out.GetTransportSocketMatches()) > 0 {
		return false
	}
	if !isTLSTransportSocket(out.GetTransportSocket()) {
		return false
	}
	existing, err := httpProtocolOptions(out)
	if err != nil {
		logger.Error("failed to read http protocol options", "cluster", out.GetName(), "error", err)
		return false
	}
	if !upstreamProtocolUnset(existing) {
		return false
	}
	// Use the BackendConfigPolicy HTTP/2 options, if any, for when HTTP/2 is negotiated. Allow
	// extended CONNECT so WebSocket upgrades can still reach the backend over HTTP/2 (RFC 8441).
	http2 := &envoycorev3.Http2ProtocolOptions{}
	if bcp := backendconfigpolicy.Http2ProtocolOptions(in); bcp != nil {
		http2 = proto.CloneOf(bcp)
	}
	http2.AllowConnect = true
	if err := translatorutils.MutateHttpOptions(out, func(opts *envoy_upstreams_v3.HttpProtocolOptions) {
		opts.UpstreamProtocolOptions = &envoy_upstreams_v3.HttpProtocolOptions_AutoConfig{
			AutoConfig: &envoy_upstreams_v3.HttpProtocolOptions_AutoHttpConfig{
				HttpProtocolOptions:  &envoycorev3.Http1ProtocolOptions{},
				Http2ProtocolOptions: http2,
			},
		}
	}); err != nil {
		logger.Error("failed to enable ALPN protocol selection", "cluster", out.GetName(), "error", err)
		return false
	}
	return true
}

// hasBackendTLSPolicy reports whether a valid BackendTLSPolicy that sets a
// transport socket is attached to the backend.
func hasBackendTLSPolicy(in ir.BackendObjectIR) bool {
	for _, att := range in.AttachedPolicies.Policies[backendTlsPolicyGroupKind.GroupKind()] {
		if len(att.Errors) > 0 {
			continue
		}
		if pol, ok := att.PolicyIr.(*backendTlsPolicy); ok && pol.transportSocket != nil {
			return true
		}
	}
	return false
}

func isTLSTransportSocket(ts *envoycorev3.TransportSocket) bool {
	switch ts.GetName() {
	case wellknown.TransportSocketTls:
		return true
	case kgwellknown.TransportSocketUpstreamProxyProtocol:
		inner, ok := innerOfProxyProtocol(ts)
		return ok && inner.GetName() == wellknown.TransportSocketTls
	default:
		return false
	}
}

func innerOfProxyProtocol(ts *envoycorev3.TransportSocket) (*envoycorev3.TransportSocket, bool) {
	pp := &envoyproxyprotocolv3.ProxyProtocolUpstreamTransport{}
	if err := ts.GetTypedConfig().UnmarshalTo(pp); err != nil {
		return nil, false
	}
	return pp.GetTransportSocket(), true
}

func httpProtocolOptions(out *envoyclusterv3.Cluster) (*envoy_upstreams_v3.HttpProtocolOptions, error) {
	raw, ok := out.GetTypedExtensionProtocolOptions()[httpProtocolOptionsKey]
	if !ok {
		return nil, nil
	}
	opts := &envoy_upstreams_v3.HttpProtocolOptions{}
	if err := anypb.UnmarshalTo(raw, opts, proto.UnmarshalOptions{}); err != nil {
		return nil, err
	}
	return opts, nil
}

// upstreamProtocolUnset reports whether no upstream HTTP version has been
// chosen: either there are no protocol options, or the only choice is an
// empty HTTP/1.1 config, such as the placeholder Envoy requires next to common
// options.
func upstreamProtocolUnset(opts *envoy_upstreams_v3.HttpProtocolOptions) bool {
	switch upo := opts.GetUpstreamProtocolOptions().(type) {
	case nil:
		return true
	case *envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_:
		h1, ok := upo.ExplicitHttpConfig.GetProtocolConfig().(*envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions)
		return ok && proto.Equal(h1.HttpProtocolOptions, &envoycorev3.Http1ProtocolOptions{})
	default:
		return false
	}
}
