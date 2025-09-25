package trafficpolicy

import (
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

func TestAutoHostRewriteIREquals(t *testing.T) {
	tests := []struct {
		name     string
		rewrite1 *autoHostRewriteIR
		rewrite2 *autoHostRewriteIR
		expected bool
	}{
		{
			name:     "both nil are equal",
			rewrite1: nil,
			rewrite2: nil,
			expected: true,
		},
		{
			name:     "nil vs non-nil are not equal",
			rewrite1: nil,
			rewrite2: &autoHostRewriteIR{enabled: wrapperspb.Bool(true)},
			expected: false,
		},
		{
			name:     "non-nil vs nil are not equal",
			rewrite1: &autoHostRewriteIR{enabled: wrapperspb.Bool(true)},
			rewrite2: nil,
			expected: false,
		},
		{
			name:     "same instance is equal",
			rewrite1: &autoHostRewriteIR{enabled: wrapperspb.Bool(true)},
			rewrite2: &autoHostRewriteIR{enabled: wrapperspb.Bool(true)},
			expected: true,
		},
		{
			name:     "same false values are equal",
			rewrite1: &autoHostRewriteIR{enabled: wrapperspb.Bool(false)},
			rewrite2: &autoHostRewriteIR{enabled: wrapperspb.Bool(false)},
			expected: true,
		},
		{
			name:     "different values are not equal",
			rewrite1: &autoHostRewriteIR{enabled: wrapperspb.Bool(true)},
			rewrite2: &autoHostRewriteIR{enabled: wrapperspb.Bool(false)},
			expected: false,
		},
		{
			name:     "nil proto fields are equal",
			rewrite1: &autoHostRewriteIR{enabled: nil},
			rewrite2: &autoHostRewriteIR{enabled: nil},
			expected: true,
		},
		{
			name:     "nil vs non-nil proto fields are not equal",
			rewrite1: &autoHostRewriteIR{enabled: nil},
			rewrite2: &autoHostRewriteIR{enabled: wrapperspb.Bool(true)},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.rewrite1.Equals(tt.rewrite2)
			assert.Equal(t, tt.expected, result)

			// Test symmetry: a.Equals(b) should equal b.Equals(a)
			reverseResult := tt.rewrite2.Equals(tt.rewrite1)
			assert.Equal(t, result, reverseResult, "Equals should be symmetric")
		})
	}

	// Test reflexivity: x.Equals(x) should always be true for non-nil values
	t.Run("reflexivity", func(t *testing.T) {
		rewrite := &autoHostRewriteIR{enabled: wrapperspb.Bool(true)}
		assert.True(t, rewrite.Equals(rewrite), "rewrite should equal itself")
	})

	// Test transitivity: if a.Equals(b) && b.Equals(c), then a.Equals(c)
	t.Run("transitivity", func(t *testing.T) {
		a := &autoHostRewriteIR{enabled: wrapperspb.Bool(false)}
		b := &autoHostRewriteIR{enabled: wrapperspb.Bool(false)}
		c := &autoHostRewriteIR{enabled: wrapperspb.Bool(false)}

		assert.True(t, a.Equals(b), "a should equal b")
		assert.True(t, b.Equals(c), "b should equal c")
		assert.True(t, a.Equals(c), "a should equal c (transitivity)")
	})
}

func TestApplyForRoute_SetsRouteActionFlag(t *testing.T) {
	plugin := &trafficPolicyPluginGwPass{}

	t.Run("autoHostRewrite true → RouteAction flag set", func(t *testing.T) {
		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
				autoHostRewrite: &autoHostRewriteIR{
					enabled: wrapperspb.Bool(true),
				},
			},
		}

		pCtx := &ir.RouteContext{Policy: policy}
		out := &envoyroutev3.Route{
			Action: &envoyroutev3.Route_Route{
				Route: &envoyroutev3.RouteAction{},
			},
		}

		require.NoError(t, plugin.ApplyForRoute(pCtx, out))

		ra := out.GetRoute()
		require.NotNil(t, ra)
		assert.NotNil(t, ra.GetAutoHostRewrite())
		assert.True(t, ra.GetAutoHostRewrite().GetValue())
	})

	t.Run("autoHostRewrite nil → RouteAction untouched", func(t *testing.T) {
		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{autoHostRewrite: nil},
		}
		pCtx := &ir.RouteContext{Policy: policy}
		out := &envoyroutev3.Route{
			Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}},
		}

		require.NoError(t, plugin.ApplyForRoute(pCtx, out))

		ra := out.GetRoute()
		require.NotNil(t, ra)
		assert.Nil(t, ra.HostRewriteSpecifier) // nothing written
	})
}
