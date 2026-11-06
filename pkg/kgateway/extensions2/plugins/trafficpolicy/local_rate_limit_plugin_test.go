package trafficpolicy

import (
	"testing"
	"time"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestLocalRateLimitIREquals(t *testing.T) {
	createSimpleRateLimit := func(tokensPerSecond uint32) *localratelimitv3.LocalRateLimit {
		return &localratelimitv3.LocalRateLimit{
			TokenBucket: &typev3.TokenBucket{
				MaxTokens:     tokensPerSecond * 10,
				TokensPerFill: wrapperspb.UInt32(tokensPerSecond),
				FillInterval:  durationpb.New(time.Second),
			},
		}
	}
	createRateLimitWithPrefix := func(prefix string) *localratelimitv3.LocalRateLimit {
		return &localratelimitv3.LocalRateLimit{
			StatPrefix: prefix,
		}
	}

	tests := []struct {
		name       string
		rateLimit1 *localRateLimitIR
		rateLimit2 *localRateLimitIR
		expected   bool
	}{
		{
			name:       "both nil are equal",
			rateLimit1: nil,
			rateLimit2: nil,
			expected:   true,
		},
		{
			name:       "nil vs non-nil are not equal",
			rateLimit1: nil,
			rateLimit2: &localRateLimitIR{config: createSimpleRateLimit(100)},
			expected:   false,
		},
		{
			name:       "non-nil vs nil are not equal",
			rateLimit1: &localRateLimitIR{config: createSimpleRateLimit(100)},
			rateLimit2: nil,
			expected:   false,
		},
		{
			name:       "same instance is equal",
			rateLimit1: &localRateLimitIR{config: createSimpleRateLimit(100)},
			rateLimit2: &localRateLimitIR{config: createSimpleRateLimit(100)},
			expected:   true,
		},
		{
			name:       "different token rates are not equal",
			rateLimit1: &localRateLimitIR{config: createSimpleRateLimit(100)},
			rateLimit2: &localRateLimitIR{config: createSimpleRateLimit(200)},
			expected:   false,
		},
		{
			name:       "different stat prefixes are not equal",
			rateLimit1: &localRateLimitIR{config: createRateLimitWithPrefix("prefix1")},
			rateLimit2: &localRateLimitIR{config: createRateLimitWithPrefix("prefix2")},
			expected:   false,
		},
		{
			name:       "same stat prefixes are equal",
			rateLimit1: &localRateLimitIR{config: createRateLimitWithPrefix("prefix1")},
			rateLimit2: &localRateLimitIR{config: createRateLimitWithPrefix("prefix1")},
			expected:   true,
		},
		{
			name:       "nil rate limit config fields are equal",
			rateLimit1: &localRateLimitIR{config: nil},
			rateLimit2: &localRateLimitIR{config: nil},
			expected:   true,
		},
		{
			name:       "nil vs non-nil rate limit config fields are not equal",
			rateLimit1: &localRateLimitIR{config: nil},
			rateLimit2: &localRateLimitIR{config: createSimpleRateLimit(100)},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.rateLimit1.Equals(tt.rateLimit2)
			assert.Equal(t, tt.expected, result)

			// Test symmetry: a.Equals(b) should equal b.Equals(a)
			reverseResult := tt.rateLimit2.Equals(tt.rateLimit1)
			assert.Equal(t, result, reverseResult, "Equals should be symmetric")
		})
	}

	// Test reflexivity: x.Equals(x) should always be true for non-nil values
	t.Run("reflexivity", func(t *testing.T) {
		rateLimit := &localRateLimitIR{config: createSimpleRateLimit(50)}
		assert.True(t, rateLimit.Equals(rateLimit), "rateLimit should equal itself")
	})

	// Test transitivity: if a.Equals(b) && b.Equals(c), then a.Equals(c)
	t.Run("transitivity", func(t *testing.T) {
		createSameRateLimit := func() *localRateLimitIR {
			return &localRateLimitIR{config: createSimpleRateLimit(75)}
		}

		a := createSameRateLimit()
		b := createSameRateLimit()
		c := createSameRateLimit()

		assert.True(t, a.Equals(b), "a should equal b")
		assert.True(t, b.Equals(c), "b should equal c")
		assert.True(t, a.Equals(c), "a should equal c (transitivity)")
	})
}

