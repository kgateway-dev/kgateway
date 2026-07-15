package listenerpolicy

import (
	"slices"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

func mergeTcpSettings(
	origin string,
	p1, p2 *listenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if p2.tcp == nil {
		return
	}
	if p1.tcp == nil {
		p1.tcp = &TcpListenerPolicyIr{}
	}
	if origin != "" {
		origin += "tcpSettings."
	}
	MergeTcpPolicies(origin, p1.tcp, p2.tcp, p2Ref, p2MergeOrigins, opts, mergeOrigins)
}

func MergeTcpPolicies(
	origin string,
	p1, p2 *TcpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	mergeOpts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if p1 == nil || p2 == nil {
		return
	}

	mergeTcpAccessLog(origin, p1, p2, p2Ref, p2MergeOrigins, mergeOpts, mergeOrigins)
}

func mergeTcpAccessLog(
	origin string,
	p1, p2 *TcpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.accessLogConfig, p2.accessLogConfig, opts) {
		return
	}
	if !policy.IsMergeable(p1.accessLogPolicies, p2.accessLogPolicies, opts) {
		return
	}

	p1.accessLogConfig = slices.Clone(p2.accessLogConfig)
	mergeOrigins.SetOne(origin+"accessLogConfig", p2Ref, p2MergeOrigins)
	p1.accessLogPolicies = slices.Clone(p2.accessLogPolicies)
	mergeOrigins.SetOne(origin+"accessLog", p2Ref, p2MergeOrigins)
}
