package krtcollections

import (
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
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

func TestGetFractionPercent(t *testing.T) {
	tests := []struct {
		name                string
		filter              gwv1.HTTPRequestMirrorFilter
		expectedNumerator   uint32
		expectedDenominator envoytype.FractionalPercent_DenominatorType
		expectNil           bool
	}{
		{
			name: "percent 50",
			filter: gwv1.HTTPRequestMirrorFilter{
				BackendRef: gwv1.BackendObjectReference{
					Name: "test",
				},
				Percent: ptr.To(int32(50)),
			},
			expectedNumerator:   50,
			expectedDenominator: envoytype.FractionalPercent_HUNDRED,
		},
		{
			name: "percent 10",
			filter: gwv1.HTTPRequestMirrorFilter{
				BackendRef: gwv1.BackendObjectReference{
					Name: "test",
				},
				Percent: ptr.To(int32(10)),
			},
			expectedNumerator:   10,
			expectedDenominator: envoytype.FractionalPercent_HUNDRED,
		},
		{
			name: "percent 100",
			filter: gwv1.HTTPRequestMirrorFilter{
				BackendRef: gwv1.BackendObjectReference{
					Name: "test",
				},
				Percent: ptr.To(int32(100)),
			},
			expectedNumerator:   100,
			expectedDenominator: envoytype.FractionalPercent_HUNDRED,
		},
		{
			name: "fraction 1/2 means 50%",
			filter: gwv1.HTTPRequestMirrorFilter{
				BackendRef: gwv1.BackendObjectReference{
					Name: "test",
				},
				Fraction: &gwv1.Fraction{
					Numerator:   1,
					Denominator: ptr.To(int32(2)),
				},
			},
			expectedNumerator:   500000,
			expectedDenominator: envoytype.FractionalPercent_MILLION,
		},
		{
			name: "fraction 1/4 means 25%",
			filter: gwv1.HTTPRequestMirrorFilter{
				BackendRef: gwv1.BackendObjectReference{
					Name: "test",
				},
				Fraction: &gwv1.Fraction{
					Numerator:   1,
					Denominator: ptr.To(int32(4)),
				},
			},
			expectedNumerator:   250000,
			expectedDenominator: envoytype.FractionalPercent_MILLION,
		},
		{
			name: "fraction with default denominator 100 (50/100 = 50%)",
			filter: gwv1.HTTPRequestMirrorFilter{
				BackendRef: gwv1.BackendObjectReference{
					Name: "test",
				},
				Fraction: &gwv1.Fraction{
					Numerator: 50,
				},
			},
			expectedNumerator:   500000,
			expectedDenominator: envoytype.FractionalPercent_MILLION,
		},
		{
			name: "nil percent and fraction means 100%",
			filter: gwv1.HTTPRequestMirrorFilter{
				BackendRef: gwv1.BackendObjectReference{
					Name: "test",
				},
			},
			expectNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := assert.New(t)
			result := getFractionPercent(tc.filter)

			if tc.expectNil {
				a.Nil(result)
			} else {
				a.NotNil(result)
				a.Equal(tc.expectedNumerator, result.DefaultValue.Numerator)
				a.Equal(tc.expectedDenominator, result.DefaultValue.Denominator)
			}
		})
	}
}

