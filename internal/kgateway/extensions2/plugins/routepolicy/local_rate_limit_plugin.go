package routepolicy

import (
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func toLocalRateLimitFilterConfig(t *v1alpha1.LocalRateLimitPolicy) (*localratelimitv3.LocalRateLimit, error) {
	if t == nil || *t == (v1alpha1.LocalRateLimitPolicy{}) {
		return nil, nil
	}

	fillInterval, err := time.ParseDuration(t.TokenBucket.FillInterval)
	if err != nil {
		return nil, err
	}

	var lrl *localratelimitv3.LocalRateLimit = &localratelimitv3.LocalRateLimit{
		StatPrefix: localRateLimitStatPrefix,
		TokenBucket: &typev3.TokenBucket{
			MaxTokens:     t.TokenBucket.MaxTokens,
			TokensPerFill: wrapperspb.UInt32(t.TokenBucket.TokensPerFill),
			FillInterval:  durationpb.New(fillInterval),
		},
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			RuntimeKey: "local_rate_limit_enabled",
			DefaultValue: &typev3.FractionalPercent{
				Numerator:   100,
				Denominator: typev3.FractionalPercent_HUNDRED,
			},
		},
		FilterEnforced: &corev3.RuntimeFractionalPercent{
			RuntimeKey: "local_rate_limit_enforced",
			DefaultValue: &typev3.FractionalPercent{
				Numerator:   100,
				Denominator: typev3.FractionalPercent_HUNDRED,
			},
		},
	}

	return lrl, nil
}
