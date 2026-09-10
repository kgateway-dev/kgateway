package trafficpolicy

import (
	"fmt"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/ratelimit/v3"
	localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	localRatelimitFilterEnabledRuntimeKey  = "local_rate_limit_enabled"
	localRatelimitFilterEnforcedRuntimeKey = "local_rate_limit_enforced"
	localRatelimitFilterDisabledRuntimeKey = "local_rate_limit_disabled"
)

type localRateLimitIR struct {
	config *localratelimitv3.LocalRateLimit
}

var _ PolicySubIR = &localRateLimitIR{}

func (l *localRateLimitIR) Equals(other PolicySubIR) bool {
	otherLocalRateLimit, ok := other.(*localRateLimitIR)
	if !ok {
		return false
	}
	if l == nil && otherLocalRateLimit == nil {
		return true
	}
	if l == nil || otherLocalRateLimit == nil {
		return false
	}
	return proto.Equal(l.config, otherLocalRateLimit.config)
}

func (l *localRateLimitIR) Validate() error {
	if l == nil || l.config == nil {
		return nil
	}
	return l.config.ValidateAll()
}

// constructLocalRateLimit constructs the local rate limit policy IR from the policy specification.
func constructLocalRateLimit(in *kgateway.TrafficPolicy, out *trafficPolicySpecIr) error {
	if in.Spec.RateLimit == nil || in.Spec.RateLimit.Local == nil {
		return nil
	}
	localRateLimit, err := toLocalRateLimitFilterConfig(in.Spec.RateLimit.Local)
	if err != nil {
		return err
	}
	out.localRateLimit = &localRateLimitIR{
		config: localRateLimit,
	}
	return nil
}

func toLocalRateLimitFilterConfig(t *kgateway.LocalRateLimitPolicy) (*localratelimitv3.LocalRateLimit, error) {
	if t == nil {
		return nil, nil
	}

	// If the local rate limit policy is empty, we add a LocalRateLimit configuration that disables
	// any other applied local rate limit policy (if any) for the target.
	if isEmptyLocalRateLimitPolicy(t) {
		return createDisabledRateLimit(), nil
	}

	if len(t.Descriptors) > 0 && t.TokenBucket == nil {
		return nil, fmt.Errorf("local rate limit descriptors require the default token bucket to be set")
	}

	tokenBucket := toEnvoyTokenBucket(t.TokenBucket)
	descriptors, rateLimits, err := createLocalRateLimitDescriptors(t.Descriptors, t.TokenBucket)
	if err != nil {
		return nil, err
	}

	filterEnabled := uint32(100)
	if t.PercentEnabled != nil {
		filterEnabled = uint32(*t.PercentEnabled) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}

	filterEnforced := uint32(100)
	if t.PercentEnforced != nil {
		filterEnforced = uint32(*t.PercentEnforced) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}

	lrl := &localratelimitv3.LocalRateLimit{
		StatPrefix:  localRateLimitStatPrefix,
		TokenBucket: tokenBucket,
		Descriptors: descriptors,
		RateLimits:  rateLimits,
		// Enable the filter for all requests unless the policy specifies another percentage.
		FilterEnabled: &envoycorev3.RuntimeFractionalPercent{
			RuntimeKey: localRatelimitFilterEnabledRuntimeKey,
			DefaultValue: &typev3.FractionalPercent{
				Numerator:   filterEnabled,
				Denominator: typev3.FractionalPercent_HUNDRED,
			},
		},
		// Enforce the filter for all enabled requests unless the policy specifies another percentage.
		FilterEnforced: &envoycorev3.RuntimeFractionalPercent{
			RuntimeKey: localRatelimitFilterEnforcedRuntimeKey,
			DefaultValue: &typev3.FractionalPercent{
				Numerator:   filterEnforced,
				Denominator: typev3.FractionalPercent_HUNDRED,
			},
		},
	}
	if t.AlwaysConsumeDefaultTokenBucket != nil {
		lrl.AlwaysConsumeDefaultTokenBucket = wrapperspb.Bool(*t.AlwaysConsumeDefaultTokenBucket)
	}
	if t.MaxDynamicDescriptors != nil {
		lrl.MaxDynamicDescriptors = wrapperspb.UInt32(uint32(*t.MaxDynamicDescriptors)) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}

	return lrl, nil
}

func isEmptyLocalRateLimitPolicy(policy *kgateway.LocalRateLimitPolicy) bool {
	return policy.TokenBucket == nil &&
		policy.PercentEnabled == nil &&
		policy.PercentEnforced == nil &&
		len(policy.Descriptors) == 0 &&
		policy.AlwaysConsumeDefaultTokenBucket == nil &&
		policy.MaxDynamicDescriptors == nil
}

