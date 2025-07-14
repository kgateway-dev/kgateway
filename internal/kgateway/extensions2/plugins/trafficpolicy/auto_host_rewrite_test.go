package trafficpolicy

import (
	"context"
	"testing"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

func TestApplyForRoute_SetsRouteActionFlag(t *testing.T) {
	ctx := context.Background()
	plugin := &trafficPolicyPluginGwPass{}

	t.Run("autoHostRewrite true → RouteAction flag set", func(t *testing.T) {
		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
				autoHostRewrite: wrapperspb.Bool(true),
			},
		}

		pCtx := &ir.RouteContext{Policy: policy}
		out := &routev3.Route{
			Action: &routev3.Route_Route{
				Route: &routev3.RouteAction{},
			},
		}

		require.NoError(t, plugin.ApplyForRoute(ctx, pCtx, out))

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
		out := &routev3.Route{
			Action: &routev3.Route_Route{Route: &routev3.RouteAction{}},
		}

		require.NoError(t, plugin.ApplyForRoute(ctx, pCtx, out))

		ra := out.GetRoute()
		require.NotNil(t, ra)
		assert.Nil(t, ra.HostRewriteSpecifier) // nothing written
	})
}
