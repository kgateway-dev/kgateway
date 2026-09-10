package trafficpolicy

import (
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/ratelimit/v3"
	localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
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

func TestToLocalRateLimitFilterConfigDescriptors(t *testing.T) {
	defaultBucket := &kgateway.TokenBucket{
		MaxTokens:     100,
		TokensPerFill: new(int32(10)),
		FillInterval:  metav1.Duration{Duration: time.Second},
	}
	descriptorBucket := kgateway.TokenBucket{
		MaxTokens:     5,
		TokensPerFill: new(int32(1)),
		FillInterval:  metav1.Duration{Duration: 2 * time.Second},
	}
	policy := &kgateway.LocalRateLimitPolicy{
		TokenBucket: defaultBucket,
		Descriptors: []kgateway.LocalRateLimitDescriptor{
			{
				Entries: []kgateway.RateLimitDescriptorEntry{{
					Type: kgateway.RateLimitDescriptorEntryTypeGeneric,
					Generic: &kgateway.RateLimitDescriptorEntryGeneric{
						Key:   "service",
						Value: "api",
					},
				}},
				TokenBucket: descriptorBucket,
			},
			{
				Entries: []kgateway.RateLimitDescriptorEntry{{
					Type:   kgateway.RateLimitDescriptorEntryTypeHeader,
					Header: new("x-user-id"),
				}},
				TokenBucket: descriptorBucket,
			},
			{
				Entries: []kgateway.RateLimitDescriptorEntry{{
					Type: kgateway.RateLimitDescriptorEntryTypeRemoteAddress,
				}},
				TokenBucket: descriptorBucket,
			},
			{
				Entries: []kgateway.RateLimitDescriptorEntry{{
					Type: kgateway.RateLimitDescriptorEntryTypePath,
				}},
				TokenBucket: descriptorBucket,
			},
		},
		AlwaysConsumeDefaultTokenBucket: new(false),
		MaxDynamicDescriptors:           new(int32(100)),
	}

	got, err := toLocalRateLimitFilterConfig(policy)
	require.NoError(t, err)
	require.NoError(t, got.ValidateAll())

	wantDescriptorBucket := &typev3.TokenBucket{
		MaxTokens:     5,
		TokensPerFill: wrapperspb.UInt32(1),
		FillInterval:  durationpb.New(2 * time.Second),
	}
	want := &localratelimitv3.LocalRateLimit{
		StatPrefix: localRateLimitStatPrefix,
		TokenBucket: &typev3.TokenBucket{
			MaxTokens:     100,
			TokensPerFill: wrapperspb.UInt32(10),
			FillInterval:  durationpb.New(time.Second),
		},
		FilterEnabled: &envoycorev3.RuntimeFractionalPercent{
			RuntimeKey: localRatelimitFilterEnabledRuntimeKey,
			DefaultValue: &typev3.FractionalPercent{
				Numerator:   100,
				Denominator: typev3.FractionalPercent_HUNDRED,
			},
		},
		FilterEnforced: &envoycorev3.RuntimeFractionalPercent{
			RuntimeKey: localRatelimitFilterEnforcedRuntimeKey,
			DefaultValue: &typev3.FractionalPercent{
				Numerator:   100,
				Denominator: typev3.FractionalPercent_HUNDRED,
			},
		},
		Descriptors: []*ratelimitv3.LocalRateLimitDescriptor{
			{
				Entries:     []*ratelimitv3.RateLimitDescriptor_Entry{{Key: "service", Value: "api"}},
				TokenBucket: wantDescriptorBucket,
			},
			{
				Entries:     []*ratelimitv3.RateLimitDescriptor_Entry{{Key: "x-user-id"}},
				TokenBucket: wantDescriptorBucket,
			},
			{
				Entries:     []*ratelimitv3.RateLimitDescriptor_Entry{{Key: remoteAddressDescriptorKey}},
				TokenBucket: wantDescriptorBucket,
			},
			{
				Entries:     []*ratelimitv3.RateLimitDescriptor_Entry{{Key: pathDescriptorKey}},
				TokenBucket: wantDescriptorBucket,
			},
		},
		RateLimits: []*envoyroutev3.RateLimit{
			{Actions: []*envoyroutev3.RateLimit_Action{{
				ActionSpecifier: &envoyroutev3.RateLimit_Action_GenericKey_{
					GenericKey: &envoyroutev3.RateLimit_Action_GenericKey{
						DescriptorKey:   "service",
						DescriptorValue: "api",
					},
				},
			}}},
			{Actions: []*envoyroutev3.RateLimit_Action{{
				ActionSpecifier: &envoyroutev3.RateLimit_Action_RequestHeaders_{
					RequestHeaders: &envoyroutev3.RateLimit_Action_RequestHeaders{
						HeaderName:    "x-user-id",
						DescriptorKey: "x-user-id",
					},
				},
			}}},
			{Actions: []*envoyroutev3.RateLimit_Action{{
				ActionSpecifier: &envoyroutev3.RateLimit_Action_RemoteAddress_{
					RemoteAddress: &envoyroutev3.RateLimit_Action_RemoteAddress{},
				},
			}}},
			{Actions: []*envoyroutev3.RateLimit_Action{{
				ActionSpecifier: &envoyroutev3.RateLimit_Action_RequestHeaders_{
					RequestHeaders: &envoyroutev3.RateLimit_Action_RequestHeaders{
						HeaderName:    ":path",
						DescriptorKey: pathDescriptorKey,
					},
				},
			}}},
		},
		AlwaysConsumeDefaultTokenBucket: wrapperspb.Bool(false),
		MaxDynamicDescriptors:           wrapperspb.UInt32(100),
	}
	assert.True(t, proto.Equal(want, got), "unexpected local rate limit config\nwant: %s\ngot:  %s", want, got)
}

func TestToLocalRateLimitFilterConfigDescriptorErrors(t *testing.T) {
	validBucket := kgateway.TokenBucket{
		MaxTokens:    1,
		FillInterval: metav1.Duration{Duration: time.Second},
	}
	validDescriptor := kgateway.LocalRateLimitDescriptor{
		Entries: []kgateway.RateLimitDescriptorEntry{{
			Type: kgateway.RateLimitDescriptorEntryTypeRemoteAddress,
		}},
		TokenBucket: validBucket,
	}

	tests := []struct {
		name    string
		policy  *kgateway.LocalRateLimitPolicy
		wantErr string
	}{
		{
			name: "missing default token bucket",
			policy: &kgateway.LocalRateLimitPolicy{
				Descriptors: []kgateway.LocalRateLimitDescriptor{validDescriptor},
			},
			wantErr: "local rate limit descriptors require the default token bucket to be set",
		},
		{
			name: "empty descriptor entries",
			policy: &kgateway.LocalRateLimitPolicy{
				TokenBucket: &validBucket,
				Descriptors: []kgateway.LocalRateLimitDescriptor{{
					TokenBucket: validBucket,
				}},
			},
			wantErr: "local rate limit descriptor 0 must contain at least one entry",
		},
		{
			name: "descriptor interval is not a multiple of default",
			policy: &kgateway.LocalRateLimitPolicy{
				TokenBucket: &validBucket,
				Descriptors: []kgateway.LocalRateLimitDescriptor{{
					Entries: validDescriptor.Entries,
					TokenBucket: kgateway.TokenBucket{
						MaxTokens:    1,
						FillInterval: metav1.Duration{Duration: 1500 * time.Millisecond},
					},
				}},
			},
			wantErr: "local rate limit descriptor 0 fill interval must be a multiple of the default token bucket fill interval",
		},
		{
			name: "duplicate descriptors",
			policy: &kgateway.LocalRateLimitPolicy{
				TokenBucket: &validBucket,
				Descriptors: []kgateway.LocalRateLimitDescriptor{
					validDescriptor,
					validDescriptor,
				},
			},
			wantErr: "local rate limit descriptor 1 duplicates an earlier descriptor",
		},
		{
			name: "malformed header entry",
			policy: &kgateway.LocalRateLimitPolicy{
				TokenBucket: &validBucket,
				Descriptors: []kgateway.LocalRateLimitDescriptor{{
					Entries: []kgateway.RateLimitDescriptorEntry{{
						Type: kgateway.RateLimitDescriptorEntryTypeHeader,
					}},
					TokenBucket: validBucket,
				}},
			},
			wantErr: "local rate limit descriptor 0: header entry requires Header field to be set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := toLocalRateLimitFilterConfig(tt.policy)
			require.EqualError(t, err, tt.wantErr)
		})
	}
}
