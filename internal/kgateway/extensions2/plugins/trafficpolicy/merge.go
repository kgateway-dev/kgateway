package trafficpolicy

import (
	"slices"

	transformationpb "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"

	pluginsdkir "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

func mergeAI(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	accessor := fieldAccessor[aiPolicyIR]{
		Get: func(spec *trafficPolicySpecIr) *aiPolicyIR { return spec.ai },
		Set: func(spec *trafficPolicySpecIr, val *aiPolicyIR) { spec.ai = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "ai")
}

func mergeExtProc(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	accessor := fieldAccessor[extprocIR]{
		Get: func(spec *trafficPolicySpecIr) *extprocIR { return spec.extProc },
		Set: func(spec *trafficPolicySpecIr, val *extprocIR) { spec.extProc = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "extProc")
}

func mergeTransformation(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.spec.transformation, p2.spec.transformation, opts) {
		return
	}

	switch opts.Strategy {
	case policy.AugmentedShallowMerge, policy.OverridableShallowMerge:
		if p1.spec.transformation == nil {
			p1.spec.transformation = &transformationIR{config: &transformationpb.RouteTransformations{}}
		}
		// Always clone so that the original policy in p2 is not modified when
		// the merge is invoked multiple times
		p1.spec.transformation.config.Transformations = slices.Clone(p2.spec.transformation.config.GetTransformations())
		mergeOrigins.SetOne("transformation", p2Ref, p2MergeOrigins)

	case policy.AugmentedDeepMerge:
		if p1.spec.transformation == nil {
			p1.spec.transformation = &transformationIR{config: &transformationpb.RouteTransformations{}}
		}
		// Always Concat so that the original policy in p1 is not modified when
		// the merge is invoked multiple times
		p1.spec.transformation.config.Transformations = slices.Concat(p1.spec.transformation.config.GetTransformations(), p2.spec.transformation.config.GetTransformations())
		mergeOrigins.Append("transformation", p2Ref, p2MergeOrigins)

	case policy.OverridableDeepMerge:
		if p1.spec.transformation == nil {
			p1.spec.transformation = &transformationIR{config: &transformationpb.RouteTransformations{}}
		}
		// Always Concat so that the original policy in p1/p2 is not modified when
		// the merge is invoked multiple times
		p1.spec.transformation.config.Transformations = slices.Concat(p2.spec.transformation.config.GetTransformations(), p1.spec.transformation.config.GetTransformations())
		mergeOrigins.Append("transformation", p2Ref, p2MergeOrigins)

	default:
		logger.Warn("unsupported merge strategy for transformation policy", "strategy", opts.Strategy, "policy", p2Ref)
	}
}

func mergeRustformation(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	accessor := fieldAccessor[rustformationIR]{
		Get: func(spec *trafficPolicySpecIr) *rustformationIR { return spec.rustformation },
		Set: func(spec *trafficPolicySpecIr, val *rustformationIR) { spec.rustformation = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "rustformation")
}

func mergeExtAuth(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	accessor := fieldAccessor[extAuthIR]{
		Get: func(spec *trafficPolicySpecIr) *extAuthIR { return spec.extAuth },
		Set: func(spec *trafficPolicySpecIr, val *extAuthIR) { spec.extAuth = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "extAuth")
}

func mergeLocalRateLimit(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	accessor := fieldAccessor[localRateLimitIR]{
		Get: func(spec *trafficPolicySpecIr) *localRateLimitIR { return spec.localRateLimit },
		Set: func(spec *trafficPolicySpecIr, val *localRateLimitIR) { spec.localRateLimit = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "rateLimit.local")
}

func mergeGlobalRateLimit(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	accessor := fieldAccessor[globalRateLimitIR]{
		Get: func(spec *trafficPolicySpecIr) *globalRateLimitIR { return spec.globalRateLimit },
		Set: func(spec *trafficPolicySpecIr, val *globalRateLimitIR) { spec.globalRateLimit = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "rateLimit.global")
}

func mergeCORS(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	accessor := fieldAccessor[corsIR]{
		Get: func(spec *trafficPolicySpecIr) *corsIR { return spec.cors },
		Set: func(spec *trafficPolicySpecIr, val *corsIR) { spec.cors = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "cors")
}

func mergeCSRF(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	accessor := fieldAccessor[csrfIR]{
		Get: func(spec *trafficPolicySpecIr) *csrfIR { return spec.csrf },
		Set: func(spec *trafficPolicySpecIr, val *csrfIR) { spec.csrf = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "csrf")
}

func mergeBuffer(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	accessor := fieldAccessor[bufferIR]{
		Get: func(spec *trafficPolicySpecIr) *bufferIR { return spec.buffer },
		Set: func(spec *trafficPolicySpecIr, val *bufferIR) { spec.buffer = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "buffer")
}

func mergeAutoHostRewrite(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	accessor := fieldAccessor[autoHostRewriteIR]{
		Get: func(spec *trafficPolicySpecIr) *autoHostRewriteIR { return spec.autoHostRewrite },
		Set: func(spec *trafficPolicySpecIr, val *autoHostRewriteIR) { spec.autoHostRewrite = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "autoHostRewrite")
}

func mergeHashPolicies(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	accessor := fieldAccessor[hashPolicyIR]{
		Get: func(spec *trafficPolicySpecIr) *hashPolicyIR { return spec.hashPolicies },
		Set: func(spec *trafficPolicySpecIr, val *hashPolicyIR) { spec.hashPolicies = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "hashPolicies")
}

func mergeTimeouts(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	accessor := fieldAccessor[timeoutsIR]{
		Get: func(spec *trafficPolicySpecIr) *timeoutsIR { return spec.timeouts },
		Set: func(spec *trafficPolicySpecIr, val *timeoutsIR) { spec.timeouts = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "timeouts")
}

func mergeRetry(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	accessor := fieldAccessor[retryIR]{
		Get: func(spec *trafficPolicySpecIr) *retryIR { return spec.retry },
		Set: func(spec *trafficPolicySpecIr, val *retryIR) { spec.retry = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "retry")
}

// fieldAccessor defines how to access and set a field on trafficPolicySpecIr
type fieldAccessor[T any] struct {
	Get func(*trafficPolicySpecIr) *T
	Set func(*trafficPolicySpecIr, *T)
}

// defaultMerge is a generic merge function that can handle any field on TrafficPolicy.spec.
// It should be used when the policy being merged does not support deep merging or custom merge logic.
func defaultMerge[T any](
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
	accessor fieldAccessor[T],
	fieldName string,
) {
	p1Field := accessor.Get(&p1.spec)
	p2Field := accessor.Get(&p2.spec)

	if !policy.IsMergeable(p1Field, p2Field, opts) {
		return
	}

	switch opts.Strategy {
	case policy.AugmentedDeepMerge, policy.OverridableDeepMerge:
		if p1Field != nil {
			return
		}
		fallthrough // can override p1 if it is unset

	case policy.AugmentedShallowMerge, policy.OverridableShallowMerge:
		accessor.Set(&p1.spec, p2Field)
		mergeOrigins.SetOne(fieldName, p2Ref, p2MergeOrigins)

	default:
		logger.Warn("unsupported merge strategy for policy", "strategy", opts.Strategy, "policy", p2Ref, "field", fieldName)
	}
}
