package krtcollections

import (
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

func TestURLRewriteApply(t *testing.T) {
	tests := []struct {
		name                string
		ir                  urlRewriteIr
		expectRegexPattern  string
		expectRegexSub      string
		expectPrefixReplace string
		route               *envoyroutev3.Route
		mergeOpts           policy.MergeOptions
	}{
		{
			name: "nothing set initially",
			route: &envoyroutev3.Route{
				Match: &envoyroutev3.RouteMatch{
					PathSpecifier: &envoyroutev3.RouteMatch_PathSeparatedPrefix{
						PathSeparatedPrefix: "/httpbin",
					},
				},
				Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}},
			},
			mergeOpts: policy.MergeOptions{Strategy: policy.AugmentedShallowMerge},
			ir: urlRewriteIr{
				PrefixReplace: "/",
			},
			expectRegexPattern: "^/httpbin\\/*",
			expectRegexSub:     "/",
		},
		{
			name: "prefix rewrite set initially is not overridden",
			route: &envoyroutev3.Route{
				Match: &envoyroutev3.RouteMatch{
					PathSpecifier: &envoyroutev3.RouteMatch_PathSeparatedPrefix{
						PathSeparatedPrefix: "/httpbin",
					},
				},
				Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
					PrefixRewrite: "/api",
				}},
			},
			mergeOpts: policy.MergeOptions{Strategy: policy.AugmentedShallowMerge},
			ir: urlRewriteIr{
				PrefixReplace: "/",
			},
			expectPrefixReplace: "/api",
		},
		{
			name: "prefix rewrite set initially, merge opts OverridableShallowMerge",
			route: &envoyroutev3.Route{
				Match: &envoyroutev3.RouteMatch{
					PathSpecifier: &envoyroutev3.RouteMatch_PathSeparatedPrefix{
						PathSeparatedPrefix: "/httpbin",
					},
				},
				Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
					PrefixRewrite: "/api",
				}},
			},
			mergeOpts: policy.MergeOptions{Strategy: policy.OverridableShallowMerge},
			ir: urlRewriteIr{
				PrefixReplace: "/",
			},
			expectRegexPattern: "^/httpbin\\/*",
			expectRegexSub:     "/",
		},
		{
			name: "regex rewrite set initially is not overridden",
			route: &envoyroutev3.Route{
				Match: &envoyroutev3.RouteMatch{
					PathSpecifier: &envoyroutev3.RouteMatch_PathSeparatedPrefix{
						PathSeparatedPrefix: "/httpbin",
					},
				},
				Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
					RegexRewrite: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
						Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/httpbin\\/*"},
						Substitution: "/",
					},
				}},
			},
			mergeOpts: policy.MergeOptions{Strategy: policy.AugmentedShallowMerge},
			ir: urlRewriteIr{
				PrefixReplace: "/",
			},
			expectRegexPattern: "^/httpbin\\/*",
			expectRegexSub:     "/",
		},
		{
			name: "regex rewrite set initially, merge opts OverridableShallowMerge",
			route: &envoyroutev3.Route{
				Match: &envoyroutev3.RouteMatch{
					PathSpecifier: &envoyroutev3.RouteMatch_PathSeparatedPrefix{
						PathSeparatedPrefix: "/httpbin",
					},
				},
				Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
					RegexRewrite: &envoy_type_matcher_v3.RegexMatchAndSubstitute{
						Pattern:      &envoy_type_matcher_v3.RegexMatcher{Regex: "^/httpbin\\/*"},
						Substitution: "/",
					},
				}},
			},
			mergeOpts: policy.MergeOptions{Strategy: policy.OverridableShallowMerge},
			ir: urlRewriteIr{
				PrefixReplace: "/api",
			},
			expectPrefixReplace: "/api",
		},
		{
			name: "full replace does not override prefix rewrite",
			route: &envoyroutev3.Route{
				Match: &envoyroutev3.RouteMatch{
					PathSpecifier: &envoyroutev3.RouteMatch_PathSeparatedPrefix{
						PathSeparatedPrefix: "/httpbin",
					},
				},
				Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
					PrefixRewrite: "/api",
				}},
			},
			mergeOpts: policy.MergeOptions{Strategy: policy.AugmentedShallowMerge},
			ir: urlRewriteIr{
				FullReplace: "/test",
			},
			expectPrefixReplace: "/api",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.ir.apply(tc.route, tc.mergeOpts)

			if pr := tc.route.GetRoute().GetPrefixRewrite(); pr != tc.expectPrefixReplace {
				t.Fatalf("prefix rewrite mismatch: got %q want %q", pr, tc.expectPrefixReplace)
			}

			rr := tc.route.GetRoute().GetRegexRewrite()
			if tc.expectRegexPattern != "" || tc.expectRegexSub != "" {
				if rr == nil {
					t.Fatalf("expected RegexRewrite to be set")
				}
				if rr.Pattern == nil || rr.Pattern.GetRegex() != tc.expectRegexPattern || rr.Substitution != tc.expectRegexSub {
					t.Fatalf("regex rewrite mismatch: got pattern=%v sub=%q, want pattern=%v sub=%q", rr.GetPattern(), rr.Substitution, tc.expectRegexPattern, tc.expectRegexSub)
				}
			} else if rr != nil {
				t.Fatalf("expected RegexRewrite to be nil, got %#v", rr)
			}
		})
	}
}

func TestParseRedirectStatusCodeAnnotation(t *testing.T) {
	sectionName := func(s string) *gwv1.SectionName {
		return ptr.To(gwv1.SectionName(s))
	}

	tests := []struct {
		name     string
		value    string
		rule     *gwv1.SectionName
		wantCode *int
		wantErr  string
	}{
		{
			name:     "valid status code 301",
			value:    "301",
			rule:     sectionName("rule1"),
			wantCode: ptr.To(301),
		},
		{
			name:     "valid status code 302",
			value:    "302",
			rule:     sectionName("rule1"),
			wantCode: ptr.To(302),
		},
		{
			name:     "valid status code 303",
			value:    "303",
			rule:     sectionName("rule1"),
			wantCode: ptr.To(303),
		},
		{
			name:     "valid status code 307",
			value:    "307",
			rule:     sectionName("rule1"),
			wantCode: ptr.To(307),
		},
		{
			name:     "valid status code 308",
			value:    "308",
			rule:     sectionName("rule1"),
			wantCode: ptr.To(308),
		},
		{
			name:     "valid status code with rule-specific format",
			value:    "rule1=301,rule2=302",
			rule:     sectionName("rule1"),
			wantCode: ptr.To(301),
		},
		{
			name:     "valid status code with rule-specific format with spaces",
			value:    "rule1=301, rule2=302",
			rule:     sectionName("rule2"),
			wantCode: ptr.To(302),
		},
		{
			name:     "rule-specific format but rule doesn't match",
			value:    "rule2=301",
			rule:     sectionName("rule1"),
			wantCode: nil,
		},
		{
			name:     "invalid status code - non-numeric",
			value:    "abc",
			rule:     sectionName("rule1"),
			wantCode: nil,
			wantErr:  "invalid redirect status code: abc",
		},
		{
			name:     "invalid rule-specific format",
			value:    "rule1:301",
			rule:     sectionName("rule1"),
			wantCode: nil,
			wantErr:  "invalid redirect status code: rule1:301",
		},
		{
			name:     "empty value",
			value:    "",
			rule:     sectionName("rule1"),
			wantCode: nil,
			wantErr:  "missing value",
		},
		{
			name:     "nil rule with simple value",
			value:    "301",
			rule:     nil,
			wantCode: ptr.To(301),
		},
		{
			name:     "nil rule with rule-specific format",
			value:    "rule1=301",
			rule:     nil,
			wantCode: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := assert.New(t)
			code, err := parseRedirectStatusCodeAnnotation(tc.value, tc.rule)

			// Check error
			if tc.wantErr != "" {
				a.ErrorContains(err, tc.wantErr)
			} else if err != nil {
				a.NoError(err)
			}

			// Check code
			if tc.wantCode != nil {
				a.Equal(tc.wantCode, code)
			}
		})
	}
}

// Helper function is already defined elsewhere
