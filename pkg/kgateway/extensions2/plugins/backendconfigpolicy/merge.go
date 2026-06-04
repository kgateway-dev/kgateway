package backendconfigpolicy

import (
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

// mergeBackendConfigPolicies merges p2 into p1 field by field. BackendConfigPolicy
// has an orthogonal field spec (connectTimeout, healthCheck, circuitBreakers,
// upstreamProxyProtocol etc), so each top level field is merged independently
// according to the merge strategy in opts. Under AugmentedShallowMerge (the
// default for same hierarchy attachments), the earlier policy in the input list
// wins per field and later policies fill in unset fields.
func mergeBackendConfigPolicies(
	p1, p2 *BackendConfigPolicyIR,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
	_ string,
) {
	if p1 == nil || p2 == nil {
		return
	}

	mergeFuncs := []func(*BackendConfigPolicyIR, *BackendConfigPolicyIR, *ir.AttachedPolicyRef, ir.MergeOrigins, policy.MergeOptions, ir.MergeOrigins){
		mergeConnectTimeout,
		mergePerConnectionBufferLimitBytes,
		mergeTcpKeepalive,
		mergeCommonHttpProtocolOptions,
		mergeHttp1ProtocolOptions,
		mergeHttp2ProtocolOptions,
		mergeTLSConfig,
		mergeLoadBalancerConfig,
		mergeHealthCheck,
		mergeOutlierDetection,
		mergeCircuitBreakers,
		mergeDnsRefreshRate,
		mergeDnsJitter,
		mergeRespectDnsTtl,
		mergeUpstreamProxyProtocol,
	}

	for _, mergeFunc := range mergeFuncs {
		mergeFunc(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins)
	}
}

func mergeConnectTimeout(p1, p2 *BackendConfigPolicyIR, p2Ref *ir.AttachedPolicyRef, p2MergeOrigins ir.MergeOrigins, opts policy.MergeOptions, mergeOrigins ir.MergeOrigins) {
	if !policy.IsMergeable(p1.connectTimeout, p2.connectTimeout, opts) {
		return
	}
	p1.connectTimeout = p2.connectTimeout
	mergeOrigins.SetOne("connectTimeout", p2Ref, p2MergeOrigins)
}

func mergePerConnectionBufferLimitBytes(p1, p2 *BackendConfigPolicyIR, p2Ref *ir.AttachedPolicyRef, p2MergeOrigins ir.MergeOrigins, opts policy.MergeOptions, mergeOrigins ir.MergeOrigins) {
	if !policy.IsMergeable(p1.perConnectionBufferLimitBytes, p2.perConnectionBufferLimitBytes, opts) {
		return
	}
	p1.perConnectionBufferLimitBytes = p2.perConnectionBufferLimitBytes
	mergeOrigins.SetOne("perConnectionBufferLimitBytes", p2Ref, p2MergeOrigins)
}

func mergeTcpKeepalive(p1, p2 *BackendConfigPolicyIR, p2Ref *ir.AttachedPolicyRef, p2MergeOrigins ir.MergeOrigins, opts policy.MergeOptions, mergeOrigins ir.MergeOrigins) {
	if !policy.IsMergeable(p1.tcpKeepalive, p2.tcpKeepalive, opts) {
		return
	}
	p1.tcpKeepalive = p2.tcpKeepalive
	mergeOrigins.SetOne("tcpKeepalive", p2Ref, p2MergeOrigins)
}

func mergeCommonHttpProtocolOptions(p1, p2 *BackendConfigPolicyIR, p2Ref *ir.AttachedPolicyRef, p2MergeOrigins ir.MergeOrigins, opts policy.MergeOptions, mergeOrigins ir.MergeOrigins) {
	if !policy.IsMergeable(p1.commonHttpProtocolOptions, p2.commonHttpProtocolOptions, opts) {
		return
	}
	p1.commonHttpProtocolOptions = p2.commonHttpProtocolOptions
	mergeOrigins.SetOne("commonHttpProtocolOptions", p2Ref, p2MergeOrigins)
}

func mergeHttp1ProtocolOptions(p1, p2 *BackendConfigPolicyIR, p2Ref *ir.AttachedPolicyRef, p2MergeOrigins ir.MergeOrigins, opts policy.MergeOptions, mergeOrigins ir.MergeOrigins) {
	if !policy.IsMergeable(p1.http1ProtocolOptions, p2.http1ProtocolOptions, opts) {
		return
	}
	p1.http1ProtocolOptions = p2.http1ProtocolOptions
	mergeOrigins.SetOne("http1ProtocolOptions", p2Ref, p2MergeOrigins)
}

func mergeHttp2ProtocolOptions(p1, p2 *BackendConfigPolicyIR, p2Ref *ir.AttachedPolicyRef, p2MergeOrigins ir.MergeOrigins, opts policy.MergeOptions, mergeOrigins ir.MergeOrigins) {
	if !policy.IsMergeable(p1.http2ProtocolOptions, p2.http2ProtocolOptions, opts) {
		return
	}
	p1.http2ProtocolOptions = p2.http2ProtocolOptions
	mergeOrigins.SetOne("http2ProtocolOptions", p2Ref, p2MergeOrigins)
}

func mergeTLSConfig(p1, p2 *BackendConfigPolicyIR, p2Ref *ir.AttachedPolicyRef, p2MergeOrigins ir.MergeOrigins, opts policy.MergeOptions, mergeOrigins ir.MergeOrigins) {
	if !policy.IsMergeable(p1.tlsConfig, p2.tlsConfig, opts) {
		return
	}
	p1.tlsConfig = p2.tlsConfig
	mergeOrigins.SetOne("tls", p2Ref, p2MergeOrigins)
}

func mergeLoadBalancerConfig(p1, p2 *BackendConfigPolicyIR, p2Ref *ir.AttachedPolicyRef, p2MergeOrigins ir.MergeOrigins, opts policy.MergeOptions, mergeOrigins ir.MergeOrigins) {
	if !policy.IsMergeable(p1.loadBalancerConfig, p2.loadBalancerConfig, opts) {
		return
	}
	p1.loadBalancerConfig = p2.loadBalancerConfig
	mergeOrigins.SetOne("loadBalancer", p2Ref, p2MergeOrigins)
}

func mergeHealthCheck(p1, p2 *BackendConfigPolicyIR, p2Ref *ir.AttachedPolicyRef, p2MergeOrigins ir.MergeOrigins, opts policy.MergeOptions, mergeOrigins ir.MergeOrigins) {
	if !policy.IsMergeable(p1.healthCheck, p2.healthCheck, opts) {
		return
	}
	p1.healthCheck = p2.healthCheck
	mergeOrigins.SetOne("healthCheck", p2Ref, p2MergeOrigins)
}

func mergeOutlierDetection(p1, p2 *BackendConfigPolicyIR, p2Ref *ir.AttachedPolicyRef, p2MergeOrigins ir.MergeOrigins, opts policy.MergeOptions, mergeOrigins ir.MergeOrigins) {
	if !policy.IsMergeable(p1.outlierDetection, p2.outlierDetection, opts) {
		return
	}
	p1.outlierDetection = p2.outlierDetection
	mergeOrigins.SetOne("outlierDetection", p2Ref, p2MergeOrigins)
}

func mergeCircuitBreakers(p1, p2 *BackendConfigPolicyIR, p2Ref *ir.AttachedPolicyRef, p2MergeOrigins ir.MergeOrigins, opts policy.MergeOptions, mergeOrigins ir.MergeOrigins) {
	if !policy.IsMergeable(p1.circuitBreakers, p2.circuitBreakers, opts) {
		return
	}
	p1.circuitBreakers = p2.circuitBreakers
	mergeOrigins.SetOne("circuitBreakers", p2Ref, p2MergeOrigins)
}

func mergeDnsRefreshRate(p1, p2 *BackendConfigPolicyIR, p2Ref *ir.AttachedPolicyRef, p2MergeOrigins ir.MergeOrigins, opts policy.MergeOptions, mergeOrigins ir.MergeOrigins) {
	if !policy.IsMergeable(p1.dnsRefreshRate, p2.dnsRefreshRate, opts) {
		return
	}
	p1.dnsRefreshRate = p2.dnsRefreshRate
	mergeOrigins.SetOne("dns.refreshRate", p2Ref, p2MergeOrigins)
}

func mergeDnsJitter(p1, p2 *BackendConfigPolicyIR, p2Ref *ir.AttachedPolicyRef, p2MergeOrigins ir.MergeOrigins, opts policy.MergeOptions, mergeOrigins ir.MergeOrigins) {
	if !policy.IsMergeable(p1.dnsJitter, p2.dnsJitter, opts) {
		return
	}
	p1.dnsJitter = p2.dnsJitter
	mergeOrigins.SetOne("dns.jitter", p2Ref, p2MergeOrigins)
}

func mergeRespectDnsTtl(p1, p2 *BackendConfigPolicyIR, p2Ref *ir.AttachedPolicyRef, p2MergeOrigins ir.MergeOrigins, opts policy.MergeOptions, mergeOrigins ir.MergeOrigins) {
	if !policy.IsMergeable(p1.respectDnsTtl, p2.respectDnsTtl, opts) {
		return
	}
	p1.respectDnsTtl = p2.respectDnsTtl
	mergeOrigins.SetOne("dns.respectTTL", p2Ref, p2MergeOrigins)
}

func mergeUpstreamProxyProtocol(p1, p2 *BackendConfigPolicyIR, p2Ref *ir.AttachedPolicyRef, p2MergeOrigins ir.MergeOrigins, opts policy.MergeOptions, mergeOrigins ir.MergeOrigins) {
	if !policy.IsMergeable(p1.upstreamProxyProtocol, p2.upstreamProxyProtocol, opts) {
		return
	}
	p1.upstreamProxyProtocol = p2.upstreamProxyProtocol
	mergeOrigins.SetOne("upstreamProxyProtocol", p2Ref, p2MergeOrigins)
}
