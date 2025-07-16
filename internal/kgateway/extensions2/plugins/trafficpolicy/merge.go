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
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.spec.AI, p2.spec.AI, opts) {
		return
	}

	switch opts.Strategy {
	case policy.AugmentedDeepMerge, policy.OverridableDeepMerge:
		if p1.spec.AI != nil {
			return
		}
		fallthrough // can override p1 if it is unset

	case policy.AugmentedShallowMerge, policy.OverridableShallowMerge:
		p1.spec.AI = p2.spec.AI
		mergeOrigins.SetOne("ai", p2Ref)

	default:
		logger.Warn("unsupported merge strategy for ai policy", "strategy", opts.Strategy, "policy", p2Ref)
	}
}

func mergeExtProc(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.spec.ExtProc, p2.spec.ExtProc, opts) {
		return
	}

	switch opts.Strategy {
	case policy.AugmentedDeepMerge, policy.OverridableDeepMerge:
		if p1.spec.ExtProc != nil {
			return
		}
		fallthrough // can override p1 if it is unset

	case policy.AugmentedShallowMerge, policy.OverridableShallowMerge:
		p1.spec.ExtProc = p2.spec.ExtProc
		mergeOrigins.SetOne("extProc", p2Ref)

	default:
		logger.Warn("unsupported merge strategy for extProc policy", "strategy", opts.Strategy, "policy", p2Ref)
	}
}

func mergeTransformation(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.spec.transform, p2.spec.transform, opts) {
		return
	}

	switch opts.Strategy {
	case policy.AugmentedShallowMerge, policy.OverridableShallowMerge:
		if p1.spec.transform == nil {
			p1.spec.transform = &transformationpb.RouteTransformations{}
		}
		// Always clone so that the original policy in p2 is not modified when
		// the merge is invoked multiple times
		p1.spec.transform.Transformations = slices.Clone(p2.spec.transform.GetTransformations())
		mergeOrigins.SetOne("transformation", p2Ref)

	case policy.AugmentedDeepMerge:
		if p1.spec.transform == nil {
			p1.spec.transform = &transformationpb.RouteTransformations{}
		}
		// Always Concat so that the original policy in p1 is not modified when
		// the merge is invoked multiple times
		p1.spec.transform.Transformations = slices.Concat(p1.spec.transform.GetTransformations(), p2.spec.transform.GetTransformations())
		mergeOrigins.Append("transformation", p2Ref)

	case policy.OverridableDeepMerge:
		if p1.spec.transform == nil {
			p1.spec.transform = &transformationpb.RouteTransformations{}
		}
		// Always Concat so that the original policy in p1/p2 is not modified when
		// the merge is invoked multiple times
		p1.spec.transform.Transformations = slices.Concat(p2.spec.transform.GetTransformations(), p1.spec.transform.GetTransformations())
		mergeOrigins.Append("transformation", p2Ref)

	default:
		logger.Warn("unsupported merge strategy for transformation policy", "strategy", opts.Strategy, "policy", p2Ref)
	}
}

func mergeRustformation(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.spec.rustformation, p2.spec.rustformation, opts) {
		return
	}

	switch opts.Strategy {
	case policy.AugmentedDeepMerge, policy.OverridableDeepMerge:
		if p1.spec.rustformation != nil {
			return
		}
		fallthrough // can override p1 if it is unset

	case policy.AugmentedShallowMerge, policy.OverridableShallowMerge:
		p1.spec.rustformation = p2.spec.rustformation
		p1.spec.rustformationStringToStash = p2.spec.rustformationStringToStash
		mergeOrigins.SetOne("rustformation", p2Ref)

	default:
		logger.Warn("unsupported merge strategy for rustformation policy", "strategy", opts.Strategy, "policy", p2Ref)
	}
}

func mergeExtAuth(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.spec.extAuth, p2.spec.extAuth, opts) {
		return
	}

	switch opts.Strategy {
	case policy.AugmentedDeepMerge, policy.OverridableDeepMerge:
		if p1.spec.extAuth != nil {
			return
		}
		fallthrough // can override p1 if it is unset

	case policy.AugmentedShallowMerge, policy.OverridableShallowMerge:
		p1.spec.extAuth = p2.spec.extAuth
		mergeOrigins.SetOne("extAuth", p2Ref)

	default:
		logger.Warn("unsupported merge strategy for extAuth policy", "strategy", opts.Strategy, "policy", p2Ref)
	}
}

