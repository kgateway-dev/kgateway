package trafficpolicy

import (
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func TestURLRewriteIREquals(t *testing.T) {
	tests := []struct {
		name     string
		a        *urlRewriteIR
		b        *urlRewriteIR
		expected bool
	}{
		{
			name:     "both nil are equal",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "nil vs non-nil are not equal",
			a:        nil,
			b:        &urlRewriteIR{},
			expected: false,
		},
		{
			name:     "non-nil vs nil are not equal",
			a:        &urlRewriteIR{},
			b:        nil,
			expected: false,
		},
		{
			name: "same hostname is equal",
			a: &urlRewriteIR{
				hostname: ptr.To("example.com"),
			},
			b: &urlRewriteIR{
				hostname: ptr.To("example.com"),
			},
			expected: true,
		},
		{
			name: "different hostname is not equal",
			a: &urlRewriteIR{
				hostname: ptr.To("example.com"),
			},
			b: &urlRewriteIR{
				hostname: ptr.To("other.com"),
			},
			expected: false,
		},
		{
			name: "hostname nil vs non-nil is not equal",
			a: &urlRewriteIR{
				hostname: nil,
			},
			b: &urlRewriteIR{
				hostname: ptr.To("example.com"),
			},
			expected: false,
		},
		{
			name: "same regex match is equal",
			a: &urlRewriteIR{
				regexMatch: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/foo/(.*)"},
					Substitution: "/bar/$1",
				},
			},
			b: &urlRewriteIR{
				regexMatch: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/foo/(.*)"},
					Substitution: "/bar/$1",
				},
			},
			expected: true,
		},
		{
			name: "different regex pattern is not equal",
			a: &urlRewriteIR{
				regexMatch: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/foo/(.*)"},
					Substitution: "/bar/$1",
				},
			},
			b: &urlRewriteIR{
				regexMatch: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/baz/(.*)"},
					Substitution: "/bar/$1",
				},
			},
			expected: false,
		},
		{
			name: "different substitution is not equal",
			a: &urlRewriteIR{
				regexMatch: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/foo/(.*)"},
					Substitution: "/bar/$1",
				},
			},
			b: &urlRewriteIR{
				regexMatch: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/foo/(.*)"},
					Substitution: "/qux/$1",
				},
			},
			expected: false,
		},
		{
			name: "regex nil vs non-nil is not equal",
			a: &urlRewriteIR{
				regexMatch: nil,
			},
			b: &urlRewriteIR{
				regexMatch: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/foo/(.*)"},
					Substitution: "/bar/$1",
				},
			},
			expected: false,
		},
		{
			name: "reflexivity - same instance",
			a: &urlRewriteIR{
				hostname: ptr.To("example.com"),
				regexMatch: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/foo/(.*)"},
					Substitution: "/bar/$1",
				},
			},
			b: &urlRewriteIR{
				hostname: ptr.To("example.com"),
				regexMatch: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/foo/(.*)"},
					Substitution: "/bar/$1",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.a.Equals(tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestURLRewriteIRValidate(t *testing.T) {
	tests := []struct {
		name        string
		ir          *urlRewriteIR
		expectError bool
	}{
		{
			name:        "nil IR is valid",
			ir:          nil,
			expectError: false,
		},
		{
			name:        "empty IR is valid",
			ir:          &urlRewriteIR{},
			expectError: false,
		},
		{
			name: "valid regex pattern",
			ir: &urlRewriteIR{
				regexMatch: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/api/v1/(.*)"},
					Substitution: "/v2/\\1",
				},
			},
			expectError: false,
		},
		{
			name: "invalid regex pattern",
			ir: &urlRewriteIR{
				regexMatch: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "[invalid("},
					Substitution: "/test",
				},
			},
			expectError: true,
		},
		{
			name: "hostname only is valid",
			ir: &urlRewriteIR{
				hostname: ptr.To("example.com"),
			},
			expectError: false,
		},
		{
			name: "nil pattern in regex match is valid",
			ir: &urlRewriteIR{
				regexMatch: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      nil,
					Substitution: "/test",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ir.Validate()
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid regex pattern")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConstructURLRewrite(t *testing.T) {
	tests := []struct {
		name             string
		spec             v1alpha1.TrafficPolicySpec
		expectedHostname *string
		expectedPattern  string
		expectedSubst    string
		expectNil        bool
	}{
		{
			name:      "nil urlRewrite produces nil output",
			spec:      v1alpha1.TrafficPolicySpec{},
			expectNil: true,
		},
		{
			name: "hostname only",
			spec: v1alpha1.TrafficPolicySpec{
				UrlRewrite: &v1alpha1.URLRewrite{
					Hostname: ptr.To("example.com"),
				},
			},
			expectedHostname: ptr.To("example.com"),
		},
		{
			name: "path only",
			spec: v1alpha1.TrafficPolicySpec{
				UrlRewrite: &v1alpha1.URLRewrite{
					Path: &v1alpha1.PathRewrite{
						Pattern:      "^/foo/(.*)",
						Substitution: "/bar/\\1",
					},
				},
			},
			expectedPattern: "^/foo/(.*)",
			expectedSubst:   "/bar/\\1",
		},
		{
			name: "both hostname and path",
			spec: v1alpha1.TrafficPolicySpec{
				UrlRewrite: &v1alpha1.URLRewrite{
					Hostname: ptr.To("example.com"),
					Path: &v1alpha1.PathRewrite{
						Pattern:      "^/api/(.*)",
						Substitution: "/v1/\\1",
					},
				},
			},
			expectedHostname: ptr.To("example.com"),
			expectedPattern:  "^/api/(.*)",
			expectedSubst:    "/v1/\\1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &trafficPolicySpecIr{}
			constructURLRewrite(tt.spec, out)

			if tt.expectNil {
				assert.Nil(t, out.urlRewrite)
				return
			}

			assert.NotNil(t, out.urlRewrite)
			if tt.expectedHostname != nil {
				assert.Equal(t, *tt.expectedHostname, *out.urlRewrite.hostname)
			} else {
				assert.Nil(t, out.urlRewrite.hostname)
			}

			if tt.expectedPattern != "" {
				assert.NotNil(t, out.urlRewrite.regexMatch)
				assert.Equal(t, tt.expectedPattern, out.urlRewrite.regexMatch.GetPattern().GetRegex())
				assert.Equal(t, tt.expectedSubst, out.urlRewrite.regexMatch.GetSubstitution())
			} else {
				assert.Nil(t, out.urlRewrite.regexMatch)
			}
		})
	}
}

func TestApplyURLRewrite(t *testing.T) {
	tests := []struct {
		name                    string
		urlRewrite              *urlRewriteIR
		route                   *envoyroutev3.Route
		expectedHostRewrite     string
		expectedRegexPattern    string
		expectedRegexSubst      string
		expectNoHostRewrite     bool
		expectNoRegexRewrite    bool
		expectExistingPreserved bool
	}{
		{
			name:                 "nil urlRewrite does nothing",
			urlRewrite:           nil,
			route:                &envoyroutev3.Route{Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}}},
			expectNoHostRewrite:  true,
			expectNoRegexRewrite: true,
		},
		{
			name: "nil route does nothing",
			urlRewrite: &urlRewriteIR{
				hostname: ptr.To("example.com"),
			},
			route:                nil,
			expectNoHostRewrite:  true,
			expectNoRegexRewrite: true,
		},
		{
			name: "apply hostname rewrite",
			urlRewrite: &urlRewriteIR{
				hostname: ptr.To("example.com"),
			},
			route:               &envoyroutev3.Route{Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}}},
			expectedHostRewrite: "example.com",
		},
		{
			name: "apply regex path rewrite",
			urlRewrite: &urlRewriteIR{
				regexMatch: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/foo/(.*)"},
					Substitution: "/bar/\\1",
				},
			},
			route:                &envoyroutev3.Route{Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}}},
			expectedRegexPattern: "^/foo/(.*)",
			expectedRegexSubst:   "/bar/\\1",
		},
		{
			name: "apply both hostname and path rewrite",
			urlRewrite: &urlRewriteIR{
				hostname: ptr.To("example.com"),
				regexMatch: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/api/(.*)"},
					Substitution: "/v1/\\1",
				},
			},
			route:                &envoyroutev3.Route{Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}}},
			expectedHostRewrite:  "example.com",
			expectedRegexPattern: "^/api/(.*)",
			expectedRegexSubst:   "/v1/\\1",
		},
		{
			name: "does not overwrite existing host rewrite",
			urlRewrite: &urlRewriteIR{
				hostname: ptr.To("new.com"),
			},
			route: &envoyroutev3.Route{
				Action: &envoyroutev3.Route_Route{
					Route: &envoyroutev3.RouteAction{
						HostRewriteSpecifier: &envoyroutev3.RouteAction_HostRewriteLiteral{
							HostRewriteLiteral: "existing.com",
						},
					},
				},
			},
			expectedHostRewrite:     "existing.com",
			expectExistingPreserved: true,
		},
		{
			name: "does not overwrite existing regex rewrite",
			urlRewrite: &urlRewriteIR{
				regexMatch: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/new/(.*)"},
					Substitution: "/new-dest/\\1",
				},
			},
			route: &envoyroutev3.Route{
				Action: &envoyroutev3.Route_Route{
					Route: &envoyroutev3.RouteAction{
						RegexRewrite: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
							Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/existing/(.*)"},
							Substitution: "/existing-dest/\\1",
						},
					},
				},
			},
			expectedRegexPattern:    "^/existing/(.*)",
			expectedRegexSubst:      "/existing-dest/\\1",
			expectExistingPreserved: true,
		},
		{
			name: "does not overwrite existing prefix rewrite",
			urlRewrite: &urlRewriteIR{
				regexMatch: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/new/(.*)"},
					Substitution: "/new-dest/\\1",
				},
			},
			route: &envoyroutev3.Route{
				Action: &envoyroutev3.Route_Route{
					Route: &envoyroutev3.RouteAction{
						PrefixRewrite: "/existing-prefix",
					},
				},
			},
			expectNoRegexRewrite:    true,
			expectExistingPreserved: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyURLRewrite(tt.urlRewrite, tt.route)

			if tt.route == nil || tt.route.GetRoute() == nil {
				return
			}

			action := tt.route.GetRoute()

			// Check host rewrite
			if tt.expectNoHostRewrite {
				assert.Nil(t, action.GetHostRewriteSpecifier())
			} else if tt.expectedHostRewrite != "" {
				literal := action.GetHostRewriteLiteral()
				assert.Equal(t, tt.expectedHostRewrite, literal)
			}

			// Check regex rewrite
			if tt.expectNoRegexRewrite {
				if !tt.expectExistingPreserved || action.GetPrefixRewrite() == "" {
					assert.Nil(t, action.GetRegexRewrite())
				}
			} else if tt.expectedRegexPattern != "" {
				regexRewrite := action.GetRegexRewrite()
				assert.NotNil(t, regexRewrite)
				assert.Equal(t, tt.expectedRegexPattern, regexRewrite.GetPattern().GetRegex())
				assert.Equal(t, tt.expectedRegexSubst, regexRewrite.GetSubstitution())
			}
		})
	}
}
