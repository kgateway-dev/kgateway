package krtcollections

import (
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"

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
