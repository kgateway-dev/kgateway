package routepolicy

import (
	"context"
	"testing"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
)

func TestExtAuthPolicyTranslation(t *testing.T) {
	t.Run("applies ext auth configuration to route", func(t *testing.T) {
		// Setup
		plugin := &routePolicyPluginGwPass{}
		ctx := context.Background()
		policy := &routePolicy{
			spec: routeSpecIr{
				extAuth: &extAuthIR{
					filter: &envoy_ext_authz_v3.ExtAuthz{
						FailureModeAllow: true,
						WithRequestBody: &envoy_ext_authz_v3.BufferSettings{
							MaxRequestBytes: 1024,
						},
					},
					providerName: "test-auth-provider",
				},
			},
		}
		pCtx := &ir.RouteContext{
			Policy: policy,
		}
		outputRoute := &envoy_config_route_v3.Route{}

		// Execute
		err := plugin.ApplyForRoute(ctx, pCtx, outputRoute)

		// Verify
		require.NoError(t, err)
		require.NotNil(t, outputRoute.TypedPerFilterConfig)
		extAuthConfig, ok := outputRoute.TypedPerFilterConfig["test-auth-provider"]
		assert.True(t, ok)
		assert.NotNil(t, extAuthConfig)
	})

	t.Run("configures listener with ext auth", func(t *testing.T) {
		// Setup
		plugin := &routePolicyPluginGwPass{}
		ctx := context.Background()
		policy := &routePolicy{
			spec: routeSpecIr{
				extAuth: &extAuthIR{
					filter: &envoy_ext_authz_v3.ExtAuthz{
						FailureModeAllow: true,
					},
					providerName: "test-auth-provider",
				},
			},
		}
		pCtx := &ir.ListenerContext{
			Policy: policy,
		}
		listener := &envoy_config_listener_v3.Listener{
			FilterChains: []*envoy_config_listener_v3.FilterChain{
				{
					Filters: []*envoy_config_listener_v3.Filter{
						{
							Name: "envoy.filters.network.http_connection_manager",
							ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
								TypedConfig: &anypb.Any{
									TypeUrl: "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
								},
							},
						},
					},
				},
			},
		}

		// Execute
		plugin.ApplyListenerPlugin(ctx, pCtx, listener)

		// Verify
		assert.True(t, plugin.extAuthListenerEnabled)
		assert.NotNil(t, plugin.extAuth)
		assert.Equal(t, "test-auth-provider", plugin.extAuth.providerName)
	})

	t.Run("adds ext auth filter to filter chain", func(t *testing.T) {
		// Setup
		plugin := &routePolicyPluginGwPass{
			extAuth: &extAuthIR{
				filter: &envoy_ext_authz_v3.ExtAuthz{
					FailureModeAllow: true,
				},
				providerName: "test-auth-provider",
			},
		}
		ctx := context.Background()
		fcc := ir.FilterChainCommon{}

		// Execute
		filters, err := plugin.HttpFilters(ctx, fcc)

		// Verify
		require.NoError(t, err)
		require.NotNil(t, filters)
		assert.Equal(t, 1, len(filters))
		assert.Equal(t, plugins.AuthZStage, filters[0].Stage)
	})

	t.Run("handles disabled ext auth configuration", func(t *testing.T) {
		// Setup
		plugin := &routePolicyPluginGwPass{}
		ctx := context.Background()
		policy := &routePolicy{
			spec: routeSpecIr{
				extAuth: nil,
			},
		}
		pCtx := &ir.RouteContext{
			Policy: policy,
		}
		outputRoute := &envoy_config_route_v3.Route{}

		// Execute
		err := plugin.ApplyForRoute(ctx, pCtx, outputRoute)

		// Verify
		require.NoError(t, err)
		assert.Nil(t, outputRoute.TypedPerFilterConfig)
	})

	t.Run("handles multiple ext auth configurations", func(t *testing.T) {
		// Setup
		plugin := &routePolicyPluginGwPass{}
		ctx := context.Background()
		policy := &routePolicy{
			spec: routeSpecIr{
				extAuth: &extAuthIR{
					filter: &envoy_ext_authz_v3.ExtAuthz{
						FailureModeAllow: true,
					},
					providerName: "test-auth-provider-1",
				},
			},
		}
		pCtx := &ir.RouteContext{
			Policy: policy,
		}
		outputRoute := &envoy_config_route_v3.Route{}

		// Execute first configuration
		err := plugin.ApplyForRoute(ctx, pCtx, outputRoute)
		require.NoError(t, err)

		// Apply second configuration
		policy.spec.extAuth.providerName = "test-auth-provider-2"
		err = plugin.ApplyForRoute(ctx, pCtx, outputRoute)
		require.NoError(t, err)

		// Verify
		require.NotNil(t, outputRoute.TypedPerFilterConfig)
		_, ok1 := outputRoute.TypedPerFilterConfig["test-auth-provider-1"]
		_, ok2 := outputRoute.TypedPerFilterConfig["test-auth-provider-2"]
		assert.True(t, ok1)
		assert.True(t, ok2)
	})
}