func TestRequestMirror(t *testing.T) {
	tests := []struct {
		name                 string
		ir                   mirrorIr
		initialRoute         *envoyroutev3.Route
		mergeOpts            policy.MergeOptions
		expectedMirrors      int
		expectedClusters     []string
		expectedFractions    []uint32
		expectedDenominators []envoytype.FractionalPercent_DenominatorType
	}{
		{
			name: "single mirror on empty route",
			ir: mirrorIr{
				Cluster: "backend-1",
				RuntimeFraction: &envoycorev3.RuntimeFractionalPercent{
					DefaultValue: &envoytype.FractionalPercent{
						Numerator:   5000,
						Denominator: envoytype.FractionalPercent_MILLION,
					},
				},
			},
			initialRoute: &envoyroutev3.Route{
				Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}},
			},
			mergeOpts:            policy.MergeOptions{Strategy: policy.AugmentedShallowMerge},
			expectedMirrors:      1,
			expectedClusters:     []string{"backend-1"},
			expectedFractions:    []uint32{5000},
			expectedDenominators: []envoytype.FractionalPercent_DenominatorType{envoytype.FractionalPercent_MILLION},
		},
		{
			name: "mirror with nil runtime fraction means 100%",
			ir: mirrorIr{
				Cluster:         "backend-1",
				RuntimeFraction: nil,
			},
			initialRoute: &envoyroutev3.Route{
				Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}},
			},
			mergeOpts:            policy.MergeOptions{Strategy: policy.AugmentedShallowMerge},
			expectedMirrors:      1,
			expectedClusters:     []string{"backend-1"},
			expectedFractions:    []uint32{0}, // 0 means nil (100%)
			expectedDenominators: []envoytype.FractionalPercent_DenominatorType{0},
		},
		{
			name: "mirror appends to existing mirror",
			ir: mirrorIr{
				Cluster: "backend-2",
				RuntimeFraction: &envoycorev3.RuntimeFractionalPercent{
					DefaultValue: &envoytype.FractionalPercent{
						Numerator:   7500,
						Denominator: envoytype.FractionalPercent_MILLION,
					},
				},
			},
			initialRoute: &envoyroutev3.Route{
				Action: &envoyroutev3.Route_Route{
					Route: &envoyroutev3.RouteAction{
						RequestMirrorPolicies: []*envoyroutev3.RouteAction_RequestMirrorPolicy{
							{
								Cluster: "backend-1",
								RuntimeFraction: &envoycorev3.RuntimeFractionalPercent{
									DefaultValue: &envoytype.FractionalPercent{
										Numerator:   5000,
										Denominator: envoytype.FractionalPercent_MILLION,
									},
								},
							},
						},
					},
				},
			},
			mergeOpts:            policy.MergeOptions{Strategy: policy.AugmentedShallowMerge},
			expectedMirrors:      2,
			expectedClusters:     []string{"backend-1", "backend-2"},
			expectedFractions:    []uint32{5000, 7500},
			expectedDenominators: []envoytype.FractionalPercent_DenominatorType{envoytype.FractionalPercent_MILLION, envoytype.FractionalPercent_MILLION},
		},
		{
			name: "mirror appends to two existing mirrors",
			ir: mirrorIr{
				Cluster: "backend-3",
				RuntimeFraction: &envoycorev3.RuntimeFractionalPercent{
					DefaultValue: &envoytype.FractionalPercent{
						Numerator:   2500,
						Denominator: envoytype.FractionalPercent_MILLION,
					},
				},
			},
			initialRoute: &envoyroutev3.Route{
				Action: &envoyroutev3.Route_Route{
					Route: &envoyroutev3.RouteAction{
						RequestMirrorPolicies: []*envoyroutev3.RouteAction_RequestMirrorPolicy{
							{
								Cluster: "backend-1",
								RuntimeFraction: &envoycorev3.RuntimeFractionalPercent{
									DefaultValue: &envoytype.FractionalPercent{
										Numerator:   5000,
										Denominator: envoytype.FractionalPercent_MILLION,
									},
								},
							},
							{
								Cluster: "backend-2",
								RuntimeFraction: &envoycorev3.RuntimeFractionalPercent{
									DefaultValue: &envoytype.FractionalPercent{
										Numerator:   10000,
										Denominator: envoytype.FractionalPercent_MILLION,
									},
								},
							},
						},
					},
				},
			},
			mergeOpts:            policy.MergeOptions{Strategy: policy.AugmentedShallowMerge},
			expectedMirrors:      3,
			expectedClusters:     []string{"backend-1", "backend-2", "backend-3"},
			expectedFractions:    []uint32{5000, 10000, 2500},
			expectedDenominators: []envoytype.FractionalPercent_DenominatorType{envoytype.FractionalPercent_MILLION, envoytype.FractionalPercent_MILLION, envoytype.FractionalPercent_MILLION},
		},
		{
			name: "mirror is additive with OverridableShallowMerge strategy",
			ir: mirrorIr{
				Cluster: "backend-2",
				RuntimeFraction: &envoycorev3.RuntimeFractionalPercent{
					DefaultValue: &envoytype.FractionalPercent{
						Numerator:   8000,
						Denominator: envoytype.FractionalPercent_MILLION,
					},
				},
			},
			initialRoute: &envoyroutev3.Route{
				Action: &envoyroutev3.Route_Route{
					Route: &envoyroutev3.RouteAction{
						RequestMirrorPolicies: []*envoyroutev3.RouteAction_RequestMirrorPolicy{
							{
								Cluster: "backend-1",
								RuntimeFraction: &envoycorev3.RuntimeFractionalPercent{
									DefaultValue: &envoytype.FractionalPercent{
										Numerator:   5000,
										Denominator: envoytype.FractionalPercent_MILLION,
									},
								},
							},
						},
					},
				},
			},
			mergeOpts:            policy.MergeOptions{Strategy: policy.OverridableShallowMerge},
			expectedMirrors:      2,
			expectedClusters:     []string{"backend-1", "backend-2"},
			expectedFractions:    []uint32{5000, 8000},
			expectedDenominators: []envoytype.FractionalPercent_DenominatorType{envoytype.FractionalPercent_MILLION, envoytype.FractionalPercent_MILLION},
		},
		{
			name: "mirror with 50 percent fraction",
			ir: mirrorIr{
				Cluster: "backend-1",
				RuntimeFraction: &envoycorev3.RuntimeFractionalPercent{
					DefaultValue: &envoytype.FractionalPercent{
						Numerator:   50,
						Denominator: envoytype.FractionalPercent_HUNDRED,
					},
				},
			},
			initialRoute: &envoyroutev3.Route{
				Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}},
			},
			mergeOpts:            policy.MergeOptions{Strategy: policy.AugmentedShallowMerge},
			expectedMirrors:      1,
			expectedClusters:     []string{"backend-1"},
			expectedFractions:    []uint32{50},
			expectedDenominators: []envoytype.FractionalPercent_DenominatorType{envoytype.FractionalPercent_HUNDRED},
		},
		{
			name: "mirror with 10 percent fraction",
			ir: mirrorIr{
				Cluster: "backend-1",
				RuntimeFraction: &envoycorev3.RuntimeFractionalPercent{
					DefaultValue: &envoytype.FractionalPercent{
						Numerator:   10,
						Denominator: envoytype.FractionalPercent_HUNDRED,
					},
				},
			},
			initialRoute: &envoyroutev3.Route{
				Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}},
			},
			mergeOpts:            policy.MergeOptions{Strategy: policy.AugmentedShallowMerge},
			expectedMirrors:      1,
			expectedClusters:     []string{"backend-1"},
			expectedFractions:    []uint32{10},
			expectedDenominators: []envoytype.FractionalPercent_DenominatorType{envoytype.FractionalPercent_HUNDRED},
		},
		{
			name: "multiple mirrors with different percentage fractions are cumulative",
			ir: mirrorIr{
				Cluster: "backend-3",
				RuntimeFraction: &envoycorev3.RuntimeFractionalPercent{
					DefaultValue: &envoytype.FractionalPercent{
						Numerator:   25,
						Denominator: envoytype.FractionalPercent_HUNDRED,
					},
				},
			},
			initialRoute: &envoyroutev3.Route{
				Action: &envoyroutev3.Route_Route{
					Route: &envoyroutev3.RouteAction{
						RequestMirrorPolicies: []*envoyroutev3.RouteAction_RequestMirrorPolicy{
							{
								Cluster: "backend-1",
								RuntimeFraction: &envoycorev3.RuntimeFractionalPercent{
									DefaultValue: &envoytype.FractionalPercent{
										Numerator:   50,
										Denominator: envoytype.FractionalPercent_HUNDRED,
									},
								},
							},
							{
								Cluster: "backend-2",
								RuntimeFraction: &envoycorev3.RuntimeFractionalPercent{
									DefaultValue: &envoytype.FractionalPercent{
										Numerator:   10,
										Denominator: envoytype.FractionalPercent_HUNDRED,
									},
								},
							},
						},
					},
				},
			},
			mergeOpts:            policy.MergeOptions{Strategy: policy.AugmentedShallowMerge},
			expectedMirrors:      3,
			expectedClusters:     []string{"backend-1", "backend-2", "backend-3"},
			expectedFractions:    []uint32{50, 10, 25},
			expectedDenominators: []envoytype.FractionalPercent_DenominatorType{envoytype.FractionalPercent_HUNDRED, envoytype.FractionalPercent_HUNDRED, envoytype.FractionalPercent_HUNDRED},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := assert.New(t)

			tc.ir.apply(tc.initialRoute, tc.mergeOpts)
			if tc.initialRoute.GetRoute() == nil {
				a.Equal(tc.expectedMirrors, 0, "expected no mirrors when route action is nil")
				return
			}

			policies := tc.initialRoute.GetRoute().GetRequestMirrorPolicies()
			a.Equal(tc.expectedMirrors, len(policies), "unexpected number of mirror policies")

			for i, cluster := range tc.expectedClusters {
				a.Equal(cluster, policies[i].Cluster, "unexpected cluster at index %d", i)
				expectedFraction := tc.expectedFractions[i]
				expectedDenom := tc.expectedDenominators[i]
				if expectedFraction == 0 {
					a.Nil(policies[i].RuntimeFraction, "expected nil RuntimeFraction at index %d", i)
				} else {
					a.NotNil(policies[i].RuntimeFraction, "expected non-nil RuntimeFraction at index %d", i)
					a.Equal(expectedFraction, policies[i].RuntimeFraction.DefaultValue.Numerator,
						"unexpected fraction numerator at index %d", i)
					a.Equal(expectedDenom, policies[i].RuntimeFraction.DefaultValue.Denominator,
						"unexpected fraction denominator at index %d", i)
				}
			}
		})
	}
}

