package policy

import (
	"testing"

	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertOriginToEnvoyStringMatcher(t *testing.T) {
	tests := []struct {
		name     string
		origin   string
		expected *envoy_type_matcher_v3.StringMatcher
	}{
		{
			name:   "exact match - no wildcards",
			origin: "https://example.com",
			expected: &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
					Exact: "https://example.com",
				},
			},
		},
		{
			name:   "exact match - with port",
			origin: "http://example.com:8080",
			expected: &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
					Exact: "http://example.com:8080",
				},
			},
		},
		{
			name:   "single wildcard at end - prefix match",
			origin: "https://*",
			expected: &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_Prefix{
					Prefix: "https://",
				},
			},
		},
		{
			name:   "wildcard subdomain - regex match",
			origin: "https://*.example.com",
			expected: &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
					SafeRegex: &envoy_type_matcher_v3.RegexMatcher{
						Regex: "^https://.*\\.example\\.com$",
					},
				},
			},
		},
		{
			name:   "wildcard subdomain - regex match with port",
			origin: "https://*.example.com:8080",
			expected: &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
					SafeRegex: &envoy_type_matcher_v3.RegexMatcher{
						Regex: "^https://.*\\.example\\.com:8080$",
					},
				},
			},
		},
		{
			name:   "wildcard subdomain - multi-level domain",
			origin: "https://*.sub.example.com",
			expected: &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
					SafeRegex: &envoy_type_matcher_v3.RegexMatcher{
						Regex: "^https://.*\\.sub\\.example\\.com$",
					},
				},
			},
		},
		{
			name:   "complex wildcard pattern - regex match",
			origin: "https://sub.*.example.com",
			expected: &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
					SafeRegex: &envoy_type_matcher_v3.RegexMatcher{
						Regex: "^https://sub\\..*\\.example\\.com$",
					},
				},
			},
		},
		{
			name:   "multiple wildcards - regex match",
			origin: "https://*.sub.*.example.com",
			expected: &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
					SafeRegex: &envoy_type_matcher_v3.RegexMatcher{
						Regex: "^https://.*\\.sub\\..*\\.example\\.com$",
					},
				},
			},
		},
		{
			name:   "wildcard at end of host - prefix match",
			origin: "https://example.*",
			expected: &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_Prefix{
					Prefix: "https://example.",
				},
			},
		},
		{
			name:   "wildcard at end with port - regex match",
			origin: "https://example.*:8080",
			expected: &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
					SafeRegex: &envoy_type_matcher_v3.RegexMatcher{
						Regex: "^https://example\\..*:8080$",
					},
				},
			},
		},
		{
			name:   "wildcard in middle - regex match",
			origin: "https://api.*.example.com",
			expected: &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
					SafeRegex: &envoy_type_matcher_v3.RegexMatcher{
						Regex: "^https://api\\..*\\.example\\.com$",
					},
				},
			},
		},
		{
			name:   "wildcard at start - regex match",
			origin: "https://*.example.com",
			expected: &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
					SafeRegex: &envoy_type_matcher_v3.RegexMatcher{
						Regex: "^https://.*\\.example\\.com$",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertOriginToEnvoyStringMatcher(tt.origin)
			require.NotNil(t, result)
			assert.Equal(t, tt.expected, result)
		})
	}
}
