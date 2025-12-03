package listenerpolicy

import (
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/httplistenerpolicy"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

func mergePolicies(
	p1, p2 *listenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	mergeOpts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
	_ string, // no merge settings
) {
	if p1 == nil || p2 == nil {
		return
	}

	mergeFuncs := []func(*listenerPolicy, *listenerPolicy, *ir.AttachedPolicyRef, ir.MergeOrigins, policy.MergeOptions, ir.MergeOrigins){
		mergeProxyProtocol,
		mergePerConnectionBufferLimitBytes,
		mergeHttp,
	}

	for _, mergeFunc := range mergeFuncs {
		mergeFunc(p1, p2, p2Ref, p2MergeOrigins, mergeOpts, mergeOrigins)
	}
}

func mergeProxyProtocol(
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
	mergeOrigins.SetOne("proxyProtocol", p2Ref, p2MergeOrigins)
}

func mergePerConnectionBufferLimitBytes(
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
	mergeOrigins.SetOne("perConnectionBufferLimitBytes", p2Ref, p2MergeOrigins)
}

func mergeHttp(
	p1, p2 *listenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.http, p2.http, opts) {
		return
	}
	httplistenerpolicy.MergePolicies(p1.http, p2.http, p2Ref, p2MergeOrigins, opts, mergeOrigins, "" /*no merge settings*/)
}
