package listenerpolicy

import (
	"fmt"
	"strings"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

func MergePolicies(
	p1, p2 *ListenerPolicyIR,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	mergeOpts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
	_ string, // no merge settings
) {
	if p1 == nil || p2 == nil {
		return
	}

	// mergePerPort runs first: it seeds new port entries from p1's default as it was before p2 is
	// merged into it.
	mergeFuncs := []func(*ListenerPolicyIR, *ListenerPolicyIR, *ir.AttachedPolicyRef, ir.MergeOrigins, policy.MergeOptions, ir.MergeOrigins){
		mergePerPort,
		mergeDefault,
	}

	for _, mergeFunc := range mergeFuncs {
		mergeFunc(p1, p2, p2Ref, p2MergeOrigins, mergeOpts, mergeOrigins)
	}
}

func mergeDefault(
	p1, p2 *ListenerPolicyIR,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	mergeListenerPolicy("default.", &p1.defaultPolicy, &p2.defaultPolicy, p2Ref, p2MergeOrigins, opts, mergeOrigins)
}

// mergePerPort merges the per-port entries of p2 into p1. A port entry is used instead of a
// policy's default, not on top of it (see getPolicy), so each side contributes its effective
// config for the port: its entry when it has one, otherwise its default. Without this, a
// policy's entry for a port would hide every other policy's default on that port.
func mergePerPort(
	p1, p2 *ListenerPolicyIR,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if len(p1.perPortPolicy) == 0 && len(p2.perPortPolicy) == 0 {
		return
	}
	if p1.perPortPolicy == nil {
		p1.perPortPolicy = map[uint32]listenerPolicy{}
	}
	ports := make([]uint32, 0, len(p1.perPortPolicy)+len(p2.perPortPolicy))
	for port := range p1.perPortPolicy {
		ports = append(ports, port)
	}
	for port := range p2.perPortPolicy {
		if _, ok := p1.perPortPolicy[port]; !ok {
			ports = append(ports, port)
		}
	}

	for _, port := range ports {
		origin := fmt.Sprintf("perPortPolicy[%d].", port)
		p1PortPolicy, ok := p1.perPortPolicy[port]
		if !ok {
			p1PortPolicy = p1.defaultPolicy.clone()
			copyDefaultMergeOrigins(mergeOrigins, mergeOrigins, origin)
		}
		p2PortPolicy, ok := p2.perPortPolicy[port]
		p2PortMergeOrigins := p2MergeOrigins
		if !ok {
			p2PortPolicy = p2.defaultPolicy
			if p2Ref == nil && p2MergeOrigins.IsSet() {
				p2PortMergeOrigins = ir.MergeOrigins{}
				copyDefaultMergeOrigins(p2PortMergeOrigins, p2MergeOrigins, origin)
			}
		}
		mergeListenerPolicy(origin, &p1PortPolicy, &p2PortPolicy, p2Ref, p2PortMergeOrigins, opts, mergeOrigins)
		p1.perPortPolicy[port] = p1PortPolicy
	}
}

// copyDefaultMergeOrigins records the origins of each "default." field of src under portOrigin in dst.
func copyDefaultMergeOrigins(dst, src ir.MergeOrigins, portOrigin string) {
	for field, refs := range src {
		if f, ok := strings.CutPrefix(field, "default."); ok {
			dst[portOrigin+f] = refs.Clone()
		}
	}
}

func mergeListenerPolicy(
	origin string,
	p1, p2 *listenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	mergeOpts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	mergeFuncs := []func(string, *listenerPolicy, *listenerPolicy, *ir.AttachedPolicyRef, ir.MergeOrigins, policy.MergeOptions, ir.MergeOrigins){
		mergeProxyProtocol,
		mergeTCPKeepalive,
		mergePerConnectionBufferLimitBytes,
		mergeTransportSocketConnectTimeout,
		mergeClientCertificateValidation,
		mergeHttpSettings,
		mergeTcpSettings,
	}

	for _, mergeFunc := range mergeFuncs {
		mergeFunc(origin, p1, p2, p2Ref, p2MergeOrigins, mergeOpts, mergeOrigins)
	}
}

func mergeProxyProtocol(
	origin string,
	p1, p2 *listenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.proxyProtocol, p2.proxyProtocol, opts) {
		return
	}

	p1.proxyProtocol = p2.proxyProtocol
	mergeOrigins.SetOne(origin+"proxyProtocol", p2Ref, p2MergeOrigins)
}

func mergePerConnectionBufferLimitBytes(
	origin string,
	p1, p2 *listenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.perConnectionBufferLimitBytes, p2.perConnectionBufferLimitBytes, opts) {
		return
	}

	p1.perConnectionBufferLimitBytes = p2.perConnectionBufferLimitBytes
	mergeOrigins.SetOne(origin+"perConnectionBufferLimitBytes", p2Ref, p2MergeOrigins)
}

func mergeTransportSocketConnectTimeout(
	origin string,
	p1, p2 *listenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.transportSocketConnectTimeout, p2.transportSocketConnectTimeout, opts) {
		return
	}

	p1.transportSocketConnectTimeout = p2.transportSocketConnectTimeout
	mergeOrigins.SetOne(origin+"transportSocketConnectTimeout", p2Ref, p2MergeOrigins)
}

func mergeTCPKeepalive(
	origin string,
	p1, p2 *listenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.tcpKeepalive, p2.tcpKeepalive, opts) {
		return
	}

	p1.tcpKeepalive = p2.tcpKeepalive
	mergeOrigins.SetOne(origin+"tcpKeepalive", p2Ref, p2MergeOrigins)
}

func mergeClientCertificateValidation(
	origin string,
	p1, p2 *listenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.clientCertificateValidation, p2.clientCertificateValidation, opts) {
		return
	}

	p1.clientCertificateValidation = p2.clientCertificateValidation
	mergeOrigins.SetOne(origin+"clientCertificateValidation", p2Ref, p2MergeOrigins)
}

func mergeHttpSettings(
	origin string,
	p1, p2 *listenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if p2.http == nil {
		return
	}
	if p1.http == nil {
		p1.http = &HttpListenerPolicyIr{}
	}
	origin += "httpSettings."
	MergeHttpPolicies(origin, p1.http, p2.http, p2Ref, p2MergeOrigins, opts, mergeOrigins, "" /*no merge settings*/)
}

func mergeTcpSettings(
	origin string,
	p1, p2 *listenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.tcp, p2.tcp, opts) {
		return
	}

	p1.tcp = p2.tcp
	mergeOrigins.SetOne(origin+"tcpSettings", p2Ref, p2MergeOrigins)
}
