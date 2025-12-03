package listenerpolicy

import (
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

func mergePolicies(
	p1, p2 *listenerPolicyIR,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	mergeOpts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
	_ string, // no merge settings
) {
	if p1 == nil || p2 == nil {
		return
	}

	mergeFuncs := []func(*listenerPolicyIR, *listenerPolicyIR, *ir.AttachedPolicyRef, ir.MergeOrigins, policy.MergeOptions, ir.MergeOrigins){
		mergeDefault,
		mergePerPort,
	}

	for _, mergeFunc := range mergeFuncs {
		mergeFunc(p1, p2, p2Ref, p2MergeOrigins, mergeOpts, mergeOrigins)
	}
}

func mergeDefault(
	p1, p2 *listenerPolicyIR,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.defaultPolicy, p2.defaultPolicy, opts) {
		return
	}

	p1.defaultPolicy = p2.defaultPolicy
	mergeOrigins.SetOne("defaultPolicy", p2Ref, p2MergeOrigins)
}

func mergePerPort(
	p1, p2 *listenerPolicyIR,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.perPortPolicy, p2.perPortPolicy, opts) {
		return
	}

	p1.perPortPolicy = p2.perPortPolicy
	mergeOrigins.SetOne("perPortPolicy", p2Ref, p2MergeOrigins)
}
