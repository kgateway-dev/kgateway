package trafficpolicy

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
)

func TestExtAuthForSpec(t *testing.T) {
	t.Run("configures request body settings", func(t *testing.T) {
		truthy := true
		// Setup
		spec := &v1alpha1.TrafficPolicy{Spec: v1alpha1.TrafficPolicySpec{
			ExtAuth: &v1alpha1.ExtAuthPolicy{
				ExtensionRef: &corev1.LocalObjectReference{
					Name: "test-extension",
				},
				WithRequestBody: &v1alpha1.BufferSettings{
					MaxRequestBytes:     1024,
					AllowPartialMessage: &truthy,
					PackAsBytes:         &truthy,
				},
			},
		}}

		// Execute
		extauthPerRoute := translatePerFilterConfig(spec.Spec.ExtAuth)

		// Verify
		require.NotNil(t, extauthPerRoute)
		require.NotNil(t, extauthPerRoute.GetCheckSettings().WithRequestBody)
		assert.Equal(t, uint32(1024), extauthPerRoute.GetCheckSettings().WithRequestBody.MaxRequestBytes)
		assert.True(t, extauthPerRoute.GetCheckSettings().WithRequestBody.AllowPartialMessage)
		assert.True(t, extauthPerRoute.GetCheckSettings().WithRequestBody.PackAsBytes)
	})
}

func TestApplyForRoute(t *testing.T) {
	t.Run("applies ext auth configuration to route", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{}
		ctx := context.Background()
		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
				extAuth: &extAuthIR{
					provider: &TrafficPolicyGatewayExtensionIR{
						Name:    "test-extension",
						ExtType: v1alpha1.GatewayExtensionTypeExtAuth,
						ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
							FailureModeAllow: true,
						},
					},
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
		require.NotNil(t, pCtx.TypedFilterConfig)
		extAuthConfig, ok := pCtx.TypedFilterConfig[extAuthFilterName("test-extension")]
		assert.True(t, ok)
		assert.NotNil(t, extAuthConfig)
	})

	t.Run("handles nil ext auth configuration", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{}
		ctx := context.Background()
		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
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
		assert.Nil(t, pCtx.TypedFilterConfig)
	})
}

func TestHttpFilters(t *testing.T) {
	t.Run("adds ext auth filter to filter chain", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{
			extAuthPerProvider: ProviderNeededMap{
				Providers: map[string]map[string]*TrafficPolicyGatewayExtensionIR{
					"test-filter-chain": {
						"test-extension": {
							Name:    "test-extension",
							ExtType: v1alpha1.GatewayExtensionTypeExtAuth,
							ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
								FailureModeAllow: true,
							},
						},
					},
				},
			},
		}
		ctx := context.Background()
		fcc := ir.FilterChainCommon{
			FilterChainName: "test-filter-chain",
		}

		// Execute
		filters, err := plugin.HttpFilters(ctx, fcc)

		// Verify
		require.NoError(t, err)
		require.NotNil(t, filters)
		assert.Equal(t, 2, len(filters)) // extauth and metadata filter
		assert.Equal(t, plugins.DuringStage(plugins.AuthZStage), filters[1].Stage)
	})
}

func TestExtAuthPolicyPlugin(t *testing.T) {
	t.Run("applies ext auth configuration to route", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{}
		ctx := context.Background()
		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
				extAuth: &extAuthIR{
					provider: &TrafficPolicyGatewayExtensionIR{
						Name:    "test-auth-extension",
						ExtType: v1alpha1.GatewayExtensionTypeExtAuth,
						ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
							FailureModeAllow: true,
							WithRequestBody: &envoy_ext_authz_v3.BufferSettings{
								MaxRequestBytes: 1024,
							},
						},
					},
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
		require.NotNil(t, pCtx.TypedFilterConfig)
		extAuthConfig, ok := pCtx.TypedFilterConfig[extAuthFilterName("test-auth-extension")]
		assert.True(t, ok)
		assert.NotNil(t, extAuthConfig)
		assert.Empty(t, pCtx.TypedFilterConfig[extAuthGlobalDisableFilterName])
	})

	t.Run("handles disabled ext auth configuration", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{}
		ctx := context.Background()
		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
				extAuth: &extAuthIR{
					enablement: v1alpha1.ExtAuthDisableAll,
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
		// assert.NotNil(t, )
		assert.NotNil(t, pCtx.TypedFilterConfig, pCtx)
		assert.NotEmpty(t, pCtx.TypedFilterConfig[extAuthGlobalDisableFilterName])
	})
}
