package listenerpolicy

import (
	"slices"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

func MergeHttpPolicies(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	mergeOpts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
	_ string, // no merge settings
) {
	if p1 == nil || p2 == nil {
		return
	}

	mergeFuncs := []func(string, *HttpListenerPolicyIr, *HttpListenerPolicyIr, *ir.AttachedPolicyRef, ir.MergeOrigins, policy.MergeOptions, ir.MergeOrigins){
		mergeAccessLog,
		mergeTracing,
		mergeUpgradeConfigs,
		mergeUseRemoteAddress,
		mergePreserveExternalRequestId,
		mergeGenerateRequestId,
		mergeXffNumTrustedHops,
		mergeServerHeaderTransformation,
		mergeStreamIdleTimeout,
		mergeIdleTimeout,
		mergeHealthCheckPolicy,
		mergePreserveHttp1HeaderCase,
		mergeAcceptHttp10,
		mergeDefaultHostForHttp10,
		mergeEarlyHeaderMutation,
		mergeMaxRequestHeadersKb,
		mergeUuidRequestIdConfig,
	}
	for _, mergeFunc := range mergeFuncs {
		mergeFunc(origin, p1, p2, p2Ref, p2MergeOrigins, mergeOpts, mergeOrigins)
	}
}

func mergeAccessLog(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
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

func mergeTracing(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
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
	mergeOrigins.SetOne(origin+"tracing", p2Ref, p2MergeOrigins)
}

func mergeUpgradeConfigs(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.upgradeConfigs, p2.upgradeConfigs, opts) {
		return
	}

	p1.upgradeConfigs = slices.Clone(p2.upgradeConfigs)
	mergeOrigins.SetOne(origin+"upgradeConfig", p2Ref, p2MergeOrigins)
}

func mergeUseRemoteAddress(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.useRemoteAddress, p2.useRemoteAddress, opts) {
		return
	}

	p1.useRemoteAddress = p2.useRemoteAddress
	mergeOrigins.SetOne(origin+"useRemoteAddress", p2Ref, p2MergeOrigins)
}

func mergePreserveExternalRequestId(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.preserveExternalRequestId, p2.preserveExternalRequestId, opts) {
		return
	}

	p1.preserveExternalRequestId = p2.preserveExternalRequestId
	mergeOrigins.SetOne(origin+"preserveExternalRequestId", p2Ref, p2MergeOrigins)
}

func mergeGenerateRequestId(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.generateRequestId, p2.generateRequestId, opts) {
		return
	}

	p1.generateRequestId = p2.generateRequestId
	mergeOrigins.SetOne(origin+"generateRequestId", p2Ref, p2MergeOrigins)
}

func mergePreserveHttp1HeaderCase(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.preserveHttp1HeaderCase, p2.preserveHttp1HeaderCase, opts) {
		return
	}

	p1.preserveHttp1HeaderCase = p2.preserveHttp1HeaderCase
	mergeOrigins.SetOne(origin+"preserveHttp1HeaderCase", p2Ref, p2MergeOrigins)
}

func mergeAcceptHttp10(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.acceptHttp10, p2.acceptHttp10, opts) {
		return
	}

	p1.acceptHttp10 = p2.acceptHttp10
	mergeOrigins.SetOne(origin+"acceptHttp10", p2Ref, p2MergeOrigins)
}

func mergeDefaultHostForHttp10(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.defaultHostForHttp10, p2.defaultHostForHttp10, opts) {
		return
	}

	p1.defaultHostForHttp10 = p2.defaultHostForHttp10
	mergeOrigins.SetOne(origin+"defaultHostForHttp10", p2Ref, p2MergeOrigins)
}

func mergeXffNumTrustedHops(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.xffNumTrustedHops, p2.xffNumTrustedHops, opts) {
		return
	}

	p1.xffNumTrustedHops = p2.xffNumTrustedHops
	mergeOrigins.SetOne(origin+"xffNumTrustedHops", p2Ref, p2MergeOrigins)
}

func mergeServerHeaderTransformation(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.serverHeaderTransformation, p2.serverHeaderTransformation, opts) {
		return
	}

	p1.serverHeaderTransformation = p2.serverHeaderTransformation
	mergeOrigins.SetOne(origin+"serverHeaderTransformation", p2Ref, p2MergeOrigins)
}

func mergeStreamIdleTimeout(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.streamIdleTimeout, p2.streamIdleTimeout, opts) {
		return
	}

	p1.streamIdleTimeout = p2.streamIdleTimeout
	mergeOrigins.SetOne(origin+"mergeStreamIdleTimeout", p2Ref, p2MergeOrigins)
}

func mergeIdleTimeout(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.idleTimeout, p2.idleTimeout, opts) {
		return
	}

	p1.idleTimeout = p2.idleTimeout
	mergeOrigins.SetOne(origin+"mergeIdleTimeout", p2Ref, p2MergeOrigins)
}

func mergeHealthCheckPolicy(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.healthCheckPolicy, p2.healthCheckPolicy, opts) {
		return
	}

	p1.healthCheckPolicy = p2.healthCheckPolicy
	mergeOrigins.SetOne(origin+"healthCheckPolicy", p2Ref, p2MergeOrigins)
}

func mergeEarlyHeaderMutation(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.earlyHeaderMutationExtensions, p2.earlyHeaderMutationExtensions, opts) {
		return
	}

	p1.earlyHeaderMutationExtensions = slices.Clone(p2.earlyHeaderMutationExtensions)
	mergeOrigins.SetOne(origin+"earlyHeaderMutationExtensions", p2Ref, p2MergeOrigins)
}

func mergeMaxRequestHeadersKb(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.maxRequestHeadersKb, p2.maxRequestHeadersKb, opts) {
		return
	}

	p1.maxRequestHeadersKb = p2.maxRequestHeadersKb
	mergeOrigins.SetOne(origin+"maxRequestHeadersKb", p2Ref, p2MergeOrigins)
}

func mergeUuidRequestIdConfig(
	origin string,
	p1, p2 *HttpListenerPolicyIr,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins ir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins ir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.uuidRequestIdConfig, p2.uuidRequestIdConfig, opts) {
		return
	}

	p1.uuidRequestIdConfig = p2.uuidRequestIdConfig
	mergeOrigins.SetOne(origin+"uuidRequestIdConfig", p2Ref, p2MergeOrigins)
}
