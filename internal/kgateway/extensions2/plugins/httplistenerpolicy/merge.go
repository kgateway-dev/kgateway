package httplistenerpolicy

import (
	"slices"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

func mergePolicies(
	p1, p2 *httpListenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	mergeOpts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if p1 == nil || p2 == nil {
		return
	}

	mergeFuncs := []func(*httpListenerPolicy, *httpListenerPolicy, *ir.AttachedPolicyRef, ir.MergeOrigins, policy.MergeOptions, ir.MergeOrigins){
		mergeAccessLog,
		mergeTracing,
		mergeUpgradeConfigs,
		mergeUseRemoteAddress,
		mergeXffNumTrustedHops,
		mergeServerHeaderTransformation,
		mergeStreamIdleTimeout,
		mergeHealthCheckPolicy,
		mergePreserveHttp1HeaderCase,
		mergeAcceptHttp10,
		mergeDefaultHostForHttp10,
	}

	for _, mergeFunc := range mergeFuncs {
		mergeFunc(p1, p2, p2Ref, p2MergeOrigins, mergeOpts, mergeOrigins)
	}
}

func mergeAccessLog(
	p1, p2 *httpListenerPolicy,
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
	mergeOrigins.SetOne("accessLogConfig", p2Ref, p2MergeOrigins)
	p1.accessLogPolicies = slices.Clone(p2.accessLogPolicies)
	mergeOrigins.SetOne("accessLog", p2Ref, p2MergeOrigins)
}

func mergeTracing(
	p1, p2 *httpListenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.tracingProvider, p2.tracingProvider, opts) {
		return
	}
	if !policy.IsMergeable(p1.tracingConfig, p2.tracingConfig, opts) {
		return
	}

	p1.tracingProvider = p2.tracingProvider
	p1.tracingConfig = p2.tracingConfig
	mergeOrigins.SetOne("tracing", p2Ref, p2MergeOrigins)
}

func mergeUpgradeConfigs(
	p1, p2 *httpListenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.upgradeConfigs, p2.upgradeConfigs, opts) {
		return
	}

	p1.upgradeConfigs = slices.Clone(p2.upgradeConfigs)
	mergeOrigins.SetOne("upgradeConfig", p2Ref, p2MergeOrigins)
}

func mergeUseRemoteAddress(
	p1, p2 *httpListenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.useRemoteAddress, p2.useRemoteAddress, opts) {
		return
	}

	p1.useRemoteAddress = p2.useRemoteAddress
	mergeOrigins.SetOne("useRemoteAddress", p2Ref, p2MergeOrigins)
}

func mergePreserveHttp1HeaderCase(
	p1, p2 *httpListenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.preserveHttp1HeaderCase, p2.preserveHttp1HeaderCase, opts) {
		return
	}

	p1.preserveHttp1HeaderCase = p2.preserveHttp1HeaderCase
	mergeOrigins.SetOne("preserveHttp1HeaderCase", p2Ref, p2MergeOrigins)
}

func mergeAcceptHttp10(
	p1, p2 *httpListenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.acceptHttp10, p2.acceptHttp10, opts) {
		return
	}

	p1.acceptHttp10 = p2.acceptHttp10
	mergeOrigins.SetOne("acceptHttp10", p2Ref, p2MergeOrigins)
}

func mergeDefaultHostForHttp10(
	p1, p2 *httpListenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.defaultHostForHttp10, p2.defaultHostForHttp10, opts) {
		return
	}

	p1.defaultHostForHttp10 = p2.defaultHostForHttp10
	mergeOrigins.SetOne("defaultHostForHttp10", p2Ref, p2MergeOrigins)
}

func mergeXffNumTrustedHops(
	p1, p2 *httpListenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.xffNumTrustedHops, p2.xffNumTrustedHops, opts) {
		return
	}

	p1.xffNumTrustedHops = p2.xffNumTrustedHops
	mergeOrigins.SetOne("xffNumTrustedHops", p2Ref, p2MergeOrigins)
}

func mergeServerHeaderTransformation(
	p1, p2 *httpListenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.serverHeaderTransformation, p2.serverHeaderTransformation, opts) {
		return
	}

	p1.serverHeaderTransformation = p2.serverHeaderTransformation
	mergeOrigins.SetOne("serverHeaderTransformation", p2Ref, p2MergeOrigins)
}

func mergeStreamIdleTimeout(
	p1, p2 *httpListenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.streamIdleTimeout, p2.streamIdleTimeout, opts) {
		return
	}

	p1.streamIdleTimeout = p2.streamIdleTimeout
	mergeOrigins.SetOne("mergeStreamIdleTimeout", p2Ref, p2MergeOrigins)
}

func mergeHealthCheckPolicy(
	p1, p2 *httpListenerPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.healthCheckPolicy, p2.healthCheckPolicy, opts) {
		return
	}

	p1.healthCheckPolicy = p2.healthCheckPolicy
	mergeOrigins.SetOne("healthCheckPolicy", p2Ref, p2MergeOrigins)
}