func mergeLocalRateLimit(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.spec.localRateLimit, p2.spec.localRateLimit, opts) {
		return
	}

	switch opts.Strategy {
	case policy.AugmentedDeepMerge, policy.OverridableDeepMerge:
		if p1.spec.localRateLimit != nil {
			return
		}
		fallthrough // can override p1 if it is unset

	case policy.AugmentedShallowMerge, policy.OverridableShallowMerge:
		p1.spec.localRateLimit = p2.spec.localRateLimit
		mergeOrigins.SetOne("rateLimit.local", p2Ref)

	default:
		logger.Warn("unsupported merge strategy for localRateLimit policy", "strategy", opts.Strategy, "policy", p2Ref)
	}
}

func mergeGlobalRateLimit(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.spec.rateLimit, p2.spec.rateLimit, opts) {
		return
	}

	switch opts.Strategy {
	case policy.AugmentedDeepMerge, policy.OverridableDeepMerge:
		if p1.spec.rateLimit != nil {
			return
		}
		fallthrough // can override p1 if it is unset

	case policy.AugmentedShallowMerge, policy.OverridableShallowMerge:
		p1.spec.rateLimit = p2.spec.rateLimit
		mergeOrigins.SetOne("rateLimit.global", p2Ref)

	default:
		logger.Warn("unsupported merge strategy for rateLimit policy", "strategy", opts.Strategy, "policy", p2Ref)
	}
}

func mergeCORS(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.spec.cors, p2.spec.cors, opts) {
		return
	}

	switch opts.Strategy {
	case policy.AugmentedDeepMerge, policy.OverridableDeepMerge:
		if p1.spec.cors != nil {
			return
		}
		fallthrough // can override p1 if it is unset

	case policy.AugmentedShallowMerge, policy.OverridableShallowMerge:
		p1.spec.cors = p2.spec.cors
		mergeOrigins.SetOne("cors", p2Ref)

	default:
		logger.Warn("unsupported merge strategy for cors policy", "strategy", opts.Strategy, "policy", p2Ref)
	}
}

func mergeCSRF(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.spec.csrf, p2.spec.csrf, opts) {
		return
	}

	switch opts.Strategy {
	case policy.AugmentedDeepMerge, policy.OverridableDeepMerge:
		if p1.spec.csrf != nil {
			return
		}
		fallthrough // can override p1 if it is unset

	case policy.AugmentedShallowMerge, policy.OverridableShallowMerge:
		p1.spec.csrf = p2.spec.csrf
		mergeOrigins.SetOne("csrf", p2Ref)

	default:
		logger.Warn("unsupported merge strategy for csrf policy", "strategy", opts.Strategy, "policy", p2Ref)
	}
}

func mergeBuffer(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.spec.buffer, p2.spec.buffer, opts) {
		return
	}

	switch opts.Strategy {
	case policy.AugmentedDeepMerge, policy.OverridableDeepMerge:
		if p1.spec.buffer != nil {
			return
		}
		fallthrough // can override p1 if it is unset

	case policy.AugmentedShallowMerge, policy.OverridableShallowMerge:
		p1.spec.buffer = p2.spec.buffer
		mergeOrigins.SetOne("buffer", p2Ref)

	default:
		logger.Warn("unsupported merge strategy for buffer policy", "strategy", opts.Strategy, "policy", p2Ref)
	}
}

func mergeAutoHostRewrite(
	p1, p2 *TrafficPolicy,
	p2Ref *pluginsdkir.AttachedPolicyRef,
	opts policy.MergeOptions,
	mergeOrigins pluginsdkir.MergeOrigins,
) {
	if !policy.IsMergeable(p1.spec.autoHostRewrite, p2.spec.autoHostRewrite, opts) {
		return
	}

	switch opts.Strategy {
	case policy.AugmentedDeepMerge, policy.OverridableDeepMerge:
		if p1.spec.autoHostRewrite != nil {
			return
		}
		fallthrough // can override p1 if it is unset

	case policy.AugmentedShallowMerge, policy.OverridableShallowMerge:
		p1.spec.autoHostRewrite = p2.spec.autoHostRewrite
		mergeOrigins.SetOne("autoHostRewrite", p2Ref)

	default:
		logger.Warn("unsupported merge strategy for AutoHostRewrite policy", "strategy", opts.Strategy, "policy", p2Ref)
	}
}
