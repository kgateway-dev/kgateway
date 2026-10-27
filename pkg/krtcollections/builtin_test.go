package krtcollections

import (
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	stateful_sessionv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/stateful_session/v3"
	stateful_cookie "github.com/envoyproxy/go-control-plane/envoy/extensions/http/stateful_session/cookie/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
	apiconditions "github.com/kgateway-dev/kgateway/v2/api/conditions"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils/sessionpersistence"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
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
		return new(gwv1.SectionName(s))
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
			wantCode: new(301),
		},
		{
			name:     "valid status code 302",
			value:    "302",
			rule:     sectionName("rule1"),
			wantCode: new(302),
		},
		{
			name:     "valid status code 303",
			value:    "303",
			rule:     sectionName("rule1"),
			wantCode: new(303),
		},
		{
			name:     "valid status code 307",
			value:    "307",
			rule:     sectionName("rule1"),
			wantCode: new(307),
		},
		{
			name:     "valid status code 308",
			value:    "308",
			rule:     sectionName("rule1"),
			wantCode: new(308),
		},
		{
			name:     "valid status code with rule-specific format",
			value:    "rule1=301,rule2=302",
			rule:     sectionName("rule1"),
			wantCode: new(301),
		},
		{
			name:     "valid status code with rule-specific format with spaces",
			value:    "rule1=301, rule2=302",
			rule:     sectionName("rule2"),
			wantCode: new(302),
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
			wantCode: new(301),
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
				Percent: new(int32(50)),
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
				Percent: new(int32(10)),
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
				Percent: new(int32(100)),
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
					Denominator: new(int32(2)),
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
					Denominator: new(int32(4)),
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
		name             string
		filter           *gwv1.HTTPRequestRedirectFilter
		listenerPort     uint32
		listenerHasTLS   bool
		expectedRedirect *envoyroutev3.RedirectAction
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
		},
		{
			name: "scheme and port both nil omits default HTTP listener port 80",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: nil,
				Port:   nil,
			},
			listenerPort: 80,
			expectedRedirect: &envoyroutev3.RedirectAction{
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
		},
		{
			name: "scheme and port both nil omits default HTTPS listener port 443",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: nil,
				Port:   nil,
			},
			listenerPort:   443,
			listenerHasTLS: true,
			expectedRedirect: &envoyroutev3.RedirectAction{
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
		},
		{
			name: "scheme http with port nil omits default port 80",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: new("http"),
				Port:   nil,
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				SchemeRewriteSpecifier: &envoyroutev3.RedirectAction_SchemeRedirect{
					SchemeRedirect: "http",
				},
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
		},
		{
			name: "scheme https with port nil omits default port 443",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: new("https"),
				Port:   nil,
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				SchemeRewriteSpecifier: &envoyroutev3.RedirectAction_HttpsRedirect{
					HttpsRedirect: true,
				},
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
		},
		{
			name: "explicit port overrides listener port",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: nil,
				Port:   new(gwv1.PortNumber(9090)),
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				PortRedirect: 9090,
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
		},
		{
			name: "scheme nil with explicit default HTTP port is omitted",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: nil,
				Port:   new(gwv1.PortNumber(80)),
			},
			listenerPort: 80,
			expectedRedirect: &envoyroutev3.RedirectAction{
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
		},
		{
			name: "scheme nil with explicit default HTTPS port is omitted",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: nil,
				Port:   new(gwv1.PortNumber(443)),
			},
			listenerPort:   443,
			listenerHasTLS: true,
			expectedRedirect: &envoyroutev3.RedirectAction{
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
		},
		{
			name: "explicit port takes precedence over scheme default",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: new("http"),
				Port:   new(gwv1.PortNumber(9090)),
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
				Scheme: new("https"),
				Port:   new(gwv1.PortNumber(8443)),
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
			name: "explicit default https port is omitted",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: new("https"),
				Port:   new(gwv1.PortNumber(443)),
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				SchemeRewriteSpecifier: &envoyroutev3.RedirectAction_HttpsRedirect{
					HttpsRedirect: true,
				},
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
		},
		{
			name: "explicit default http port is omitted",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: new("http"),
				Port:   new(gwv1.PortNumber(80)),
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				SchemeRewriteSpecifier: &envoyroutev3.RedirectAction_SchemeRedirect{
					SchemeRedirect: "http",
				},
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
		},
		{
			name: "hostname redirect",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Hostname: new(gwv1.PreciseHostname("example.com")),
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				HostRedirect: "example.com",
				PortRedirect: 8080,
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
		},
		{
			name: "status code 302 found",
			filter: &gwv1.HTTPRequestRedirectFilter{
				StatusCode: new(302),
			},
			listenerPort: 8080,
			expectedRedirect: &envoyroutev3.RedirectAction{
				PortRedirect: 8080,
				ResponseCode: envoyroutev3.RedirectAction_FOUND,
			},
		},
		{
			name: "complete redirect with all fields",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme:     new("https"),
				Hostname:   new(gwv1.PreciseHostname("secure.example.com")),
				Port:       new(gwv1.PortNumber(8443)),
				StatusCode: new(308),
				Path: &gwv1.HTTPPathModifier{
					Type:            gwv1.FullPathHTTPPathModifier,
					ReplaceFullPath: new("/new-path"),
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
					ReplacePrefixMatch: new("/api"),
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redirectIR, err := convertRequestRedirectIR(nil, tt.filter, nil, nil)
			require.NoError(t, err)
			require.NotNil(t, redirectIR)

			builtinPol := &builtinPlugin{
				filter: &filterIR{
					filterType: gwv1.HTTPRouteFilterRequestRedirect,
					policy:     redirectIR,
				},
			}
			pCtx := &ir.RouteContext{
				ListenerPort:   tt.listenerPort,
				ListenerHasTLS: tt.listenerHasTLS,
				Policy:         builtinPol,
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

// Regression for https://github.com/kgateway-dev/kgateway/issues/13889: one HTTPRoute attached to
// multiple listeners must not share a single RedirectAction; listener-specific port post-processing
// must not overwrite other listeners' routes.
func TestRequestRedirectSamePolicyMultipleListeners(t *testing.T) {
	redirectIR, err := convertRequestRedirectIR(nil, &gwv1.HTTPRequestRedirectFilter{
		Path: &gwv1.HTTPPathModifier{
			Type:            gwv1.FullPathHTTPPathModifier,
			ReplaceFullPath: new("/somewhere/"),
		},
		StatusCode: new(301),
	}, nil, nil)
	require.NoError(t, err)

	builtinPol := &builtinPlugin{
		filter: &filterIR{
			filterType: gwv1.HTTPRouteFilterRequestRedirect,
			policy:     redirectIR,
		},
	}
	pass := &builtinPluginGwPass{}

	route80 := &envoyroutev3.Route{}
	require.NoError(t, pass.ApplyForRoute(&ir.RouteContext{
		ListenerPort: 80,
		Policy:       builtinPol,
	}, route80))
	route666 := &envoyroutev3.Route{}
	require.NoError(t, pass.ApplyForRoute(&ir.RouteContext{
		ListenerPort: 666,
		Policy:       builtinPol,
	}, route666))
	route8080 := &envoyroutev3.Route{}
	require.NoError(t, pass.ApplyForRoute(&ir.RouteContext{
		ListenerPort: 8080,
		Policy:       builtinPol,
	}, route8080))

	assert.Equal(t, uint32(0), route80.GetRedirect().GetPortRedirect())
	assert.Equal(t, uint32(666), route666.GetRedirect().GetPortRedirect())
	assert.Equal(t, uint32(8080), route8080.GetRedirect().GetPortRedirect())
	assert.Equal(t, uint32(0), redirectIR.Redir.GetPortRedirect(), "IR redirect template must keep PortRedirect unset")
}

func TestRequestRedirectLowerPriorityPolicyDoesNotPostProcessWinningRedirect(t *testing.T) {
	winningRedirectIR, err := convertRequestRedirectIR(nil, &gwv1.HTTPRequestRedirectFilter{
		Scheme: new("https"),
		Port:   new(gwv1.PortNumber(8443)),
	}, nil, nil)
	require.NoError(t, err)

	losingRedirectIR, err := convertRequestRedirectIR(nil, &gwv1.HTTPRequestRedirectFilter{}, nil, nil)
	require.NoError(t, err)

	pass := &builtinPluginGwPass{}
	outputRoute := &envoyroutev3.Route{}

	require.NoError(t, pass.ApplyForRoute(&ir.RouteContext{
		ListenerPort: 80,
		Policy: &builtinPlugin{
			filter: &filterIR{
				filterType: gwv1.HTTPRouteFilterRequestRedirect,
				policy:     winningRedirectIR,
			},
		},
		InheritedPolicyPriority: apiannotations.ShallowMergePreferChild,
	}, outputRoute))

	require.NoError(t, pass.ApplyForRoute(&ir.RouteContext{
		ListenerPort: 80,
		Policy: &builtinPlugin{
			filter: &filterIR{
				filterType: gwv1.HTTPRouteFilterRequestRedirect,
				policy:     losingRedirectIR,
			},
		},
		InheritedPolicyPriority: apiannotations.ShallowMergePreferChild,
	}, outputRoute))

	expectedRedirect := &envoyroutev3.RedirectAction{
		SchemeRewriteSpecifier: &envoyroutev3.RedirectAction_HttpsRedirect{
			HttpsRedirect: true,
		},
		PortRedirect: 8443,
		ResponseCode: envoyroutev3.RedirectAction_FOUND,
	}
	assert.True(t, proto.Equal(expectedRedirect, outputRoute.GetRedirect()),
		"lower-priority redirect must not mutate the winning redirect\nexpected: %+v\nactual: %+v",
		expectedRedirect,
		outputRoute.GetRedirect(),
	)
}

func TestAbsoluteTimeoutIgnored(t *testing.T) {
	timeout := gwv1.Duration("1h")
	cookie := gwv1.CookieBasedSessionPersistence
	header := gwv1.HeaderBasedSessionPersistence
	permanent := gwv1.PermanentCookieLifetimeType
	session := gwv1.SessionCookieLifetimeType

	tests := []struct {
		name string
		sp   *gwv1.SessionPersistence
		want bool
	}{
		{name: "nil", sp: nil, want: false},
		{name: "no timeout", sp: &gwv1.SessionPersistence{Type: &cookie, Cookie: &gwv1.CookieConfig{}}, want: false},
		{
			name: "permanent cookie",
			sp:   &gwv1.SessionPersistence{Type: &cookie, AbsoluteTimeout: &timeout, Cookie: &gwv1.CookieConfig{LifetimeType: &permanent}},
			want: false,
		},
		{
			name: "session cookie",
			sp:   &gwv1.SessionPersistence{Type: &cookie, AbsoluteTimeout: &timeout, Cookie: &gwv1.CookieConfig{LifetimeType: &session}},
			want: true,
		},
		{
			name: "cookie lifetime defaults to session",
			sp:   &gwv1.SessionPersistence{Type: &cookie, AbsoluteTimeout: &timeout, Cookie: &gwv1.CookieConfig{}},
			want: true,
		},
		{name: "type defaults to cookie", sp: &gwv1.SessionPersistence{AbsoluteTimeout: &timeout}, want: true},
		{name: "header", sp: &gwv1.SessionPersistence{Type: &header, AbsoluteTimeout: &timeout}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, absoluteTimeoutIgnored(tt.sp))
		})
	}
}

func TestSessionPersistenceBackendPolicyTakesPrecedence(t *testing.T) {
	routeSP := convertSessionPersistence(&gwv1.SessionPersistence{Cookie: &gwv1.CookieConfig{Name: new(gwv1.CookieName("route-cookie"))}})
	require.NotNil(t, routeSP)
	newRoute := func() *envoyroutev3.Route {
		return &envoyroutev3.Route{Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}}}
	}

	t.Run("route rule applies when the backend has no session persistence", func(t *testing.T) {
		pass := &builtinPluginGwPass{needStatefulSession: map[string]bool{}}
		out := newRoute()
		pass.applyRulePolicy(&ir.RouteContext{FilterChainName: "fc"}, ruleIR{sessionPersistence: routeSP}, policy.MergeOptions{Strategy: policy.AugmentedShallowMerge}, out)
		assert.Contains(t, out.GetTypedPerFilterConfig(), sessionpersistence.FilterName, "route rule session persistence should be set")
		assert.True(t, pass.needStatefulSession["fc"], "filter chain should need the stateful session filter")
	})

	t.Run("backend session persistence wins over the route rule", func(t *testing.T) {
		backendSP, err := sessionpersistence.Build(sessionpersistence.Config{Type: sessionpersistence.Cookie, Name: "policy-cookie"})
		require.NoError(t, err)
		pCtx := &ir.RouteContext{FilterChainName: "fc"}
		pCtx.TypedFilterConfig.AddTypedConfig(sessionpersistence.FilterName, backendSP)

		pass := &builtinPluginGwPass{needStatefulSession: map[string]bool{}}
		out := newRoute()
		pass.applyRulePolicy(pCtx, ruleIR{sessionPersistence: routeSP}, policy.MergeOptions{Strategy: policy.AugmentedShallowMerge}, out)
		assert.NotContains(t, out.GetTypedPerFilterConfig(), sessionpersistence.FilterName, "route rule session persistence should be skipped")
		assert.Equal(t, backendSP, pCtx.TypedFilterConfig.GetTypedConfig(sessionpersistence.FilterName), "backend session persistence should be kept")
	})
}

