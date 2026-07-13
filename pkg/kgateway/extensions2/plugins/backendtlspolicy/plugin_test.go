package backendtlspolicy

import (
	"context"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyproxyprotocolv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/proxy_protocol/v3"
	envoyrawbufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/raw_buffer/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func tlsSocket(t *testing.T, sni string) *envoycorev3.TransportSocket {
	t.Helper()
	tlsAny, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{Sni: sni})
	require.NoError(t, err)
	return &envoycorev3.TransportSocket{
		Name: envoywellknown.TransportSocketTls,
		ConfigType: &envoycorev3.TransportSocket_TypedConfig{
			TypedConfig: tlsAny,
		},
	}
}

func proxyProtocolWrappedRawBuffer(t *testing.T) *envoycorev3.TransportSocket {
	t.Helper()
	rawAny, err := utils.MessageToAny(&envoyrawbufferv3.RawBuffer{})
	require.NoError(t, err)
	pp := &envoyproxyprotocolv3.ProxyProtocolUpstreamTransport{
		Config: &envoycorev3.ProxyProtocolConfig{Version: envoycorev3.ProxyProtocolConfig_V2},
		TransportSocket: &envoycorev3.TransportSocket{
			Name: envoywellknown.TransportSocketRawBuffer,
			ConfigType: &envoycorev3.TransportSocket_TypedConfig{
				TypedConfig: rawAny,
			},
		},
	}
	ppAny, err := utils.MessageToAny(pp)
	require.NoError(t, err)
	return &envoycorev3.TransportSocket{
		Name: wellknown.TransportSocketUpstreamProxyProtocol,
		ConfigType: &envoycorev3.TransportSocket_TypedConfig{
			TypedConfig: ppAny,
		},
	}
}

// TestProcessBackend_NoExistingSocket: BTP installs its TLS socket when nothing
// is already set.
func TestProcessBackend_NoExistingSocket(t *testing.T) {
	pol := &backendTlsPolicy{transportSocket: tlsSocket(t, "btp.example.com")}
	cluster := &envoyclusterv3.Cluster{}

	processBackend(context.Background(), pol, ir.BackendObjectIR{}, cluster)

	require.NotNil(t, cluster.TransportSocket)
	assert.Equal(t, envoywellknown.TransportSocketTls, cluster.TransportSocket.GetName())
}

// TestProcessBackend_ReplacesInnerOfProxyProtocolWrapper: if BCP already
// wrapped the cluster in upstream proxy protocol with a raw_buffer inner, BTP
// must replace the inner with its TLS socket and keep the wrapper.
func TestProcessBackend_ReplacesInnerOfProxyProtocolWrapper(t *testing.T) {
	pol := &backendTlsPolicy{transportSocket: tlsSocket(t, "btp.example.com")}
	cluster := &envoyclusterv3.Cluster{TransportSocket: proxyProtocolWrappedRawBuffer(t)}

	processBackend(context.Background(), pol, ir.BackendObjectIR{}, cluster)

	require.NotNil(t, cluster.TransportSocket)
	require.Equal(t, wellknown.TransportSocketUpstreamProxyProtocol, cluster.TransportSocket.GetName(),
		"BTP must preserve the upstream_proxy_protocol wrapper")

	pp := &envoyproxyprotocolv3.ProxyProtocolUpstreamTransport{}
	require.NoError(t, cluster.TransportSocket.GetTypedConfig().UnmarshalTo(pp))
	require.NotNil(t, pp.TransportSocket)
	assert.Equal(t, envoywellknown.TransportSocketTls, pp.TransportSocket.GetName(),
		"inner socket should be replaced with BTP's TLS socket")

	inner := &envoytlsv3.UpstreamTlsContext{}
	require.NoError(t, pp.TransportSocket.GetTypedConfig().UnmarshalTo(inner))
	assert.Equal(t, "btp.example.com", inner.GetSni(), "inner TLS context should come from BTP")
}

