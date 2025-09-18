package trafficpolicy

import (
	"encoding/json"
	"slices"

	transformationpb "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	pluginsdkir "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

type mergeOpts struct {
	TrafficPolicy TrafficPolicyMergeOpts `json:"trafficPolicy,omitempty"`
}

type TrafficPolicyMergeOpts struct {
	ExtAuth string `json:"extAuth,omitempty"`

	ExtProc string `json:"extProc,omitempty"`

	Transformation string `json:"transformation,omitempty"`
}

// MergeTrafficPolicies merges two TrafficPolicy IRs, returning a map that contains information
// about the origin policy reference for each merged field.
func MergeTrafficPolicies(
	p1, p2 *TrafficPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
	tpOpts TrafficPolicyMergeOpts,
) {
	if p1 == nil || p2 == nil {
		return
	}

	mergeFuncs := []func(*TrafficPolicy, *TrafficPolicy, *ir.AttachedPolicyRef, pluginsdkir.MergeOrigins, policy.MergeOptions, pluginsdkir.MergeOrigins, TrafficPolicyMergeOpts){
		mergeAI,
		mergeExtProc,
		mergeTransformation,
		mergeRustformation,
		mergeExtAuth,
		mergeLocalRateLimit,
		mergeGlobalRateLimit,
		mergeCORS,
		mergeCSRF,
		mergeHeaderModifiers,
		mergeBuffer,
		mergeAutoHostRewrite,
		mergeTimeouts,
		mergeRetry,
		mergeRBAC,
	}

	for _, mergeFunc := range mergeFuncs {
		mergeFunc(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, tpOpts)
	}
}

func mergeTrafficPolicies(
	p1, p2 *TrafficPolicy,
	p2Ref *ir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
	mergeSettingsJSON string,
) {
	var polMergeOpts mergeOpts
	if mergeSettingsJSON != "" {
		err := json.Unmarshal([]byte(mergeSettingsJSON), &polMergeOpts)
		if err != nil {
			logger.Error("error parsing merge settings; skipping merge", "value", mergeSettingsJSON, "error", err)
			return
		}
	}

	MergeTrafficPolicies(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, polMergeOpts.TrafficPolicy)
}

func mergeAI(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
	_ TrafficPolicyMergeOpts,
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
	tpOpts TrafficPolicyMergeOpts,
) {
	accessor := fieldAccessor[extprocIR]{
		Get: func(spec *trafficPolicySpecIr) *extprocIR { return spec.extProc },
		Set: func(spec *trafficPolicySpecIr, val *extprocIR) { spec.extProc = val },
	}

	if tpOpts.ExtProc != "" {
		// this is merging 2 policies at the same hierarchical level (no parent->child relationship),
		// so use mergeOpts since it overrides the default merge strategy
		opts.Strategy = policy.ToInternalMergeStrategy(tpOpts.ExtProc)
	}
	if !policy.IsMergeable(p1.spec.extProc, p2.spec.extProc, opts) {
		return
	}

	switch opts.Strategy {
	case policy.AugmentedDeepMerge:
		if p1.spec.extProc == nil {
			p1.spec.extProc = &extprocIR{}
		}
		// p2 will always have just 1 item in its providerNames, and if p1 contains that then
		// it implies that this provider was already considered from a higher priority policy,
		// so ignore it
		if p2.spec.extProc.providerNames.Len() > 0 && !p1.spec.extProc.providerNames.IsSuperset(p2.spec.extProc.providerNames) {
			// Always Concat so that the original slice in the IR is never modified
			// Note: p1 is preferred over p2 (slice order)
			p1.spec.extProc.perProviderConfig = slices.Concat(p1.spec.extProc.perProviderConfig, p2.spec.extProc.perProviderConfig)
			// Always Clone so that the original slice in the IR is never modified
			tmp := p1.spec.extProc.providerNames.Clone()
			tmp.Insert(p2.spec.extProc.providerNames.UnsortedList()...)
			p1.spec.extProc.providerNames = tmp
			mergeOrigins.Append("extProc", p2Ref, p2MergeOrigins)
		}
		if p2.spec.extProc.disableAllProviders {
			p1.spec.extProc.disableAllProviders = true
			mergeOrigins.SetOne("extProc", p2Ref, p2MergeOrigins)
		}

	case policy.OverridableDeepMerge:
		if p1.spec.extProc == nil {
			p1.spec.extProc = &extprocIR{}
		}
		// p2 will always have just 1 item in its providerNames, and if p1 contains that then
		// it implies that this provider was already considered from a higher priority policy,
		// so ignore it
		if p2.spec.extProc.providerNames.Len() > 0 && !p1.spec.extProc.providerNames.IsSuperset(p2.spec.extProc.providerNames) {
			// Always Concat so that the original slice in the IR is never modified
			// Note: p2 is preferred over p1 (slice order)
			p1.spec.extProc.perProviderConfig = slices.Concat(p2.spec.extProc.perProviderConfig, p1.spec.extProc.perProviderConfig)
			// Always Clone so that the original slice in the IR is never modified
			tmp := p1.spec.extProc.providerNames.Clone()
			tmp.Insert(p2.spec.extProc.providerNames.UnsortedList()...)
			p1.spec.extProc.providerNames = tmp
			mergeOrigins.Append("extProc", p2Ref, p2MergeOrigins)
		}
		if p2.spec.extProc.disableAllProviders {
			p1.spec.extProc.disableAllProviders = true
			mergeOrigins.SetOne("extProc", p2Ref, p2MergeOrigins)
		}

	default:
		defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "extProc")
	}
}

