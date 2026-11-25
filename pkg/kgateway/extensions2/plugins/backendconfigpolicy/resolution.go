package backendconfigpolicy

import (
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// TLSField is the MergeOrigins key of spec.tls, which other policy kinds may supersede.
const TLSField = "tls"

// bcpField is one top-level BackendConfigPolicy field, named by its MergeOrigins key. The
// table backs SetFields and ClearFields, so it must list every field merge.go merges;
// TestFieldTableMatchesMerge enforces that.
type bcpField struct {
	name  string
	isSet func(*BackendConfigPolicyIR) bool
	clear func(*BackendConfigPolicyIR)
}

var bcpFields = []bcpField{
	{"connectTimeout", func(p *BackendConfigPolicyIR) bool { return p.connectTimeout != nil }, func(p *BackendConfigPolicyIR) { p.connectTimeout = nil }},
	{"perConnectionBufferLimitBytes", func(p *BackendConfigPolicyIR) bool { return p.perConnectionBufferLimitBytes != nil }, func(p *BackendConfigPolicyIR) { p.perConnectionBufferLimitBytes = nil }},
	{"tcpKeepalive", func(p *BackendConfigPolicyIR) bool { return p.tcpKeepalive != nil }, func(p *BackendConfigPolicyIR) { p.tcpKeepalive = nil }},
	{"commonHttpProtocolOptions", func(p *BackendConfigPolicyIR) bool { return p.commonHttpProtocolOptions != nil }, func(p *BackendConfigPolicyIR) { p.commonHttpProtocolOptions = nil }},
	{"http1ProtocolOptions", func(p *BackendConfigPolicyIR) bool { return p.http1ProtocolOptions != nil }, func(p *BackendConfigPolicyIR) { p.http1ProtocolOptions = nil }},
	{"http2ProtocolOptions", func(p *BackendConfigPolicyIR) bool { return p.http2ProtocolOptions != nil }, func(p *BackendConfigPolicyIR) { p.http2ProtocolOptions = nil }},
	{TLSField, func(p *BackendConfigPolicyIR) bool { return p.tlsConfig != nil }, func(p *BackendConfigPolicyIR) { p.tlsConfig = nil }},
	{"loadBalancer", func(p *BackendConfigPolicyIR) bool { return p.loadBalancerConfig != nil }, func(p *BackendConfigPolicyIR) { p.loadBalancerConfig = nil }},
	{"healthCheck", func(p *BackendConfigPolicyIR) bool { return p.healthCheck != nil }, func(p *BackendConfigPolicyIR) { p.healthCheck = nil }},
	{"outlierDetection", func(p *BackendConfigPolicyIR) bool { return p.outlierDetection != nil }, func(p *BackendConfigPolicyIR) { p.outlierDetection = nil }},
	{"circuitBreakers", func(p *BackendConfigPolicyIR) bool { return p.circuitBreakers != nil }, func(p *BackendConfigPolicyIR) { p.circuitBreakers = nil }},
	{"dns.refreshRate", func(p *BackendConfigPolicyIR) bool { return p.dnsRefreshRate != nil }, func(p *BackendConfigPolicyIR) { p.dnsRefreshRate = nil }},
	{"dns.jitter", func(p *BackendConfigPolicyIR) bool { return p.dnsJitter != nil }, func(p *BackendConfigPolicyIR) { p.dnsJitter = nil }},
	{"dns.respectTTL", func(p *BackendConfigPolicyIR) bool { return p.respectDnsTtl != nil }, func(p *BackendConfigPolicyIR) { p.respectDnsTtl = nil }},
	{"dns.lookupFamily", func(p *BackendConfigPolicyIR) bool { return p.dnsLookupFamily != nil }, func(p *BackendConfigPolicyIR) { p.dnsLookupFamily = nil }},
	{"upstreamProxyProtocol", func(p *BackendConfigPolicyIR) bool { return p.upstreamProxyProtocol != nil }, func(p *BackendConfigPolicyIR) { p.upstreamProxyProtocol = nil }},
	{"tunnel", func(p *BackendConfigPolicyIR) bool { return p.tunnel != nil }, func(p *BackendConfigPolicyIR) { p.tunnel = nil }},
	{"sessionPersistence", func(p *BackendConfigPolicyIR) bool { return p.sessionPersistence != nil }, func(p *BackendConfigPolicyIR) { p.sessionPersistence = nil }},
}

// setFields is the plugin's SetFields hook.
func setFields(polir ir.PolicyIR) []string {
	p, ok := polir.(*BackendConfigPolicyIR)
	if !ok || p == nil {
		return nil
	}
	var out []string
	for _, f := range bcpFields {
		if f.isSet(p) {
			out = append(out, f.name)
		}
	}
	return out
}

// clearFields is the plugin's ClearFields hook. Fields are pointers to immutable values,
// so a shallow copy is enough to leave pol untouched.
func clearFields(polir ir.PolicyIR, fields []string) ir.PolicyIR {
	p, ok := polir.(*BackendConfigPolicyIR)
	if !ok || p == nil {
		return polir
	}
	c := *p
	for _, name := range fields {
		for _, f := range bcpFields {
			if f.name == name {
				f.clear(&c)
			}
		}
	}
	return &c
}

// effectivePolicy returns the BackendConfigPolicy that translation applies to backend, nil
// when none is attached or the merged policy has errors (translation then blackholes the
// cluster). It reads the backend's policy resolution, so it agrees with translation and
// status.
func effectivePolicy(backend *ir.BackendObjectIR) *BackendConfigPolicyIR {
	att, ok := effectiveAttachment(backend)
	if !ok {
		return nil
	}
	pol, _ := att.PolicyIr.(*BackendConfigPolicyIR)
	return pol
}

// effectiveAttachment is effectivePolicy with the merged attachment; mergeOwner names the
// policy that supplies each of its fields.
func effectiveAttachment(backend *ir.BackendObjectIR) (ir.PolicyAtt, bool) {
	att, ok := backend.PolicyResolution.Effective(wellknown.BackendConfigPolicyGVK.GroupKind())
	if !ok || len(att.Errors) > 0 {
		return ir.PolicyAtt{}, false
	}
	return att, true
}
