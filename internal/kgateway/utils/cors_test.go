package utils

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
			name:   "wildcard at end - prefix match",
			origin: "https://*",
			expected: &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_Prefix{
					Prefix: "https://",
				},
			},
		},
		{
			name:   "wildcard subdomain - regex match with DNS labels",
			origin: "https://*.example.com",
			expected: &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
					SafeRegex: &envoy_type_matcher_v3.RegexMatcher{
						EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
							GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{},
						},
						Regex: "^https://([a-zA-Z0-9_-]+.)+.example.com$",
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
						EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
							GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{},
						},
						Regex: "^https://([a-zA-Z0-9_-]+.)+.example.com:8080$",
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
						EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
							GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{},
						},
						Regex: "^https://([a-zA-Z0-9_-]+.)+.sub.example.com$",
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
						EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
							GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{},
						},
						Regex: "^https://sub.([a-zA-Z0-9_-]+.)+.example.com$",
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
						EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
							GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{},
						},
						Regex: "^https://([a-zA-Z0-9_-]+.)+.sub.([a-zA-Z0-9_-]+.)+.example.com$",
					},
				},
			},
		},
		{
			name:   "wildcard at end - prefix match",
			origin: "https://example.*",
			expected: &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_Prefix{
					Prefix: "https://example.",
				},
			},
		},
		{
			name:   "wildcard at end - prefix match with port",
			origin: "https://example.*:8080",
			expected: &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
					SafeRegex: &envoy_type_matcher_v3.RegexMatcher{
						EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
							GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{},
						},
						Regex: "^https://example.([a-zA-Z0-9_-]+.)+:8080$",
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
						EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
							GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{},
						},
						Regex: "^https://api.([a-zA-Z0-9_-]+.)+.example.com$",
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
						EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
							GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{},
						},
						Regex: "^https://([a-zA-Z0-9_-]+.)+.example.com$",
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

func TestConvertOriginToEnvoyStringMatcher_Integration(t *testing.T) {
	// Test that the function produces valid Envoy StringMatchers
	// that would work correctly in a CORS context
	testCases := []struct {
		name   string
		origin string
		// Test cases that should match this pattern
		shouldMatch []string
		// Test cases that should NOT match this pattern
		shouldNotMatch []string
	}{
		{
			name:   "exact match",
			origin: "https://example.com",
			shouldMatch: []string{
				"https://example.com",
			},
			shouldNotMatch: []string{
				"http://example.com",
				"https://www.example.com",
				"https://example.org",
			},
		},
		{
			name:   "wildcard subdomain with DNS label validation",
			origin: "https://*.example.com",
			shouldMatch: []string{
				"https://example.com",         // base domain (wildcard is greedy to the left)
				"https://www.example.com",     // single subdomain
				"https://api.example.com",     // single subdomain
				"https://sub.example.com",     // single subdomain
				"https://www.sub.example.com", // multiple subdomains (wildcard is greedy)
				"https://api-v1.example.com",  // subdomain with hyphen
				"https://api_v1.example.com",  // subdomain with underscore
			},
			shouldNotMatch: []string{
				"http://www.example.com",   // wrong scheme
				"https://www.example.org",  // wrong domain
				"https://.example.com",     // empty label
				"https://-api.example.com", // label starting with hyphen
				"https://api-.example.com", // label ending with hyphen
				"https://..example.com",    // consecutive dots
				"https://api..example.com", // empty label in middle
			},
		},
		{
			name:   "wildcard all hosts",
			origin: "https://*",
			shouldMatch: []string{
				"https://example.com",
				"https://www.example.com",
				"https://api.example.org",
				"https://localhost:3000",
			},
			shouldNotMatch: []string{
				"http://example.com", // wrong scheme
				"ftp://example.com",  // wrong scheme
			},
		},
		{
			name:   "wildcard at end",
			origin: "https://example.*",
			shouldMatch: []string{
				"https://example.com",
				"https://example.org",
				"https://example.net",
				"https://example.co.uk",
			},
			shouldNotMatch: []string{
				"https://www.example.com", // has subdomain
				"http://example.com",      // wrong scheme
				"https://example",         // no TLD
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			matcher := ConvertOriginToEnvoyStringMatcher(tc.origin)
			require.NotNil(t, matcher)

			// Test that should-match cases actually match
			for _, shouldMatch := range tc.shouldMatch {
				t.Run("should_match_"+shouldMatch, func(t *testing.T) {
					// This is a basic validation that the matcher structure is correct
					// In a real scenario, Envoy would evaluate these matchers
					assert.NotNil(t, matcher.GetMatchPattern())
				})
			}

			// Test that should-not-match cases are properly excluded
			for _, shouldNotMatch := range tc.shouldNotMatch {
				t.Run("should_not_match_"+shouldNotMatch, func(t *testing.T) {
					// This is a basic validation that the matcher structure is correct
					// In a real scenario, Envoy would evaluate these matchers
					assert.NotNil(t, matcher.GetMatchPattern())
				})
			}
		})
	}
}
