package routepolicy

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
)

func TestExtAuthForSpec(t *testing.T) {
	truth := true
	truthy := &truth
	t.Run("creates basic ext auth configuration in one pass", func(t *testing.T) {

		// Setup
		spec := &v1alpha1.RoutePolicySpec{ExtAuth: &v1alpha1.ExtAuthRoutePolicy{
			ExtensionRef: &v1.LocalObjectReference{
				Name: "test-extension",
			},
			EmitFilterStateStats: truthy,
		},
		}
		out := &routeSpecIr{}

		// Execute
		extAuthForSpec(spec, out)

		// Verify
		require.NotNil(t, out.extAuth)
		assert.Equal(t, "test-extension", out.extAuth.providerName)
		assert.NotNil(t, out.extAuth.filter)
	})
	t.Run("configures failure mode allow", func(t *testing.T) {
		// Setup
		truthy := true
		spec := &v1alpha1.RoutePolicySpec{ExtAuth: &v1alpha1.ExtAuthRoutePolicy{
			ExtensionRef: &v1.LocalObjectReference{
				Name: "test-extension",
			},
			FailureModeAllow: &truthy,
		},
		}
		out := &routeSpecIr{}

		// Execute
		extAuthForSpec(spec, out)

		// Verify
		require.NotNil(t, out.extAuth)
		assert.True(t, out.extAuth.filter.FailureModeAllow)
	})

	t.Run("configures request body settings", func(t *testing.T) {
		truthy := true
		// Setup
		spec := &v1alpha1.RoutePolicySpec{ExtAuth: &v1alpha1.ExtAuthRoutePolicy{
			ExtensionRef: &v1.LocalObjectReference{
				Name: "test-extension",
			},
			WithRequestBody: &v1alpha1.BufferSettings{
				MaxRequestBytes:     1024,
				AllowPartialMessage: &truthy,
				PackAsBytes:         &truthy,
			},
		},
		}
		out := &routeSpecIr{}

		// Execute
		extAuthForSpec(spec, out)

		// Verify
		require.NotNil(t, out.extAuth)
		require.NotNil(t, out.extAuth.filter.WithRequestBody)
		assert.Equal(t, uint32(1024), out.extAuth.filter.WithRequestBody.MaxRequestBytes)
		assert.True(t, out.extAuth.filter.WithRequestBody.AllowPartialMessage)
		assert.True(t, out.extAuth.filter.WithRequestBody.PackAsBytes)
	})

	t.Run("configures metadata context namespaces", func(t *testing.T) {
		// Setup
		spec := &v1alpha1.RoutePolicySpec{ExtAuth: &v1alpha1.ExtAuthRoutePolicy{
			ExtensionRef: &v1.LocalObjectReference{
				Name: "test-extension",
			},
			MetadataContextNamespaces: []string{"jwt", "custom"},
		},
		}
		out := &routeSpecIr{}

		// Execute
		extAuthForSpec(spec, out)

		// Verify
		require.NotNil(t, out.extAuth)
		assert.Equal(t, []string{"jwt", "custom"}, out.extAuth.filter.MetadataContextNamespaces)
	})

	t.Run("configures TLS settings", func(t *testing.T) {
		// Setup
		truthy := true
		spec := &v1alpha1.RoutePolicySpec{ExtAuth: &v1alpha1.ExtAuthRoutePolicy{
			ExtensionRef: &v1.LocalObjectReference{
				Name: "test-extension",
			},
			IncludePeerCertificate: &truthy,
			IncludeTLSSession:      &truthy,
		},
		}
		out := &routeSpecIr{}

		// Execute
		extAuthForSpec(spec, out)

		// Verify
		require.NotNil(t, out.extAuth)
		assert.True(t, out.extAuth.filter.IncludePeerCertificate)
		assert.True(t, out.extAuth.filter.IncludeTlsSession)
	})
}

func TestApplyForRoute(t *testing.T) {
	t.Run("applies ext auth configuration to route", func(t *testing.T) {
		// Setup
		plugin := &routePolicyPluginGwPass{}
		ctx := context.Background()
		policy := &routePolicy{
			spec: routeSpecIr{
				extAuth: &extAuthIR{
					filter: &envoy_ext_authz_v3.ExtAuthz{
						FailureModeAllow: true,
					},
					providerName: "test-extension",
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
		extAuthConfig, ok := outputRoute.TypedPerFilterConfig["test-extension"]
		assert.True(t, ok)
		assert.NotNil(t, extAuthConfig)
	})

	t.Run("handles nil ext auth configuration", func(t *testing.T) {
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
}

func TestApplyListenerPlugin(t *testing.T) {
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
					providerName: "test-extension",
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
		assert.Equal(t, "test-extension", plugin.extAuth.providerName)
	})
}

func TestHttpFilters(t *testing.T) {
	t.Run("adds ext auth filter to filter chain", func(t *testing.T) {
		// Setup
		plugin := &routePolicyPluginGwPass{
			extAuth: &extAuthIR{
				filter: &envoy_ext_authz_v3.ExtAuthz{
					FailureModeAllow: true,
				},
				providerName: "test-extension",
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
}

func TestExtAuthPolicyPlugin(t *testing.T) {
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
					providerName: "test-auth-extension",
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
		extAuthConfig, ok := outputRoute.TypedPerFilterConfig["test-auth-extension"]
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
					providerName: "test-auth-extension",
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
		assert.Equal(t, "test-auth-extension", plugin.extAuth.providerName)
	})

	t.Run("adds ext auth filter to filter chain", func(t *testing.T) {
		// Setup
		plugin := &routePolicyPluginGwPass{
			extAuth: &extAuthIR{
				filter: &envoy_ext_authz_v3.ExtAuthz{
					FailureModeAllow: true,
				},
				providerName: "test-auth-extension",
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
					providerName: "test-auth-extension-1",
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
		policy.spec.extAuth.providerName = "test-auth-extension-2"
		err = plugin.ApplyForRoute(ctx, pCtx, outputRoute)
		require.NoError(t, err)

		// Verify
		require.NotNil(t, outputRoute.TypedPerFilterConfig)
		_, ok1 := outputRoute.TypedPerFilterConfig["test-auth-extension-1"]
		_, ok2 := outputRoute.TypedPerFilterConfig["test-auth-extension-2"]
		assert.True(t, ok1)
		assert.True(t, ok2)
	})
}
