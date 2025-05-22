package trafficpolicy

import (
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

const (
	localRatelimitFilterEnabledRuntimeKey  = "local_rate_limit_enabled"
	localRatelimitFilterEnforcedRuntimeKey = "local_rate_limit_enforced"
	localRatelimitFilterDisabledRuntimeKey = "local_rate_limit_disabled"
)

func toLocalRateLimitFilterConfig(t *v1alpha1.LocalRateLimitPolicy) (*localratelimitv3.LocalRateLimit, error) {
	if t == nil {
		return nil, nil
	}

	// If the local rate limit policy is empty, we add a LocalRateLimit configuration that disables
	// any other applied local rate limit policy (if any) for the target.
	if *t == (v1alpha1.LocalRateLimitPolicy{}) {
		return createDisabledRateLimit(), nil
	}

	tokenBucket := &typev3.TokenBucket{}
	if t.TokenBucket != nil {
		fillInterval, err := time.ParseDuration(string(t.TokenBucket.FillInterval))
		if err != nil {
			return nil, err
		}
		tokenBucket.FillInterval = durationpb.New(fillInterval)
		tokenBucket.MaxTokens = t.TokenBucket.MaxTokens
		if t.TokenBucket.TokensPerFill != nil {
			tokenBucket.TokensPerFill = wrapperspb.UInt32(*t.TokenBucket.TokensPerFill)
		}
	}

	var lrl *localratelimitv3.LocalRateLimit = &localratelimitv3.LocalRateLimit{
		StatPrefix:  localRateLimitStatPrefix,
		TokenBucket: tokenBucket,
		// By default filter is enabled for 0% of the requests. We enable it for all requests.
		// TODO: Make this configurable in the rate limit policy API.
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			RuntimeKey: localRatelimitFilterEnabledRuntimeKey,
			DefaultValue: &typev3.FractionalPercent{
				Numerator:   100,
				Denominator: typev3.FractionalPercent_HUNDRED,
			},
		},
		// By default filter is enforced for 0% of the requests (out of the enabled fraction).
		// We enable it for all requests.
		// TODO: Make this configurable in the rate limit policy API.
		FilterEnforced: &corev3.RuntimeFractionalPercent{
			RuntimeKey: localRatelimitFilterEnforcedRuntimeKey,
			DefaultValue: &typev3.FractionalPercent{
				Numerator:   100,
				Denominator: typev3.FractionalPercent_HUNDRED,
			},
		},
	}

	return lrl, nil
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
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			RuntimeKey:   localRatelimitFilterDisabledRuntimeKey,
			DefaultValue: &typev3.FractionalPercent{},
		},
	}
}