func unmarshalPerRoute(t *testing.T, a *anypb.Any) *stateful_sessionv3.StatefulSessionPerRoute {
	t.Helper()
	if a == nil {
		return nil
	}
	out := &stateful_sessionv3.StatefulSessionPerRoute{}
	require.NoError(t, a.UnmarshalTo(out))
	return out
}

func TestConvertSessionPersistenceCookiePath(t *testing.T) {
	tests := []struct {
		name string
		path *string
		want string
	}{
		{name: "unset defaults to root", path: nil, want: "/"},
		{name: "empty defaults to root", path: new(""), want: "/"},
		{name: "set is honored", path: new("/app"), want: "/app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			perRoute := unmarshalPerRoute(t, convertSessionPersistence(&gwv1.SessionPersistence{Cookie: &gwv1.CookieConfig{Path: tt.path}}))
			require.NotNil(t, perRoute, "session persistence should be converted")
			state := &stateful_cookie.CookieBasedSessionState{}
			require.NoError(t, perRoute.GetStatefulSession().GetSessionState().GetTypedConfig().UnmarshalTo(state))
			assert.Equal(t, tt.want, state.GetCookie().GetPath())
		})
	}
	assert.Nil(t, convertSessionPersistence(nil), "nil session persistence")
}

func TestSessionPersistenceToAPI(t *testing.T) {
	tests := []struct {
		name string
		in   *gwv1.SessionPersistence
		want *kgateway.SessionPersistence
	}{
		{
			name: "defaults",
			in:   &gwv1.SessionPersistence{},
			want: &kgateway.SessionPersistence{
				Type:   new(kgateway.CookieBasedSessionPersistence),
				Cookie: &kgateway.SessionPersistenceCookieConfig{Name: new(kgateway.CookieName("sessionPersistence"))},
			},
		},
		{
			name: "cookie fields are copied",
			in: &gwv1.SessionPersistence{
				AbsoluteTimeout: new(gwv1.Duration("1h")),
				Cookie: &gwv1.CookieConfig{
					Name: new(gwv1.CookieName("s")), Path: new("/app"), LifetimeType: new(gwv1.PermanentCookieLifetimeType),
				},
			},
			want: &kgateway.SessionPersistence{
				Type:            new(kgateway.CookieBasedSessionPersistence),
				AbsoluteTimeout: new(gwv1.Duration("1h")),
				Cookie: &kgateway.SessionPersistenceCookieConfig{
					Name: new(kgateway.CookieName("s")), Path: new("/app"), LifetimeType: new(kgateway.PermanentCookieLifetimeType),
				},
			},
		},
		{
			name: "timeout without lifetime stays a session cookie",
			in:   &gwv1.SessionPersistence{AbsoluteTimeout: new(gwv1.Duration("1h")), Cookie: &gwv1.CookieConfig{}},
			want: &kgateway.SessionPersistence{
				Type:            new(kgateway.CookieBasedSessionPersistence),
				AbsoluteTimeout: new(gwv1.Duration("1h")),
				Cookie:          &kgateway.SessionPersistenceCookieConfig{Name: new(kgateway.CookieName("sessionPersistence"))},
			},
		},
		{
			name: "invalid timeout is dropped",
			in:   &gwv1.SessionPersistence{AbsoluteTimeout: new(gwv1.Duration("bogus"))},
			want: &kgateway.SessionPersistence{
				Type:   new(kgateway.CookieBasedSessionPersistence),
				Cookie: &kgateway.SessionPersistenceCookieConfig{Name: new(kgateway.CookieName("sessionPersistence"))},
			},
		},
		{
			name: "header defaults the name",
			in:   &gwv1.SessionPersistence{Type: new(gwv1.HeaderBasedSessionPersistence), AbsoluteTimeout: new(gwv1.Duration("1h"))},
			want: &kgateway.SessionPersistence{
				Type:            new(kgateway.HeaderBasedSessionPersistence),
				AbsoluteTimeout: new(gwv1.Duration("1h")),
				Header:          &kgateway.SessionPersistenceHeaderConfig{Name: "x-session-persistence"},
			},
		},
		{
			name: "header name is copied",
			in:   &gwv1.SessionPersistence{Type: new(gwv1.HeaderBasedSessionPersistence), Header: &gwv1.HeaderConfig{Name: "x-s"}},
			want: &kgateway.SessionPersistence{
				Type:   new(kgateway.HeaderBasedSessionPersistence),
				Header: &kgateway.SessionPersistenceHeaderConfig{Name: "x-s"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sessionPersistenceToAPI(tt.in))
		})
	}
}

