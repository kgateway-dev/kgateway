package trafficpolicy

import (
	"context"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
)

func TestExtAuthIREquals(t *testing.T) {
	// Helper to create simple extauth configurations for testing
	createSimpleExtAuth := func(disabled bool) *envoy_ext_authz_v3.ExtAuthzPerRoute {
		return &envoy_ext_authz_v3.ExtAuthzPerRoute{
			Override: &envoy_ext_authz_v3.ExtAuthzPerRoute_Disabled{
				Disabled: disabled,
			},
		}
	}
	createProvider := func(name string) *TrafficPolicyGatewayExtensionIR {
		return &TrafficPolicyGatewayExtensionIR{
			Name: name,
			ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
				Services: &envoy_ext_authz_v3.ExtAuthz_GrpcService{
					GrpcService: &envoycorev3.GrpcService{
						TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
							EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
								ClusterName: name,
							},
						},
					},
				},
			},
		}
	}

	tests := []struct {
		name     string
		extauth1 *extAuthIR
		extauth2 *extAuthIR
		expected bool
	}{
		{
			name:     "both nil are equal",
			extauth1: nil,
			extauth2: nil,
			expected: true,
		},
		{
			name:     "nil vs non-nil are not equal",
			extauth1: nil,
			extauth2: &extAuthIR{perRoute: createSimpleExtAuth(false)},
			expected: false,
		},
		{
			name:     "non-nil vs nil are not equal",
			extauth1: &extAuthIR{perRoute: createSimpleExtAuth(false)},
			extauth2: nil,
			expected: false,
		},
		{
			name:     "same instance is equal",
			extauth1: &extAuthIR{perRoute: createSimpleExtAuth(false)},
			extauth2: &extAuthIR{perRoute: createSimpleExtAuth(false)},
			expected: true,
		},
		{
			name:     "different disabled settings are not equal",
			extauth1: &extAuthIR{perRoute: createSimpleExtAuth(true)},
			extauth2: &extAuthIR{perRoute: createSimpleExtAuth(false)},
			expected: false,
		},
		{
			name:     "different providers are not equal",
			extauth1: &extAuthIR{provider: createProvider("service1")},
			extauth2: &extAuthIR{provider: createProvider("service2")},
			expected: false,
		},
		{
			name:     "same providers are equal",
			extauth1: &extAuthIR{provider: createProvider("service1")},
			extauth2: &extAuthIR{provider: createProvider("service1")},
			expected: true,
		},
		{
			name:     "different disablement settings are not equal",
			extauth1: &extAuthIR{disableAllProviders: true},
			extauth2: &extAuthIR{disableAllProviders: false},
			expected: false,
		},
		{
			name:     "same disablement settings are equal",
			extauth1: &extAuthIR{disableAllProviders: true},
			extauth2: &extAuthIR{disableAllProviders: true},
			expected: true,
		},
		{
			name:     "nil extauth fields are equal",
			extauth1: &extAuthIR{perRoute: nil},
			extauth2: &extAuthIR{perRoute: nil},
			expected: true,
		},
		{
			name:     "nil vs non-nil extauth fields are not equal",
			extauth1: &extAuthIR{perRoute: nil},
			extauth2: &extAuthIR{perRoute: createSimpleExtAuth(false)},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.extauth1.Equals(tt.extauth2)
			assert.Equal(t, tt.expected, result)

			// Test symmetry: a.Equals(b) should equal b.Equals(a)
			reverseResult := tt.extauth2.Equals(tt.extauth1)
			assert.Equal(t, result, reverseResult, "Equals should be symmetric")
		})
	}

	// Test reflexivity: x.Equals(x) should always be true for non-nil values
	t.Run("reflexivity", func(t *testing.T) {
		extauth := &extAuthIR{perRoute: createSimpleExtAuth(false)}
		assert.True(t, extauth.Equals(extauth), "extauth should equal itself")
	})

	// Test transitivity: if a.Equals(b) && b.Equals(c), then a.Equals(c)
	t.Run("transitivity", func(t *testing.T) {
		createSameExtAuth := func() *extAuthIR {
			return &extAuthIR{perRoute: createSimpleExtAuth(true)}
		}

		a := createSameExtAuth()
		b := createSameExtAuth()
		c := createSameExtAuth()

		assert.True(t, a.Equals(b), "a should equal b")
		assert.True(t, b.Equals(c), "b should equal c")
		assert.True(t, a.Equals(c), "a should equal c (transitivity)")
	})
}

func TestExtAuthForSpec(t *testing.T) {
	t.Run("configures request body settings", func(t *testing.T) {
		truthy := true
		// Setup
		spec := &v1alpha1.TrafficPolicy{Spec: v1alpha1.TrafficPolicySpec{
			ExtAuth: &v1alpha1.ExtAuthPolicy{
				ExtensionRef: &v1alpha1.NamespacedObjectReference{
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
		extauthPerRoute := buildExtAuthPerRouteFilterConfig(spec.Spec.ExtAuth)

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
		outputRoute := &envoyroutev3.Route{}

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
		outputRoute := &envoyroutev3.Route{}

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
		outputRoute := &envoyroutev3.Route{}

		// Execute
		err := plugin.ApplyForRoute(ctx, pCtx, outputRoute)

		// Verify
		require.NoError(t, err)
		require.NotNil(t, pCtx.TypedFilterConfig)
		extAuthConfig, ok := pCtx.TypedFilterConfig[extAuthFilterName("test-auth-extension")]
		assert.True(t, ok)
		assert.NotNil(t, extAuthConfig)
		assert.Empty(t, pCtx.TypedFilterConfig[ExtAuthGlobalDisableFilterName])
	})

	t.Run("handles disabled ext auth configuration", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{}
		ctx := context.Background()
		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
				extAuth: &extAuthIR{
					disableAllProviders: true,
				},
			},
		}
		pCtx := &ir.RouteContext{
			Policy: policy,
		}
		outputRoute := &envoyroutev3.Route{}

		// Execute
		err := plugin.ApplyForRoute(ctx, pCtx, outputRoute)

		// Verify
		require.NoError(t, err)
		// assert.NotNil(t, )
		assert.NotNil(t, pCtx.TypedFilterConfig, pCtx)
		assert.NotEmpty(t, pCtx.TypedFilterConfig[ExtAuthGlobalDisableFilterName])
	})
}