func mergeTransformation(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
	tpOpts TrafficPolicyMergeOpts,
) {
	if tpOpts.Transformation != "" {
		// this is merging 2 policies at the same hierarchical level (no parent->child relationship),
		// so use tpOpts since it overrides the default merge strategy
		opts.Strategy = policy.ToInternalMergeStrategy(tpOpts.Transformation)
	}
	if !policy.IsMergeable(p1.spec.transformation, p2.spec.transformation, opts) {
		return
	}

	switch opts.Strategy {
	case policy.AugmentedShallowMerge, policy.OverridableShallowMerge:
		if p1.spec.transformation == nil {
			p1.spec.transformation = &transformationIR{config: &transformationpb.RouteTransformations{}}
		}
		// Always Clone so that the original slice in the IR is never modified
		p1.spec.transformation.config.Transformations = slices.Clone(p2.spec.transformation.config.GetTransformations())
		mergeOrigins.SetOne("transformation", p2Ref, p2MergeOrigins)

	case policy.AugmentedDeepMerge:
		if p1.spec.transformation == nil {
			p1.spec.transformation = &transformationIR{config: &transformationpb.RouteTransformations{}}
		}
		// Always Concat so that the original slice in the IR is never modified
		p1.spec.transformation.config.Transformations = slices.Concat(p1.spec.transformation.config.GetTransformations(), p2.spec.transformation.config.GetTransformations())
		mergeOrigins.Append("transformation", p2Ref, p2MergeOrigins)

	case policy.OverridableDeepMerge:
		if p1.spec.transformation == nil {
			p1.spec.transformation = &transformationIR{config: &transformationpb.RouteTransformations{}}
		}
		// Always Concat so that the original slice in the IR is never modified
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
	_ TrafficPolicyMergeOpts,
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
	tpOpts TrafficPolicyMergeOpts,
) {
	accessor := fieldAccessor[extAuthIR]{
		Get: func(spec *trafficPolicySpecIr) *extAuthIR { return spec.extAuth },
		Set: func(spec *trafficPolicySpecIr, val *extAuthIR) { spec.extAuth = val },
	}

	if tpOpts.ExtAuth != "" {
		// this is merging 2 policies at the same hierarchical level (no parent->child relationship),
		// so use mergeOpts since it overrides the default merge strategy
		opts.Strategy = policy.ToInternalMergeStrategy(tpOpts.ExtAuth)
	}
	if !policy.IsMergeable(p1.spec.extAuth, p2.spec.extAuth, opts) {
		return
	}

	switch opts.Strategy {
	case policy.AugmentedDeepMerge:
		if p1.spec.extAuth == nil {
			p1.spec.extAuth = &extAuthIR{}
		}
		// as p2 is not a merged policy, it will always have just 1 item in its providerNames
		// as each extauth policy can only reference a single provider.
		// If p1 contains the singular provider in p2 then it implies that this provider
		// was already considered from a higher priority policy, so ignore it
		if p2.spec.extAuth.providerNames.Len() > 0 && !p1.spec.extAuth.providerNames.IsSuperset(p2.spec.extAuth.providerNames) {
			// Always Concat so that the original slice in the IR is never modified
			// Note: p1 is preferred over p2 (slice order)
			p1.spec.extAuth.perProviderConfig = slices.Concat(p1.spec.extAuth.perProviderConfig, p2.spec.extAuth.perProviderConfig)
			// Always Clone so that the original slice in the IR is never modified
			tmp := p1.spec.extAuth.providerNames.Clone()
			tmp.Insert(p2.spec.extAuth.providerNames.UnsortedList()...)
			p1.spec.extAuth.providerNames = tmp
			mergeOrigins.Append("extAuth", p2Ref, p2MergeOrigins)
		}
		if p2.spec.extAuth.disableAllProviders {
			p1.spec.extAuth.disableAllProviders = true
			mergeOrigins.SetOne("extAuth", p2Ref, p2MergeOrigins)
		}

	case policy.OverridableDeepMerge:
		if p1.spec.extAuth == nil {
			p1.spec.extAuth = &extAuthIR{}
		}
		// p2 will always have just 1 item in its providerNames, and if p1 contains that then
		// it implies that this provider was already considered from a higher priority policy,
		// so ignore it
		if p2.spec.extAuth.providerNames.Len() > 0 && !p1.spec.extAuth.providerNames.IsSuperset(p2.spec.extAuth.providerNames) {
			// Always Concat so that the original slice in the IR is never modified
			// Note: p2 is preferred over p1 (slice order)
			p1.spec.extAuth.perProviderConfig = slices.Concat(p2.spec.extAuth.perProviderConfig, p1.spec.extAuth.perProviderConfig)
			// Always Clone so that the original slice in the IR is never modified
			tmp := p1.spec.extAuth.providerNames.Clone()
			tmp.Insert(p2.spec.extAuth.providerNames.UnsortedList()...)
			p1.spec.extAuth.providerNames = tmp
			mergeOrigins.Append("extAuth", p2Ref, p2MergeOrigins)
		}
		if p2.spec.extAuth.disableAllProviders {
			p1.spec.extAuth.disableAllProviders = true
			mergeOrigins.SetOne("extAuth", p2Ref, p2MergeOrigins)
		}

	default:
		defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "extAuth")
	}
}

