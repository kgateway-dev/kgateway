package backendconfigpolicy

import (
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	udpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/udp/udp_proxy/v3"
	envoymaglevv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/load_balancing_policies/maglev/v3"
	envoyringhashv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/load_balancing_policies/ring_hash/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

var _ ir.UdpBackendPolicy = &BackendConfigPolicyIR{}

// UdpProxy implements ir.UdpBackendPolicy. Ring Hash and Maglev hash on udp_proxy's own hash
// policy, since the cluster's hash policies read headers, cookies or a downstream connection that
// UDP does not have. A sourceIP hash policy carries over, and UdpProxy reports the HTTP and TCP
// settings as ignored.
func (d *BackendConfigPolicyIR) UdpProxy() ir.UdpProxyPolicy {
	var out ir.UdpProxyPolicy
	var header, cookie bool
	for _, hp := range d.consistentHashPolicies() {
		switch {
		case hp.GetConnectionProperties().GetSourceIp():
			if out.HashPolicy == nil {
				out.HashPolicy = &udpproxyv3.UdpProxyConfig_HashPolicy{
					PolicySpecifier: &udpproxyv3.UdpProxyConfig_HashPolicy_SourceIp{SourceIp: true},
				}
			}
		case hp.GetHeader() != nil:
			header = true
		case hp.GetCookie() != nil:
			cookie = true
		}
	}
	for _, f := range []struct {
		name string
		set  bool
	}{
		{"header hashPolicies", header},
		{"cookie hashPolicies", cookie},
		{"sessionPersistence", d.sessionPersistence != nil},
		{"commonHttpProtocolOptions", d.commonHttpProtocolOptions != nil},
		{"http1ProtocolOptions", d.http1ProtocolOptions != nil},
		{"http2ProtocolOptions", d.http2ProtocolOptions != nil},
		{"tls", d.tlsConfig != nil},
		{"tcpKeepalive", d.tcpKeepalive != nil},
		{"upstreamProxyProtocol", d.upstreamProxyProtocol != nil},
		{"tunnel", d.tunnel != nil},
	} {
		if f.set {
			out.Ignored = append(out.Ignored, f.name)
		}
	}
	return out
}

// consistentHashPolicies returns the hash policies of a Ring Hash or Maglev load balancer.
func (d *BackendConfigPolicyIR) consistentHashPolicies() []*envoyroutev3.RouteAction_HashPolicy {
	if d.loadBalancerConfig == nil || d.loadBalancerConfig.loadBalancingPolicy == nil {
		return nil
	}
	for _, p := range d.loadBalancerConfig.loadBalancingPolicy.GetPolicies() {
		msg, err := p.GetTypedExtensionConfig().GetTypedConfig().UnmarshalNew()
		if err != nil {
			continue
		}
		switch lb := msg.(type) {
		case *envoyringhashv3.RingHash:
			return lb.GetConsistentHashingLbConfig().GetHashPolicy()
		case *envoymaglevv3.Maglev:
			return lb.GetConsistentHashingLbConfig().GetHashPolicy()
		}
	}
	return nil
}
