package trafficpolicy

import (
	"testing"
	"time"

	localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestLocalRateLimitIREquals(t *testing.T) {
	createSimpleRateLimit := func(tokensPerSecond uint32) *localratelimitv3.LocalRateLimit {
		return &localratelimitv3.LocalRateLimit{
			TokenBucket: &typev3.TokenBucket{
				MaxTokens:     tokensPerSecond * 10,
				TokensPerFill: wrapperspb.UInt32(tokensPerSecond),
				FillInterval:  durationpb.New(time.Second),
			},
		}
	}
	createRateLimitWithPrefix := func(prefix string) *localratelimitv3.LocalRateLimit {
		return &localratelimitv3.LocalRateLimit{
			StatPrefix: prefix,
		}
	}

	tests := []struct {
		name       string
		rateLimit1 *localRateLimitIR
		rateLimit2 *localRateLimitIR
		expected   bool
	}{
		{
			name:       "both nil are equal",
			rateLimit1: nil,
			rateLimit2: nil,
			expected:   true,
		},
		{
			name:       "nil vs non-nil are not equal",
			rateLimit1: nil,
			rateLimit2: &localRateLimitIR{config: createSimpleRateLimit(100)},
			expected:   false,
		},
		{
			name:       "non-nil vs nil are not equal",
			rateLimit1: &localRateLimitIR{config: createSimpleRateLimit(100)},
			rateLimit2: nil,
			expected:   false,
		},
		{
			name:       "same instance is equal",
			rateLimit1: &localRateLimitIR{config: createSimpleRateLimit(100)},
			rateLimit2: &localRateLimitIR{config: createSimpleRateLimit(100)},
			expected:   true,
		},
		{
			name:       "different token rates are not equal",
			rateLimit1: &localRateLimitIR{config: createSimpleRateLimit(100)},
			rateLimit2: &localRateLimitIR{config: createSimpleRateLimit(200)},
			expected:   false,
		},
		{
			name:       "different stat prefixes are not equal",
			rateLimit1: &localRateLimitIR{config: createRateLimitWithPrefix("prefix1")},
			rateLimit2: &localRateLimitIR{config: createRateLimitWithPrefix("prefix2")},
			expected:   false,
		},
		{
			name:       "same stat prefixes are equal",
			rateLimit1: &localRateLimitIR{config: createRateLimitWithPrefix("prefix1")},
			rateLimit2: &localRateLimitIR{config: createRateLimitWithPrefix("prefix1")},
			expected:   true,
		},
		{
			name:       "nil rate limit config fields are equal",
			rateLimit1: &localRateLimitIR{config: nil},
			rateLimit2: &localRateLimitIR{config: nil},
			expected:   true,
		},
		{
			name:       "nil vs non-nil rate limit config fields are not equal",
			rateLimit1: &localRateLimitIR{config: nil},
			rateLimit2: &localRateLimitIR{config: createSimpleRateLimit(100)},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.rateLimit1.Equals(tt.rateLimit2)
			assert.Equal(t, tt.expected, result)

			// Test symmetry: a.Equals(b) should equal b.Equals(a)
			reverseResult := tt.rateLimit2.Equals(tt.rateLimit1)
			assert.Equal(t, result, reverseResult, "Equals should be symmetric")
		})
	}

	// Test reflexivity: x.Equals(x) should always be true for non-nil values
	t.Run("reflexivity", func(t *testing.T) {
		rateLimit := &localRateLimitIR{config: createSimpleRateLimit(50)}
		assert.True(t, rateLimit.Equals(rateLimit), "rateLimit should equal itself")
	})

	// Test transitivity: if a.Equals(b) && b.Equals(c), then a.Equals(c)
	t.Run("transitivity", func(t *testing.T) {
		createSameRateLimit := func() *localRateLimitIR {
			return &localRateLimitIR{config: createSimpleRateLimit(75)}
		}

		a := createSameRateLimit()
		b := createSameRateLimit()
		c := createSameRateLimit()

		assert.True(t, a.Equals(b), "a should equal b")
		assert.True(t, b.Equals(c), "b should equal c")
		assert.True(t, a.Equals(c), "a should equal c (transitivity)")
	})
}

func TestLocalRateLimitIRValidate(t *testing.T) {
	tests := []struct {
		name      string
		rateLimit *localRateLimitIR
		wantErr   bool
	}{
		{
			name:      "nil rate limit is valid",
			rateLimit: nil,
			wantErr:   false,
		},
		{
			name:      "rate limit with nil config is valid",
			rateLimit: &localRateLimitIR{config: nil},
			wantErr:   false,
		},
		{
			name: "valid rate limit config passes validation",
			rateLimit: &localRateLimitIR{
				config: &localratelimitv3.LocalRateLimit{
					StatPrefix: "test_prefix",
					TokenBucket: &typev3.TokenBucket{
						MaxTokens:     1000,
						TokensPerFill: wrapperspb.UInt32(100),
						FillInterval:  durationpb.New(time.Second),
					},
				},
			},
			wantErr: false,
		},
		{
			name:      "empty rate limit config is valid",
			rateLimit: &localRateLimitIR{},
			wantErr:   false,
		},
		{
			name: "empty rate limit config fails validation",
			rateLimit: &localRateLimitIR{
				config: &localratelimitv3.LocalRateLimit{},
			},
			wantErr: true,
		},
		{
			name: "rate limit config with invalid fill interval fails validation",
			rateLimit: &localRateLimitIR{
				config: &localratelimitv3.LocalRateLimit{
					StatPrefix: "test_prefix",
					TokenBucket: &typev3.TokenBucket{
						MaxTokens:     100,
						TokensPerFill: wrapperspb.UInt32(10),
						FillInterval:  &durationpb.Duration{Seconds: -1},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rateLimit.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
