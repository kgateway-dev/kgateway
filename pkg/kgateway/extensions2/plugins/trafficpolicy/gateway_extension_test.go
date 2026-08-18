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
		name            string
		percentEnabled  *int32
		percentEnforced *int32
		wantEnabledNum  uint32
		wantEnforcedNum uint32
	}{
		{
			name:            "unset defaults to fully enabled and enforced",
			percentEnabled:  nil,
			percentEnforced: nil,
			wantEnabledNum:  100,
			wantEnforcedNum: 100,
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

			got := buildRateLimitFilter(&envoycorev3.GrpcService{}, provider)

			require.NotNil(t, got.FilterEnabled)
			assert.Equal(t, "ratelimit.api-gateway.filter_enabled", got.FilterEnabled.RuntimeKey)
			assert.Equal(t, tt.wantEnabledNum, got.FilterEnabled.DefaultValue.Numerator)
			assert.Equal(t, typev3.FractionalPercent_HUNDRED, got.FilterEnabled.DefaultValue.Denominator)

			require.NotNil(t, got.FilterEnforced)
			assert.Equal(t, "ratelimit.api-gateway.filter_enforced", got.FilterEnforced.RuntimeKey)
			assert.Equal(t, tt.wantEnforcedNum, got.FilterEnforced.DefaultValue.Numerator)
			assert.Equal(t, typev3.FractionalPercent_HUNDRED, got.FilterEnforced.DefaultValue.Denominator)
		})
	}
}

func TestBuildRateLimitFilterRuntimeKeysAreDomainScoped(t *testing.T) {
	providerA := &kgateway.RateLimitProvider{Domain: "tier-a", Timeout: metav1.Duration{Duration: 100 * time.Millisecond}}
	providerB := &kgateway.RateLimitProvider{Domain: "tier-b", Timeout: metav1.Duration{Duration: 100 * time.Millisecond}}

	filterA := buildRateLimitFilter(&envoycorev3.GrpcService{}, providerA)
	filterB := buildRateLimitFilter(&envoycorev3.GrpcService{}, providerB)

	assert.NotEqual(t, filterA.FilterEnabled.RuntimeKey, filterB.FilterEnabled.RuntimeKey,
		"distinct RateLimitProvider domains must get distinct runtime keys so one provider's override can't clobber another's")
	assert.NotEqual(t, filterA.FilterEnforced.RuntimeKey, filterB.FilterEnforced.RuntimeKey)
}