func TestLocalRateLimitIRValidate(t *testing.T) {
	tests := []struct {
		name      string
		rateLimit *localRateLimitIR
		wantErr   bool
	}{
		{
			name:      "nil rate limit is valid",
			rateLimit: nil,
			wantErr:   false,
		},
		{
			name:      "rate limit with nil config is valid",
			rateLimit: &localRateLimitIR{config: nil},
			wantErr:   false,
		},
		{
			name: "valid rate limit config passes validation",
			rateLimit: &localRateLimitIR{
				config: &localratelimitv3.LocalRateLimit{
					StatPrefix: "test_prefix",
					TokenBucket: &typev3.TokenBucket{
						MaxTokens:     1000,
						TokensPerFill: wrapperspb.UInt32(100),
						FillInterval:  durationpb.New(time.Second),
					},
				},
			},
			wantErr: false,
		},
		{
			name:      "empty rate limit config is valid",
			rateLimit: &localRateLimitIR{},
			wantErr:   false,
		},
		{
			name: "empty rate limit config fails validation",
			rateLimit: &localRateLimitIR{
				config: &localratelimitv3.LocalRateLimit{},
			},
			wantErr: true,
		},
		{
			name: "rate limit config with invalid fill interval fails validation",
			rateLimit: &localRateLimitIR{
				config: &localratelimitv3.LocalRateLimit{
					StatPrefix: "test_prefix",
					TokenBucket: &typev3.TokenBucket{
						MaxTokens:     100,
						TokensPerFill: wrapperspb.UInt32(10),
						FillInterval:  &durationpb.Duration{Seconds: -1},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rateLimit.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestToLocalRateLimitFilterConfigShareAcrossGateway(t *testing.T) {
	tokenBucket := &kgateway.TokenBucket{
		MaxTokens:    100,
		FillInterval: metav1.Duration{Duration: time.Second},
	}

	tests := []struct {
		name               string
		shareAcrossGateway *bool
		wantLocalCluster   bool
	}{
		{
			name:               "unset keeps per-replica rate limiting",
			shareAcrossGateway: nil,
			wantLocalCluster:   false,
		},
		{
			name:               "false keeps per-replica rate limiting",
			shareAcrossGateway: new(false),
			wantLocalCluster:   false,
		},
		{
			name:               "true shares the token bucket across the local cluster",
			shareAcrossGateway: new(true),
			wantLocalCluster:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toLocalRateLimitFilterConfig(&kgateway.LocalRateLimitPolicy{
				TokenBucket:        tokenBucket,
				ShareAcrossGateway: tt.shareAcrossGateway,
			})
			assert.Equal(t, tt.wantLocalCluster, got.GetLocalClusterRateLimit() != nil)
			assert.False(t, got.GetLocalRateLimitPerDownstreamConnection(),
				"per-connection rate limiting is incompatible with local_cluster_rate_limit")
			assert.Equal(t, uint32(100), got.GetTokenBucket().GetMaxTokens())
		})
	}
}

func TestLocalRateLimitStatPrefix(t *testing.T) {
	tokenBucket := &kgateway.TokenBucket{
		MaxTokens:    10,
		FillInterval: metav1.Duration{Duration: time.Second},
	}
	construct := func(statPrefix *string) *localRateLimitIR {
		out := &trafficPolicySpecIr{}
		constructLocalRateLimit(&kgateway.TrafficPolicy{
			Spec: kgateway.TrafficPolicySpec{
				RateLimit: &kgateway.RateLimit{
					Local: &kgateway.LocalRateLimitPolicy{TokenBucket: tokenBucket, StatPrefix: statPrefix},
				},
			},
		}, out)
		require.NotNil(t, out.localRateLimit)
		return out.localRateLimit
	}

	tests := []struct {
		name         string
		statPrefix   *string
		wantPrefix   string
		wantTemplate string
	}{
		{
			name:       "unset keeps the default prefix",
			statPrefix: nil,
			wantPrefix: localRateLimitStatPrefix,
		},
		{
			name:       "literal prefix is set in the IR",
			statPrefix: new("payments.checkout"),
			wantPrefix: "payments.checkout",
		},
		{
			name:       "empty segments in a literal prefix are dropped",
			statPrefix: new(".payments..checkout."),
			wantPrefix: "payments.checkout",
		},
		{
			name:         "templated prefix keeps the default until resolved per route",
			statPrefix:   new("{{route_namespace}}.{{route_name}}"),
			wantPrefix:   localRateLimitStatPrefix,
			wantTemplate: "{{route_namespace}}.{{route_name}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := construct(tt.statPrefix)
			assert.Equal(t, tt.wantPrefix, got.config.GetStatPrefix())
			assert.Equal(t, tt.wantTemplate, got.statPrefixTemplate)
		})
	}
}

func TestLocalRateLimitIREqualsStatPrefixTemplate(t *testing.T) {
	config := &localratelimitv3.LocalRateLimit{StatPrefix: localRateLimitStatPrefix}
	a := &localRateLimitIR{config: config, statPrefixTemplate: "{{route_name}}"}
	b := &localRateLimitIR{config: config, statPrefixTemplate: "{{route_namespace}}"}
	c := &localRateLimitIR{config: config, statPrefixTemplate: "{{route_name}}"}

	assert.False(t, a.Equals(b))
	assert.False(t, b.Equals(a))
	assert.True(t, a.Equals(c))
}

func TestLocalRateLimitIRForRoute(t *testing.T) {
	shared := &localRateLimitIR{
		config:             &localratelimitv3.LocalRateLimit{StatPrefix: localRateLimitStatPrefix},
		statPrefixTemplate: "{{route_namespace}}.{{route_name}}.{{rule_name}}",
	}

	routeA := shared.forRoute("route-a", "ns", "rule0")
	routeB := shared.forRoute("route-b", "ns", "")

	assert.Equal(t, "ns.route-a.rule0", routeA.config.GetStatPrefix())
	assert.Equal(t, "ns.route-b", routeB.config.GetStatPrefix(), "an unnamed rule drops its segment")
	assert.Empty(t, routeA.statPrefixTemplate, "the resolved copy no longer carries a template")

	// The shared IR is used for every route the policy applies to, so it must not change.
	assert.Equal(t, localRateLimitStatPrefix, shared.config.GetStatPrefix())
	assert.Equal(t, "{{route_namespace}}.{{route_name}}.{{rule_name}}", shared.statPrefixTemplate)

	t.Run("resolving to an empty prefix falls back to the default", func(t *testing.T) {
		onlyRule := &localRateLimitIR{
			config:             &localratelimitv3.LocalRateLimit{StatPrefix: localRateLimitStatPrefix},
			statPrefixTemplate: "{{rule_name}}",
		}
		assert.Equal(t, localRateLimitStatPrefix, onlyRule.forRoute("route-a", "ns", "").config.GetStatPrefix())
	})

	t.Run("without a template the IR is returned as is", func(t *testing.T) {
		literal := &localRateLimitIR{config: &localratelimitv3.LocalRateLimit{StatPrefix: "payments"}}
		assert.Same(t, literal, literal.forRoute("route-a", "ns", "rule0"))
	})

	t.Run("nil IR stays nil", func(t *testing.T) {
		var nilIR *localRateLimitIR
		assert.Nil(t, nilIR.forRoute("route-a", "ns", "rule0"))
	})
}

func TestApplyForRouteResolvesLocalRateLimitStatPrefix(t *testing.T) {
	plugin := &trafficPolicyPluginGwPass{}
	policy := &TrafficPolicy{spec: trafficPolicySpecIr{
		localRateLimit: &localRateLimitIR{
			config:             &localratelimitv3.LocalRateLimit{StatPrefix: localRateLimitStatPrefix},
			statPrefixTemplate: "{{route_namespace}}.{{route_name}}",
		},
	}}

	applyFor := func(routeName string) string {
		pCtx := &ir.RouteContext{
			FilterChainName: "listener~80",
			Policy:          policy,
			In: ir.HttpRouteRuleMatchIR{
				Parent: &ir.HttpRouteIR{ObjectSource: ir.ObjectSource{Name: routeName, Namespace: "ns"}},
			},
		}
		out := &envoyroutev3.Route{Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}}}
		require.NoError(t, plugin.ApplyForRoute(pCtx, out))

		config, ok := pCtx.TypedFilterConfig.GetTypedConfig(localRateLimitFilterNamePrefix).(*localratelimitv3.LocalRateLimit)
		require.True(t, ok, "expected a per-route local rate limit config")
		return config.GetStatPrefix()
	}

	assert.Equal(t, "ns.route-a", applyFor("route-a"))
	assert.Equal(t, "ns.route-b", applyFor("route-b"))
	assert.Equal(t, localRateLimitStatPrefix, policy.spec.localRateLimit.config.GetStatPrefix(),
		"resolving for a route must not modify the shared policy IR")
}
