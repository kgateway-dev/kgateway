package trafficpolicy

import (
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func TestBuildStringListMatcher(t *testing.T) {
	tests := []struct {
		name      string
		headers   []string
		wantNil   bool
		wantExact []string
	}{
		{
			name:    "nil input returns nil",
			headers: nil,
			wantNil: true,
		},
		{
			name:    "empty slice returns nil",
			headers: []string{},
			wantNil: true,
		},
		{
			name:      "single header",
			headers:   []string{"location"},
			wantExact: []string{"location"},
		},
		{
			name:      "multiple headers produce exact matchers in order",
			headers:   []string{"location", "set-cookie", "www-authenticate"},
			wantExact: []string{"location", "set-cookie", "www-authenticate"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildStringListMatcher(tt.headers)
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			require.Len(t, result.Patterns, len(tt.wantExact))
			for i, want := range tt.wantExact {
				assert.Equal(t, want, result.Patterns[i].GetExact())
			}
		})
	}
}

func TestBuildRateLimitFilterShadowMode(t *testing.T) {
	int32Ptr := func(v int32) *int32 { return &v }

	tests := []struct {
		name              string
		percentEnabled    *int32
		percentEnforced   *int32
		wantEnabledUnset  bool
		wantEnabledNum    uint32
		wantEnforcedUnset bool
		wantEnforcedNum   uint32
	}{
		{
			name:              "unset leaves both nil so Envoy falls back to its own default runtime keys",
			percentEnabled:    nil,
			percentEnforced:   nil,
			wantEnabledUnset:  true,
			wantEnforcedUnset: true,
		},
		{
			name:            "shadow mode: fully enabled, not enforced",
			percentEnabled:  int32Ptr(100),
			percentEnforced: int32Ptr(0),
			wantEnabledNum:  100,
			wantEnforcedNum: 0,
		},
		{
			name:            "partial shadow: fully enabled, partially enforced",
			percentEnabled:  int32Ptr(100),
			percentEnforced: int32Ptr(25),
			wantEnabledNum:  100,
			wantEnforcedNum: 25,
		},
		{
			name:            "filter disabled entirely",
			percentEnabled:  int32Ptr(0),
			percentEnforced: int32Ptr(0),
			wantEnabledNum:  0,
			wantEnforcedNum: 0,
		},
		{
			name:              "only enabled set, enforced left to Envoy's default",
			percentEnabled:    int32Ptr(100),
			percentEnforced:   nil,
			wantEnabledNum:    100,
			wantEnforcedUnset: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &kgateway.RateLimitProvider{
				Domain:          "api-gateway",
				FailOpen:        true,
				Timeout:         metav1.Duration{Duration: 100 * time.Millisecond},
				PercentEnabled:  tt.percentEnabled,
				PercentEnforced: tt.percentEnforced,
			}

			got := buildRateLimitFilter("default/full-ratelimit", &envoycorev3.GrpcService{}, provider)

			if tt.wantEnabledUnset {
				assert.Nil(t, got.FilterEnabled,
					"nil PercentEnabled must leave FilterEnabled unset so Envoy consults its own global runtime key default")
			} else {
				require.NotNil(t, got.FilterEnabled)
				assert.Equal(t, "ratelimit.default.full-ratelimit.filter_enabled", got.FilterEnabled.RuntimeKey)
				assert.Equal(t, tt.wantEnabledNum, got.FilterEnabled.DefaultValue.Numerator)
				assert.Equal(t, typev3.FractionalPercent_HUNDRED, got.FilterEnabled.DefaultValue.Denominator)
			}

			if tt.wantEnforcedUnset {
				assert.Nil(t, got.FilterEnforced,
					"nil PercentEnforced must leave FilterEnforced unset so Envoy consults its own global runtime key default")
			} else {
				require.NotNil(t, got.FilterEnforced)
				assert.Equal(t, "ratelimit.default.full-ratelimit.filter_enforced", got.FilterEnforced.RuntimeKey)
				assert.Equal(t, tt.wantEnforcedNum, got.FilterEnforced.DefaultValue.Numerator)
				assert.Equal(t, typev3.FractionalPercent_HUNDRED, got.FilterEnforced.DefaultValue.Denominator)
			}
		})
	}
}

func TestBuildRateLimitFilterRuntimeKeysAreScopedByProviderNotDomain(t *testing.T) {
	int32Ptr := func(v int32) *int32 { return &v }

	// Two distinct GatewayExtensions may legitimately share the same Domain (kgateway does not
	// enforce Domain uniqueness). The runtime key must still be per-provider, otherwise an
	// operator overriding one provider's key at runtime would silently affect the other.
	providerA := &kgateway.RateLimitProvider{
		Domain:          "api-gateway",
		Timeout:         metav1.Duration{Duration: 100 * time.Millisecond},
		PercentEnforced: int32Ptr(25),
	}
	providerB := &kgateway.RateLimitProvider{
		Domain:          "api-gateway",
		Timeout:         metav1.Duration{Duration: 100 * time.Millisecond},
		PercentEnforced: int32Ptr(75),
	}

	filterA := buildRateLimitFilter("default/provider-a", &envoycorev3.GrpcService{}, providerA)
	filterB := buildRateLimitFilter("default/provider-b", &envoycorev3.GrpcService{}, providerB)

	assert.NotEqual(t, filterA.FilterEnforced.RuntimeKey, filterB.FilterEnforced.RuntimeKey,
		"same-domain providers must still get distinct runtime keys so one provider's override can't clobber another's")
}