func mergeLocalRateLimit(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
	_ TrafficPolicyMergeOpts,
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
	_ TrafficPolicyMergeOpts,
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
	_ TrafficPolicyMergeOpts,
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
	_ TrafficPolicyMergeOpts,
) {
	accessor := fieldAccessor[csrfIR]{
		Get: func(spec *trafficPolicySpecIr) *csrfIR { return spec.csrf },
		Set: func(spec *trafficPolicySpecIr, val *csrfIR) { spec.csrf = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "csrf")
}

func mergeHeaderModifiers(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
	_ TrafficPolicyMergeOpts,
) {
	accessor := fieldAccessor[headerModifiersIR]{
		Get: func(spec *trafficPolicySpecIr) *headerModifiersIR { return spec.headerModifiers },
		Set: func(spec *trafficPolicySpecIr, val *headerModifiersIR) { spec.headerModifiers = val },
	}

	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "headerModifiers")
}

func mergeBuffer(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
	_ TrafficPolicyMergeOpts,
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
	_ TrafficPolicyMergeOpts,
) {
	accessor := fieldAccessor[autoHostRewriteIR]{
		Get: func(spec *trafficPolicySpecIr) *autoHostRewriteIR { return spec.autoHostRewrite },
		Set: func(spec *trafficPolicySpecIr, val *autoHostRewriteIR) { spec.autoHostRewrite = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "autoHostRewrite")
}

func mergeTimeouts(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
	_ TrafficPolicyMergeOpts,
) {
	accessor := fieldAccessor[timeoutsIR]{
		Get: func(spec *trafficPolicySpecIr) *timeoutsIR { return spec.timeouts },
		Set: func(spec *trafficPolicySpecIr, val *timeoutsIR) { spec.timeouts = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "timeouts")
}

func mergeRBAC(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
	_ TrafficPolicyMergeOpts,
) {
	accessor := fieldAccessor[rbacIR]{
		Get: func(spec *trafficPolicySpecIr) *rbacIR { return spec.rbac },
		Set: func(spec *trafficPolicySpecIr, val *rbacIR) { spec.rbac = val },
	}
	defaultMerge(p1, p2, p2Ref, p2MergeOrigins, opts, mergeOrigins, accessor, "rbac")
}

func mergeRetry(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	p2MergeOrigins pluginsdkir.MergeOrigins,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
	_ TrafficPolicyMergeOpts,
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