func TestReportSessionPersistenceOverridden(t *testing.T) {
	routeSP := convertSessionPersistence(&gwv1.SessionPersistence{Cookie: &gwv1.CookieConfig{Name: new(gwv1.CookieName("route-cookie"))}})
	backendSP, err := sessionpersistence.Build(sessionpersistence.Config{Type: sessionpersistence.Cookie, Name: "policy-cookie"})
	require.NoError(t, err)
	backendSPAny, err := utils.MessageToAny(backendSP)
	require.NoError(t, err)
	mergeOpts := policy.MergeOptions{Strategy: policy.AugmentedShallowMerge}

	route := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default", UID: "uid"}}
	parentRef := gwv1.ParentReference{Name: "gw"}
	backend := func(name string) ir.HttpBackend {
		obj := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "default", Name: name}, 8080, "", "")
		return ir.HttpBackend{Backend: ir.BackendRefIR{ClusterName: obj.ClusterName(), BackendObject: &obj}}
	}
	ruleCtx := func(index int, backends ...ir.HttpBackend) *ir.RouteContext {
		return &ir.RouteContext{FilterChainName: "fc", In: ir.HttpRouteRuleMatchIR{
			Parent:    &ir.HttpRouteIR{SourceObject: route},
			ParentRef: parentRef,
			RuleIndex: index,
			Backends:  backends,
		}}
	}
	newPass := func() (*builtinPluginGwPass, func() []metav1.Condition) {
		rm := reports.NewReportMap()
		rep := reports.NewReporter(&rm)
		pass := NewGatewayTranslationPass(ir.GwTranslationCtx{}, rep).(*builtinPluginGwPass)
		return pass, func() []metav1.Condition {
			return rep.Route(route).ParentRef(&parentRef).(*reports.ParentRefReport).Conditions
		}
	}
	programmed := func(conds []metav1.Condition) *metav1.Condition {
		return meta.FindStatusCondition(conds, apiconditions.KgatewayConditionProgrammed)
	}
	newRoute := func(clusters ...*envoyroutev3.WeightedCluster_ClusterWeight) *envoyroutev3.Route {
		action := &envoyroutev3.RouteAction{}
		if len(clusters) > 0 {
			action.ClusterSpecifier = &envoyroutev3.RouteAction_WeightedClusters{WeightedClusters: &envoyroutev3.WeightedCluster{Clusters: clusters}}
		}
		return &envoyroutev3.Route{Action: &envoyroutev3.Route_Route{Route: action}}
	}

	t.Run("no report without a backend policy", func(t *testing.T) {
		pass, conds := newPass()
		pass.applyRulePolicy(ruleCtx(0, backend("a")), ruleIR{sessionPersistence: routeSP}, mergeOpts, newRoute())
		assert.Nil(t, programmed(conds()), "nothing to report")
	})

	t.Run("single backend and split report the overridden backends of all rules", func(t *testing.T) {
		pass, conds := newPass()

		single := ruleCtx(0, backend("a"))
		single.TypedFilterConfig.AddTypedConfig(sessionpersistence.FilterName, backendSPAny)
		pass.applyRulePolicy(single, ruleIR{sessionPersistence: routeSP}, mergeOpts, newRoute())

		a, b := backend("a"), backend("b")
		split := newRoute(
			&envoyroutev3.WeightedCluster_ClusterWeight{Name: a.Backend.ClusterName, TypedPerFilterConfig: map[string]*anypb.Any{sessionpersistence.FilterName: backendSPAny}},
			&envoyroutev3.WeightedCluster_ClusterWeight{Name: b.Backend.ClusterName},
		)
		pass.applyRulePolicy(ruleCtx(2, a, b), ruleIR{sessionPersistence: routeSP}, mergeOpts, split)

		cond := programmed(conds())
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionTrue, cond.Status, "the route is still programmed")
		assert.Equal(t, apiconditions.KgatewayReasonSessionPersistenceOverridden, cond.Reason)
		assert.Equal(t, "rule 0: sessionPersistence is overridden by the BackendConfigPolicy of Service default/a; "+
			"rule 2: sessionPersistence is overridden by the BackendConfigPolicy of Service default/a", cond.Message,
			"both rules are reported, and only the split backend with a policy")
	})

	t.Run("does not hide a failed rule", func(t *testing.T) {
		pass, conds := newPass()
		pass.reporter.Route(route).ParentRef(&parentRef).SetCondition(reporter.RouteCondition{
			Type:   gwv1.RouteConditionType(apiconditions.KgatewayConditionProgrammed),
			Status: metav1.ConditionFalse,
			Reason: "RouteRuleReplaced",
		})
		single := ruleCtx(1, backend("a"))
		single.TypedFilterConfig.AddTypedConfig(sessionpersistence.FilterName, backendSPAny)
		pass.applyRulePolicy(single, ruleIR{sessionPersistence: routeSP}, mergeOpts, newRoute())
		assert.Equal(t, metav1.ConditionFalse, programmed(conds()).Status, "Programmed=False must not be overwritten")
	})

	t.Run("equal parent refs with different pointers share one message", func(t *testing.T) {
		// e.g. a delegated child route without parentRefs gets a freshly built parent ref per delegating rule
		newRef := func() gwv1.ParentReference {
			return gwv1.ParentReference{
				Group: new(gwv1.Group("gateway.networking.k8s.io")), Kind: new(gwv1.Kind("Gateway")),
				Namespace: new(gwv1.Namespace("default")), Name: "gw",
			}
		}
		pass, _ := newPass()
		for _, index := range []int{0, 1} {
			ctx := ruleCtx(index, backend("a"))
			ctx.In.ParentRef = newRef()
			ctx.TypedFilterConfig.AddTypedConfig(sessionpersistence.FilterName, backendSPAny)
			pass.applyRulePolicy(ctx, ruleIR{sessionPersistence: routeSP}, mergeOpts, newRoute())
		}
		ref := newRef()
		cond := meta.FindStatusCondition(pass.reporter.Route(route).ParentRef(&ref).(*reports.ParentRefReport).Conditions,
			apiconditions.KgatewayConditionProgrammed)
		require.NotNil(t, cond)
		assert.Equal(t, "rule 0: sessionPersistence is overridden by the BackendConfigPolicy of Service default/a; "+
			"rule 1: sessionPersistence is overridden by the BackendConfigPolicy of Service default/a", cond.Message,
			"both rules are reported")
	})

	t.Run("inherited session persistence is reported on the delegating parent route", func(t *testing.T) {
		pass, _ := newPass()
		parentRoute := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "parent", Namespace: "default", UID: "parent-uid"}}
		parentGatewayRef := gwv1.ParentReference{Name: "gw"}
		parentPol := &builtinPlugin{rule: ruleIR{sessionPersistence: routeSP}}
		parentRule := &ir.HttpRouteRuleMatchIR{
			Parent:    &ir.HttpRouteIR{SourceObject: parentRoute},
			ParentRef: parentGatewayRef,
			RuleIndex: 3,
			AttachedPolicies: ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{
				ir.VirtualBuiltInGK: {{PolicyIr: parentPol}},
			}},
		}
		child := ruleCtx(0, backend("a"))
		child.In.DelegatingParent = parentRule
		child.Policy = parentPol
		child.TypedFilterConfig.AddTypedConfig(sessionpersistence.FilterName, backendSPAny)
		require.NoError(t, pass.ApplyForRoute(child, newRoute()))

		parentCond := meta.FindStatusCondition(
			pass.reporter.Route(parentRoute).ParentRef(&parentGatewayRef).(*reports.ParentRefReport).Conditions,
			apiconditions.KgatewayConditionProgrammed)
		require.NotNil(t, parentCond, "the parent route owns the overridden sessionPersistence")
		assert.Equal(t, "rule 3: sessionPersistence is overridden by the BackendConfigPolicy of Service default/a", parentCond.Message)
		assert.Nil(t, meta.FindStatusCondition(
			pass.reporter.Route(route).ParentRef(&parentRef).(*reports.ParentRefReport).Conditions,
			apiconditions.KgatewayConditionProgrammed), "the child route has no sessionPersistence of its own")
	})

	t.Run("only the effective rule session persistence is reported", func(t *testing.T) {
		pass, childConds := newPass()
		parentRoute := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "parent", Namespace: "default", UID: "parent-uid"}}
		parentGatewayRef := gwv1.ParentReference{Name: "gw"}
		parentPol := &builtinPlugin{rule: ruleIR{sessionPersistence: routeSP}}
		childPol := &builtinPlugin{rule: ruleIR{sessionPersistence: routeSP}}
		parentRule := &ir.HttpRouteRuleMatchIR{
			Parent:    &ir.HttpRouteIR{SourceObject: parentRoute},
			ParentRef: parentGatewayRef,
			RuleIndex: 3,
			AttachedPolicies: ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{
				ir.VirtualBuiltInGK: {{PolicyIr: parentPol}},
			}},
		}
		child := ruleCtx(0, backend("a"))
		child.In.DelegatingParent = parentRule
		child.In.AttachedPolicies = ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{
			ir.VirtualBuiltInGK: {{PolicyIr: childPol}},
		}}
		child.TypedFilterConfig.AddTypedConfig(sessionpersistence.FilterName, backendSPAny)
		out := newRoute()

		// Rule policies are applied from highest (the child's own) to lowest (inherited) priority.
		child.Policy = childPol
		require.NoError(t, pass.ApplyForRoute(child, out))
		child.Policy = parentPol
		require.NoError(t, pass.ApplyForRoute(child, out))

		cond := programmed(childConds())
		require.NotNil(t, cond, "the child rule's own sessionPersistence is the effective one")
		assert.Equal(t, "rule 0: sessionPersistence is overridden by the BackendConfigPolicy of Service default/a", cond.Message)
		assert.Nil(t, meta.FindStatusCondition(
			pass.reporter.Route(parentRoute).ParentRef(&parentGatewayRef).(*reports.ParentRefReport).Conditions,
			apiconditions.KgatewayConditionProgrammed), "the parent's sessionPersistence would not apply anyway")
	})

	t.Run("no reporter", func(t *testing.T) {
		pass := &builtinPluginGwPass{needStatefulSession: map[string]bool{}}
		single := ruleCtx(0, backend("a"))
		single.TypedFilterConfig.AddTypedConfig(sessionpersistence.FilterName, backendSPAny)
		assert.NotPanics(t, func() { pass.applyRulePolicy(single, ruleIR{sessionPersistence: routeSP}, mergeOpts, newRoute()) })
	})
}
