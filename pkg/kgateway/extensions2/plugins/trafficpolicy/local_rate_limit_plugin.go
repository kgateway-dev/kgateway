package trafficpolicy

import (
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ratelimitcommonv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/ratelimit/v3"
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
func constructLocalRateLimit(in *kgateway.TrafficPolicy, out *trafficPolicySpecIr) {
	if in.Spec.RateLimit == nil || in.Spec.RateLimit.Local == nil {
		return
	}
	localRateLimit := toLocalRateLimitFilterConfig(in.Spec.RateLimit.Local)
	out.localRateLimit = &localRateLimitIR{
		config: localRateLimit,
	}
}

func toLocalRateLimitFilterConfig(t *kgateway.LocalRateLimitPolicy) *localratelimitv3.LocalRateLimit {
	if t == nil {
		return nil
	}

	// If the local rate limit policy is empty, we add a LocalRateLimit configuration that disables
	// any other applied local rate limit policy (if any) for the target.
	// LocalRateLimitPolicy contains a slice field so it cannot be compared with '=='.
	if t.TokenBucket == nil && t.PercentEnabled == nil && t.PercentEnforced == nil && len(t.Descriptors) == 0 {
		return createDisabledRateLimit()
	}

	tokenBucket := &typev3.TokenBucket{}
	if t.TokenBucket != nil {
		tokenBucket.FillInterval = durationpb.New(t.TokenBucket.FillInterval.Duration)
		tokenBucket.MaxTokens = uint32(t.TokenBucket.MaxTokens) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
		if t.TokenBucket.TokensPerFill != nil {
			tokenBucket.TokensPerFill = wrapperspb.UInt32(uint32(*t.TokenBucket.TokensPerFill)) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
		}
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
		// By default filter is enabled for 0% of the requests.
		FilterEnabled: &envoycorev3.RuntimeFractionalPercent{
			RuntimeKey: localRatelimitFilterEnabledRuntimeKey,
			DefaultValue: &typev3.FractionalPercent{
				Numerator:   filterEnabled,
				Denominator: typev3.FractionalPercent_HUNDRED,
			},
		},
		// By default filter is enforced for 0% of the requests (out of the enabled fraction).
		FilterEnforced: &envoycorev3.RuntimeFractionalPercent{
			RuntimeKey: localRatelimitFilterEnforcedRuntimeKey,
			DefaultValue: &typev3.FractionalPercent{
				Numerator:   filterEnforced,
				Denominator: typev3.FractionalPercent_HUNDRED,
			},
		},
		Descriptors: toLocalRateLimitDescriptors(t.Descriptors),
	}

	return lrl
}

// toLocalRateLimitDescriptors translates API descriptors to Envoy LocalRateLimitDescriptor protos.
// Each proto specifies the key-value entries to match against and the token bucket to apply
// when a request's generated descriptor entries match.
func toLocalRateLimitDescriptors(descriptors []kgateway.LocalRateLimitDescriptor) []*ratelimitcommonv3.LocalRateLimitDescriptor {
	if len(descriptors) == 0 {
		return nil
	}
	result := make([]*ratelimitcommonv3.LocalRateLimitDescriptor, 0, len(descriptors))
	for _, d := range descriptors {
		entries := make([]*ratelimitcommonv3.RateLimitDescriptor_Entry, 0, len(d.Entries))
		for _, e := range d.Entries {
			entries = append(entries, &ratelimitcommonv3.RateLimitDescriptor_Entry{
				Key:   e.Key,
				Value: e.Value,
			})
		}
		tb := &typev3.TokenBucket{
			MaxTokens:    uint32(d.TokenBucket.MaxTokens), // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
			FillInterval: durationpb.New(d.TokenBucket.FillInterval.Duration),
		}
		if d.TokenBucket.TokensPerFill != nil {
			tb.TokensPerFill = wrapperspb.UInt32(uint32(*d.TokenBucket.TokensPerFill)) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
		}
		result = append(result, &ratelimitcommonv3.LocalRateLimitDescriptor{
			Entries:     entries,
			TokenBucket: tb,
		})
	}
	return result
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
