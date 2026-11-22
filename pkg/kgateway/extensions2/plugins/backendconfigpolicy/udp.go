package backendconfigpolicy

import (
	udpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/udp/udp_proxy/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

var _ ir.UdpBackendPolicy = &BackendConfigPolicyIR{}

// udpHashPolicies is what a Ring Hash or Maglev load balancer's hash policies mean for udp_proxy.
type udpHashPolicies struct {
	// hashPolicy is the udp_proxy hash policy from the first sourceIP hash policy, or nil.
	hashPolicy *udpproxyv3.UdpProxyConfig_HashPolicy
	header     bool
	cookie     bool
}

// translateUdpHashPolicies maps hash policies to udp_proxy. Ring Hash and Maglev hash on
// udp_proxy's own hash policy, since the cluster's hash policies read headers, cookies or a
// downstream connection that UDP does not have. Only a sourceIP hash policy carries over.
func translateUdpHashPolicies(policies []kgateway.HashPolicy) udpHashPolicies {
	var out udpHashPolicies
	for _, hp := range policies {
		switch {
		case hp.SourceIP != nil:
			if out.hashPolicy == nil {
				out.hashPolicy = &udpproxyv3.UdpProxyConfig_HashPolicy{
					PolicySpecifier: &udpproxyv3.UdpProxyConfig_HashPolicy_SourceIp{SourceIp: true},
				}
			}
		case hp.Header != nil:
			out.header = true
		case hp.Cookie != nil:
			out.cookie = true
		}
	}
	return out
}

// UdpProxy implements ir.UdpBackendPolicy. It reports the HTTP and TCP settings as ignored.
func (d *BackendConfigPolicyIR) UdpProxy() ir.UdpProxyPolicy {
	var hash udpHashPolicies
	if d.loadBalancerConfig != nil {
		hash = d.loadBalancerConfig.udpHash
	}
	out := ir.UdpProxyPolicy{HashPolicy: hash.hashPolicy}
	for _, f := range []struct {
		name string
		set  bool
	}{
		{"header hashPolicies", hash.header},
		{"cookie hashPolicies", hash.cookie},
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