func toEnvoyTokenBucket(tokenBucket *kgateway.TokenBucket) *typev3.TokenBucket {
	if tokenBucket == nil {
		return nil
	}

	out := &typev3.TokenBucket{
		FillInterval: durationpb.New(tokenBucket.FillInterval.Duration),
		MaxTokens:    uint32(tokenBucket.MaxTokens), // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if tokenBucket.TokensPerFill != nil {
		out.TokensPerFill = wrapperspb.UInt32(uint32(*tokenBucket.TokensPerFill)) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	return out
}

func createLocalRateLimitDescriptors(
	descriptors []kgateway.LocalRateLimitDescriptor,
	defaultTokenBucket *kgateway.TokenBucket,
) ([]*ratelimitv3.LocalRateLimitDescriptor, []*envoyroutev3.RateLimit, error) {
	if len(descriptors) == 0 {
		return nil, nil, nil
	}

	localDescriptors := make([]*ratelimitv3.LocalRateLimitDescriptor, 0, len(descriptors))
	rateLimits := make([]*envoyroutev3.RateLimit, 0, len(descriptors))
	seenDescriptors := make(map[string]struct{}, len(descriptors))
	for descriptorIndex, descriptor := range descriptors {
		if len(descriptor.Entries) == 0 {
			return nil, nil, fmt.Errorf("local rate limit descriptor %d must contain at least one entry", descriptorIndex)
		}
		if defaultTokenBucket != nil &&
			defaultTokenBucket.FillInterval.Duration > 0 &&
			descriptor.TokenBucket.FillInterval.Duration%defaultTokenBucket.FillInterval.Duration != 0 {
			return nil, nil, fmt.Errorf(
				"local rate limit descriptor %d fill interval must be a multiple of the default token bucket fill interval",
				descriptorIndex,
			)
		}

		localDescriptor := &ratelimitv3.LocalRateLimitDescriptor{
			Entries:     make([]*ratelimitv3.RateLimitDescriptor_Entry, 0, len(descriptor.Entries)),
			TokenBucket: toEnvoyTokenBucket(&descriptor.TokenBucket),
		}
		rateLimit := &envoyroutev3.RateLimit{
			Actions: make([]*envoyroutev3.RateLimit_Action, 0, len(descriptor.Entries)),
		}
		for _, entry := range descriptor.Entries {
			action, localEntry, err := translateRateLimitDescriptorEntry(entry)
			if err != nil {
				return nil, nil, fmt.Errorf("local rate limit descriptor %d: %w", descriptorIndex, err)
			}
			localDescriptor.Entries = append(localDescriptor.Entries, localEntry)
			rateLimit.Actions = append(rateLimit.Actions, action)
		}

		descriptorKey, err := proto.MarshalOptions{Deterministic: true}.Marshal(&ratelimitv3.RateLimitDescriptor{
			Entries: localDescriptor.Entries,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("marshal local rate limit descriptor %d: %w", descriptorIndex, err)
		}
		if _, found := seenDescriptors[string(descriptorKey)]; found {
			return nil, nil, fmt.Errorf("local rate limit descriptor %d duplicates an earlier descriptor", descriptorIndex)
		}
		seenDescriptors[string(descriptorKey)] = struct{}{}

		localDescriptors = append(localDescriptors, localDescriptor)
		rateLimits = append(rateLimits, rateLimit)
	}

	return localDescriptors, rateLimits, nil
}

// createDisabledRateLimit returns a LocalRateLimit configuration that disables rate limiting.
// This is used when an empty policy is provided to override any existing rate limit configuration.
func createDisabledRateLimit() *localratelimitv3.LocalRateLimit {
	return &localratelimitv3.LocalRateLimit{
		StatPrefix: localRateLimitStatPrefix,
		// Config per route requires a token bucket, so we create a minimal one
		TokenBucket: &typev3.TokenBucket{
			MaxTokens:    1,
			FillInterval: durationpb.New(1),
		},
		// Set filter enabled to 0% to effectively disable rate limiting
		FilterEnabled: &envoycorev3.RuntimeFractionalPercent{
			RuntimeKey:   localRatelimitFilterDisabledRuntimeKey,
			DefaultValue: &typev3.FractionalPercent{},
		},
	}
}

func (p *trafficPolicyPluginGwPass) handleLocalRateLimit(fcn string, typedFilterConfig *ir.TypedFilterConfigMap, localRateLimit *localRateLimitIR) {
	if localRateLimit == nil {
		return
	}
	typedFilterConfig.AddTypedConfig(localRateLimitFilterNamePrefix, localRateLimit.config)

	// Add a filter to the chain. When having a rate limit for a route we need to also have a
	// globally disabled rate limit filter in the chain otherwise it will be ignored.
	// If there is also rate limit for the listener, it will not override this one.
	if p.localRateLimitInChain == nil {
		p.localRateLimitInChain = make(map[string]*localratelimitv3.LocalRateLimit)
	}
	if _, ok := p.localRateLimitInChain[fcn]; !ok {
		p.localRateLimitInChain[fcn] = &localratelimitv3.LocalRateLimit{
			StatPrefix: localRateLimitStatPrefix,
		}
	}
}
