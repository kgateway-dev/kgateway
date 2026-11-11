package backendconfigpolicy

import (
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	udpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/udp/udp_proxy/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func TestBackendConfigPolicyUdpProxy(t *testing.T) {
	sourceIP := &udpproxyv3.UdpProxyConfig_HashPolicy{
		PolicySpecifier: &udpproxyv3.UdpProxyConfig_HashPolicy_SourceIp{SourceIp: true},
	}
	tests := []struct {
		name        string
		lb          *kgateway.LoadBalancer
		ir          BackendConfigPolicyIR
		wantHash    *udpproxyv3.UdpProxyConfig_HashPolicy
		wantIgnored []string
	}{
		{
			name: "no load balancer",
		},
		{
			name: "ring hash without hash policies picks hosts at random",
			lb:   &kgateway.LoadBalancer{RingHash: &kgateway.LoadBalancerRingHashConfig{}},
		},
		{
			name: "ring hash sourceIP carries over",
			lb: &kgateway.LoadBalancer{RingHash: &kgateway.LoadBalancerRingHashConfig{
				HashPolicies: []kgateway.HashPolicy{{SourceIP: &kgateway.SourceIP{}}},
			}},
			wantHash: sourceIP,
		},
		{
			name: "maglev sourceIP carries over, header and cookie are ignored",
			lb: &kgateway.LoadBalancer{Maglev: &kgateway.LoadBalancerMaglevConfig{
				HashPolicies: []kgateway.HashPolicy{
					{Header: &kgateway.Header{Name: "x-user"}},
					{Cookie: &kgateway.Cookie{Name: "session"}},
					{SourceIP: &kgateway.SourceIP{}},
				},
			}},
			wantHash:    sourceIP,
			wantIgnored: []string{"header hashPolicies", "cookie hashPolicies"},
		},
		{
			name: "settings that only act on HTTP or TCP are ignored",
			ir: BackendConfigPolicyIR{
				commonHttpProtocolOptions: &envoycorev3.HttpProtocolOptions{},
				tcpKeepalive:              &envoycorev3.TcpKeepalive{},
				sessionPersistence:        &sessionPersistenceIR{},
			},
			wantIgnored: []string{"sessionPersistence", "commonHttpProtocolOptions", "tcpKeepalive"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bcp := tc.ir
			if tc.lb != nil {
				lb, err := translateLoadBalancerConfig(tc.lb, "policy", "ns")
				require.NoError(t, err)
				bcp.loadBalancerConfig = lb
			}
			got := bcp.UdpProxy()
			assert.True(t, proto.Equal(tc.wantHash, got.HashPolicy), "hash policy: got %v", got.HashPolicy)
			assert.Equal(t, tc.wantIgnored, got.Ignored)
		})
	}
}