func TestRequestRedirect(t *testing.T) {
	tests := []struct {
		name                  string
		filter                *gwv1.HTTPRequestRedirectFilter
		listenerPort          uint32
		expectedRedirect      *envoyroutev3.RedirectAction
		expectedNeedsListener bool
	}{
		{
			name: "scheme and port both nil uses listener port",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: nil,
				Port:   nil,
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				PortRedirect: 8080,
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
			expectedNeedsListener: true,
		},
		{
			name: "scheme http with port nil uses default 80",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: ptr.To("http"),
				Port:   nil,
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				SchemeRewriteSpecifier: &envoyroutev3.RedirectAction_SchemeRedirect{
					SchemeRedirect: "http",
				},
				PortRedirect: 80,
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
		},
		{
			name: "scheme https with port nil uses default 443",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: ptr.To("https"),
				Port:   nil,
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				SchemeRewriteSpecifier: &envoyroutev3.RedirectAction_HttpsRedirect{
					HttpsRedirect: true,
				},
				PortRedirect: 443,
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
		},
		{
			name: "explicit port overrides listener port",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: nil,
				Port:   ptr.To(gwv1.PortNumber(9090)),
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				PortRedirect: 9090,
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
		},
		{
			name: "explicit port takes precedence over scheme default",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: ptr.To("http"),
				Port:   ptr.To(gwv1.PortNumber(9090)),
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				SchemeRewriteSpecifier: &envoyroutev3.RedirectAction_SchemeRedirect{
					SchemeRedirect: "http",
				},
				PortRedirect: 9090,
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
		},
		{
			name: "scheme https with explicit port 8443",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: ptr.To("https"),
				Port:   ptr.To(gwv1.PortNumber(8443)),
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				SchemeRewriteSpecifier: &envoyroutev3.RedirectAction_HttpsRedirect{
					HttpsRedirect: true,
				},
				PortRedirect: 8443,
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
		},
		{
			name: "hostname redirect",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Hostname: ptr.To(gwv1.PreciseHostname("example.com")),
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				HostRedirect: "example.com",
				PortRedirect: 8080,
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
			expectedNeedsListener: true,
		},
		{
			name: "status code 302 found",
			filter: &gwv1.HTTPRequestRedirectFilter{
				StatusCode: ptr.To(302),
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				PortRedirect: 8080,
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
			expectedNeedsListener: true,
		},
		{
			name: "complete redirect with all fields",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme:     ptr.To("https"),
				Hostname:   ptr.To(gwv1.PreciseHostname("secure.example.com")),
				Port:       ptr.To(gwv1.PortNumber(8443)),
				StatusCode: ptr.To(308),
				Path: &gwv1.HTTPPathModifier{
					Type:            gwv1.FullPathHTTPPathModifier,
					ReplaceFullPath: ptr.To("/new-path"),
				},
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				SchemeRewriteSpecifier: &envoyroutev3.RedirectAction_HttpsRedirect{
					HttpsRedirect: true,
				},
				HostRedirect: "secure.example.com",
				PortRedirect: 8443,
				PathRewriteSpecifier: &envoyroutev3.RedirectAction_PathRedirect{
					PathRedirect: "/new-path",
				},
				ResponseCode: envoyroutev3.RedirectAction_PERMANENT_REDIRECT,
			},
		},
		{
			name: "path prefix redirect",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Path: &gwv1.HTTPPathModifier{
					Type:               gwv1.PrefixMatchHTTPPathModifier,
					ReplacePrefixMatch: ptr.To("/api"),
				},
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				PathRewriteSpecifier: &envoyroutev3.RedirectAction_PrefixRewrite{
					PrefixRewrite: "/api",
				},
				PortRedirect: 8080,
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
			expectedNeedsListener: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redirectIR, err := convertRequestRedirectIR(nil, tt.filter, nil, nil)
			require.NoError(t, err)
			require.NotNil(t, redirectIR)
			assert.Equal(t, tt.expectedNeedsListener, redirectIR.NeedsListenerPort, "NeedsListenerPort flag mismatch")

			builtinPol := &builtinPlugin{
				filter: &filterIR{
					filterType: gwv1.HTTPRouteFilterRequestRedirect,
					policy:     redirectIR,
				},
			}
			pCtx := &ir.RouteContext{
				ListenerPort: tt.listenerPort,
				Policy:       builtinPol,
			}

			outputRoute := &envoyroutev3.Route{}
			pass := &builtinPluginGwPass{}

			err = pass.ApplyForRoute(pCtx, outputRoute)
			require.NoError(t, err)

			redirect := outputRoute.GetRedirect()
			require.NotNil(t, redirect, "redirect action should be set")
			assert.True(t, proto.Equal(tt.expectedRedirect, redirect), "redirect action mismatch\nexpected: %+v\nactual: %+v", tt.expectedRedirect, redirect)
		})
	}
}
