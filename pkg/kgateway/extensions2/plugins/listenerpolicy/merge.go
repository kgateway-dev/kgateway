package listenerpolicy

import (
	"fmt"

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
	if p1 != nil && p2 != nil {
		if p1.NoOrigin || p2.NoOrigin {
			p1.NoOrigin = true
			p2.NoOrigin = true
		}
	}

	mergeFuncs := []func(*ListenerPolicyIR, *ListenerPolicyIR, *ir.AttachedPolicyRef, ir.MergeOrigins, policy.MergeOptions, ir.MergeOrigins){
		mergeDefault,
		mergePerPort,
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
	origin := "default."
	if (p1 != nil && p1.NoOrigin) || (p2 != nil && p2.NoOrigin) {
		origin = ""
	}
	mergeListenerPolicy(origin, &p1.defaultPolicy, &p2.defaultPolicy, p2Ref, p2MergeOrigins, opts, mergeOrigins)
}

func mergePerPort(
	p1, p2 *ListenerPolicyIR,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	for port, p2PortPolicy := range p2.perPortPolicy {
		if p1PortPolicy, ok := p1.perPortPolicy[port]; ok {
			f := fmt.Sprintf("perPortPolicy[%d].", port)
			if (p1 != nil && p1.NoOrigin) || (p2 != nil && p2.NoOrigin) {
				f = ""
			}
			mergeListenerPolicy(f, &p1PortPolicy, &p2PortPolicy, p2Ref, p2MergeOrigins, opts, mergeOrigins)
			p1.perPortPolicy[port] = p1PortPolicy
		} else {
			f := fmt.Sprintf("perPortPolicy[%d]", port)
			if (p1 != nil && p1.NoOrigin) || (p2 != nil && p2.NoOrigin) {
				f = ""
			}
			if p1.perPortPolicy == nil {
				p1.perPortPolicy = map[uint32]listenerPolicy{}
			}
			p1.perPortPolicy[port] = p2PortPolicy
			mergeOrigins.SetOne(f, p2Ref, p2MergeOrigins)
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
		mergePerConnectionBufferLimitBytes,
		mergeHttpSettings,
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
	if origin != "" {
		origin += "httpSettings."
	}
	MergeHttpPolicies(origin, p1.http, p2.http, p2Ref, p2MergeOrigins, opts, mergeOrigins, "" /*no merge settings*/)
}
