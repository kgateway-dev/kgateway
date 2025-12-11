package trafficpolicy

import (
	"testing"

	corsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestCorsIREquals(t *testing.T) {
	createSimpleCors := func(allowOrigin string) *corsv3.CorsPolicy {
		return &corsv3.CorsPolicy{
			AllowOriginStringMatch: []*envoy_type_matcher_v3.StringMatcher{
				{
					MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
						Exact: allowOrigin,
					},
				},
			},
			AllowCredentials: wrapperspb.Bool(true),
		}
	}
	createCorsWith := func(allowCredentials bool) *corsv3.CorsPolicy {
		return &corsv3.CorsPolicy{
			AllowCredentials: wrapperspb.Bool(allowCredentials),
		}
	}

	tests := []struct {
		name     string
		cors1    *corsIR
		cors2    *corsIR
		expected bool
	}{
		{
			name:     "both nil are equal",
			cors1:    nil,
			cors2:    nil,
			expected: true,
		},
		{
			name:     "nil vs non-nil are not equal",
			cors1:    nil,
			cors2:    &corsIR{policy: createSimpleCors("*")},
			expected: false,
		},
		{
			name:     "non-nil vs nil are not equal",
			cors1:    &corsIR{policy: createSimpleCors("*")},
			cors2:    nil,
			expected: false,
		},
		{
			name:     "same instance is equal",
			cors1:    &corsIR{policy: createSimpleCors("https://example.com")},
			cors2:    &corsIR{policy: createSimpleCors("https://example.com")},
			expected: true,
		},
		{
			name:     "different origins are not equal",
			cors1:    &corsIR{policy: createSimpleCors("https://example.com")},
			cors2:    &corsIR{policy: createSimpleCors("https://other.com")},
			expected: false,
		},
		{
			name:     "different credentials settings are not equal",
			cors1:    &corsIR{policy: createCorsWith(true)},
			cors2:    &corsIR{policy: createCorsWith(false)},
			expected: false,
		},
		{
			name:     "same credentials settings are equal",
			cors1:    &corsIR{policy: createCorsWith(true)},
			cors2:    &corsIR{policy: createCorsWith(true)},
			expected: true,
		},
		{
			name:     "nil cors config fields are equal",
			cors1:    &corsIR{policy: nil},
			cors2:    &corsIR{policy: nil},
			expected: true,
		},
		{
			name:     "nil vs non-nil cors config fields are not equal",
			cors1:    &corsIR{policy: nil},
			cors2:    &corsIR{policy: createSimpleCors("*")},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cors1.Equals(tt.cors2)
			assert.Equal(t, tt.expected, result)

			// Test symmetry: a.Equals(b) should equal b.Equals(a)
			reverseResult := tt.cors2.Equals(tt.cors1)
			assert.Equal(t, result, reverseResult, "Equals should be symmetric")
		})
	}

	// Test reflexivity: x.Equals(x) should always be true for non-nil values
	t.Run("reflexivity", func(t *testing.T) {
		cors := &corsIR{policy: createSimpleCors("https://test.com")}
		assert.True(t, cors.Equals(cors), "cors should equal itself")
	})

	// Test transitivity: if a.Equals(b) && b.Equals(c), then a.Equals(c)
	t.Run("transitivity", func(t *testing.T) {
		createSameCors := func() *corsIR {
			return &corsIR{policy: createCorsWith(false)}
		}

		a := createSameCors()
		b := createSameCors()
		c := createSameCors()

		assert.True(t, a.Equals(b), "a should equal b")
		assert.True(t, b.Equals(c), "b should equal c")
		assert.True(t, a.Equals(c), "a should equal c (transitivity)")
	})
}