// TestProcessBackend_NilPolicySocket: a BTP that did not produce a transport
// socket (e.g. translation error) must not touch the cluster's existing socket.
func TestProcessBackend_NilPolicySocket(t *testing.T) {
	pol := &backendTlsPolicy{}
	existing := proxyProtocolWrappedRawBuffer(t)
	cluster := &envoyclusterv3.Cluster{TransportSocket: existing}

	processBackend(context.Background(), pol, ir.BackendObjectIR{}, cluster)
	assert.Same(t, existing, cluster.TransportSocket)
}

// TestFilterTargetRefs checks that valid backend-like refs pass through and
// non-backend refs (routes, gateways) are rejected with InvalidTargetRefError.
func TestFilterTargetRefs(t *testing.T) {
	ref := func(group, kind, name string) gwv1.LocalPolicyTargetReferenceWithSectionName {
		return gwv1.LocalPolicyTargetReferenceWithSectionName{
			LocalPolicyTargetReference: gwv1.LocalPolicyTargetReference{
				Group: gwv1.Group(group),
				Kind:  gwv1.Kind(kind),
				Name:  gwv1.ObjectName(name),
			},
		}
	}

	tests := []struct {
		name      string
		refs      []gwv1.LocalPolicyTargetReferenceWithSectionName
		wantValid int
		wantErrs  int
	}{
		{
			name:      "Service ref is accepted",
			refs:      []gwv1.LocalPolicyTargetReferenceWithSectionName{ref("", "Service", "svc")},
			wantValid: 1,
			wantErrs:  0,
		},
		{
			name:      "Backend ref is accepted",
			refs:      []gwv1.LocalPolicyTargetReferenceWithSectionName{ref("gateway.kgateway.dev", "Backend", "be")},
			wantValid: 1,
			wantErrs:  0,
		},
		{
			name:      "Hostname ref is accepted",
			refs:      []gwv1.LocalPolicyTargetReferenceWithSectionName{ref("networking.istio.io", "Hostname", "host")},
			wantValid: 1,
			wantErrs:  0,
		},
		{
			name:      "HTTPRoute ref is rejected",
			refs:      []gwv1.LocalPolicyTargetReferenceWithSectionName{ref("gateway.networking.k8s.io", "HTTPRoute", "route")},
			wantValid: 0,
			wantErrs:  1,
		},
		{
			name:      "Gateway ref is rejected",
			refs:      []gwv1.LocalPolicyTargetReferenceWithSectionName{ref("gateway.networking.k8s.io", "Gateway", "gw")},
			wantValid: 0,
			wantErrs:  1,
		},
		{
			name: "mixed refs split correctly",
			refs: []gwv1.LocalPolicyTargetReferenceWithSectionName{
				ref("", "Service", "svc"),
				ref("gateway.networking.k8s.io", "HTTPRoute", "route"),
			},
			wantValid: 1,
			wantErrs:  1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			valid, errs := filterTargetRefs(tc.refs)
			require.Len(t, valid, tc.wantValid)
			require.Len(t, errs, tc.wantErrs)
			for _, err := range errs {
				var targetRefErr *InvalidTargetRefError
				require.ErrorAs(t, err, &targetRefErr, "error should be InvalidTargetRefError")
			}
		})
	}
}

// TestIsBackendTargetRef checks which kinds are considered valid backend targets.
func TestIsBackendTargetRef(t *testing.T) {
	tests := []struct {
		group string
		kind  string
		want  bool
	}{
		{"", "Service", true},
		{"core", "Service", true},
		{"gateway.kgateway.dev", "Backend", true},
		{"networking.istio.io", "Hostname", true},
		{"gateway.networking.k8s.io", "HTTPRoute", false},
		{"gateway.networking.k8s.io", "GRPCRoute", false},
		{"gateway.networking.k8s.io", "Gateway", false},
		{"", "HTTPRoute", false},
	}
	for _, tc := range tests {
		name := tc.group + "/" + tc.kind
		t.Run(name, func(t *testing.T) {
			ref := gwv1.LocalPolicyTargetReference{
				Group: gwv1.Group(tc.group),
				Kind:  gwv1.Kind(tc.kind),
			}
			assert.Equal(t, tc.want, isBackendTargetRef(ref))
		})
	}
}
