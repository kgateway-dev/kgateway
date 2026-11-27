package trafficpolicy

import (
	"fmt"
	"strings"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
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
			extauth2: &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{perRouteConfig: createSimpleExtAuth(false)}}},
			expected: false,
		},
		{
			name:     "non-nil vs nil are not equal",
			extauth1: &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{perRouteConfig: createSimpleExtAuth(false)}}},
			extauth2: nil,
			expected: false,
		},
		{
			name:     "same instance is equal",
			extauth1: &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{perRouteConfig: createSimpleExtAuth(false)}}},
			extauth2: &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{perRouteConfig: createSimpleExtAuth(false)}}},
			expected: true,
		},
		{
			name:     "different disabled settings are not equal",
			extauth1: &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{perRouteConfig: createSimpleExtAuth(true)}}},
			extauth2: &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{perRouteConfig: createSimpleExtAuth(false)}}},
			expected: false,
		},
		{
			name:     "different providers are not equal",
			extauth1: &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{provider: createProvider("service1")}}},
			extauth2: &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{provider: createProvider("service2")}}},
			expected: false,
		},
		{
			name:     "same providers are equal",
			extauth1: &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{provider: createProvider("service1")}}},
			extauth2: &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{provider: createProvider("service1")}}},
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
			extauth1: &extAuthIR{},
			extauth2: &extAuthIR{},
			expected: true,
		},
		{
			name:     "nil vs non-nil extauth fields are not equal",
			extauth1: &extAuthIR{},
			extauth2: &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{perRouteConfig: createSimpleExtAuth(false)}}},
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
}

func TestExtAuthForSpec(t *testing.T) {
	t.Run("configures request body settings", func(t *testing.T) {
		// Setup
		spec := &kgateway.TrafficPolicy{Spec: kgateway.TrafficPolicySpec{
			ExtAuth: &kgateway.ExtAuthPolicy{
				ExtensionRef: &shared.NamespacedObjectReference{
					Name: "test-extension",
				},
				WithRequestBody: &kgateway.ExtAuthBufferSettings{
					MaxRequestBytes:     1024,
					AllowPartialMessage: true,
					PackAsBytes:         true,
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
		plugin := &trafficPolicyPluginGwPass{enableAuthMetadata: true}
		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
				extAuth: &extAuthIR{
					perProviderConfig: []*perProviderExtAuthConfig{
						{
							provider: &TrafficPolicyGatewayExtensionIR{
								Name: "test-extension",
								ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
									FailureModeAllow: true,
								},
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
		err := plugin.ApplyForRoute(pCtx, outputRoute)

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
		err := plugin.ApplyForRoute(pCtx, outputRoute)

		// Verify
		require.NoError(t, err)
		assert.Nil(t, pCtx.TypedFilterConfig)
	})
}

func TestHttpFilters(t *testing.T) {
	t.Run("adds ext auth filter to filter chain", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{
			enableAuthMetadata: true,
			extAuthPerProvider: ProviderNeededMap{
				Providers: map[string][]Provider{
					"test-filter-chain": {
						{
							Name:        "test-extension",
							FilterStage: filters.DuringStage(filters.AuthNStage),
							Extension: &TrafficPolicyGatewayExtensionIR{
								Name: "test-extension",
								ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
									FailureModeAllow: true,
								},
							},
						},
					},
				},
			},
		}
		fcc := ir.FilterChainCommon{
			FilterChainName: "test-filter-chain",
		}

		// Execute
		httpFilters, err := plugin.HttpFilters(ir.HttpFiltersContext{}, fcc)
		// Verify
		require.NoError(t, err)
		require.NotNil(t, httpFilters)
		// global disable filter, auth-enabled metadata filter, then the extauth filter
		assert.Equal(t, 3, len(httpFilters))
		assert.Equal(t, ExtAuthGlobalDisableFilterName, httpFilters[0].Filter.GetName())
		assert.Equal(t, filters.BeforeStage(filters.FaultStage), httpFilters[0].Stage)
		assert.Equal(t, ExtAuthEnabledFilterName, httpFilters[1].Filter.GetName())
		assert.Equal(t, filters.AfterStage(filters.AuthNStage), httpFilters[1].Stage)
		assert.Equal(t, extAuthFilterName("test-extension"), httpFilters[2].Filter.GetName())
		// An ExtAuthz without a gRPC service keeps the historical HTTP placement.
		assert.Equal(t, filters.DuringStage(filters.AuthNStage), httpFilters[2].Stage)
	})
}

func TestMaterializeExtAuthSelectsFilterInstancesPerRoute(t *testing.T) {
	const filterChain = "test-filter-chain"

	grpcProvider := func(name string, metadata bool, weight int32) *TrafficPolicyGatewayExtensionIR {
		extAuth := &envoy_ext_authz_v3.ExtAuthz{
			Services: &envoy_ext_authz_v3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{}},
		}
		if metadata {
			extAuth.MetadataContextNamespaces = []string{JWTAuthnMetadataNamespace}
		}
		return &TrafficPolicyGatewayExtensionIR{Name: name, ExtAuth: extAuth, PrecedenceWeight: weight}
	}

	headerProvider := grpcProvider("ns/header-provider", false, 1)
	metadataProvider := grpcProvider("ns/metadata-provider", true, 10)
	headerLegacyName := extAuthVariantFilterName("ns/header-provider", false)
	headerPostName := extAuthVariantFilterName("ns/header-provider", true)
	metadataLegacyName := extAuthVariantFilterName("ns/metadata-provider", false)
	metadataPostName := extAuthVariantFilterName("ns/metadata-provider", true)

	inheritedConfig := &envoy_ext_authz_v3.ExtAuthzPerRoute{
		Override: &envoy_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &envoy_ext_authz_v3.CheckSettings{
				ContextExtensions: map[string]string{"tenant": "payments", "region": "us"},
				WithRequestBody: &envoy_ext_authz_v3.BufferSettings{
					MaxRequestBytes: 2048,
				},
			},
		},
	}
	overrideConfig := &envoy_ext_authz_v3.ExtAuthzPerRoute{
		Override: &envoy_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &envoy_ext_authz_v3.CheckSettings{
				ContextExtensions: map[string]string{"tenant": "checkout"},
			},
		},
	}

	plugin := &trafficPolicyPluginGwPass{
		extAuthVhostStates: map[*envoyroutev3.VirtualHost]*extAuthScopeState{},
		extAuthRouteStates: map[*envoyroutev3.Route]*extAuthScopeState{},
	}

	legacyRoute := &envoyroutev3.Route{}
	metadataRoute := &envoyroutev3.Route{}
	overrideRoute := &envoyroutev3.Route{}
	vhost := &envoyroutev3.VirtualHost{Routes: []*envoyroutev3.Route{legacyRoute, metadataRoute, overrideRoute}}
	out := &envoyroutev3.RouteConfiguration{VirtualHosts: []*envoyroutev3.VirtualHost{vhost}}

	// The gateway attaches a legacy provider with per-route context. The route
	// only adds a metadata consumer; the gateway provider must still be included
	// in the route's variant selection and keep its context.
	gatewayIR := &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{
		provider:       headerProvider,
		perRouteConfig: inheritedConfig,
	}}}
	routeIR := &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{provider: metadataProvider}}}
	overrideIR := &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{
		{provider: metadataProvider},
		{provider: headerProvider, perRouteConfig: overrideConfig},
	}}

	plugin.handleExtAuth(filterChain, &ir.TypedFilterConfigMap{}, gatewayIR)
	plugin.handleExtAuth(filterChain, &ir.TypedFilterConfigMap{}, routeIR)
	plugin.handleExtAuth(filterChain, &ir.TypedFilterConfigMap{}, overrideIR)
	rcState := plugin.recordExtAuthScope(out, filterChain, gatewayIR)
	plugin.recordExtAuthScope(metadataRoute, filterChain, routeIR)
	plugin.recordExtAuthScope(overrideRoute, filterChain, overrideIR)

	// A route without its own metadata consumer inherits the canonical config
	// written at route-config scope; materialization must not rewrite it.
	rcTyped := ir.TypedFilterConfigMap{}
	plugin.writeExtAuthProtoConfigs(&rcTyped, rcState)
	legacyRoute.TypedPerFilterConfig = rcTyped.ToAnyMap()

	plugin.materializeExtAuth(out)

	disabled := func(t *testing.T, configs map[string]*anypb.Any, name string) bool {
		t.Helper()
		require.Contains(t, configs, name)
		if configs[name].GetTypeUrl() != "type.googleapis.com/envoy.config.route.v3.FilterConfig" {
			return false
		}
		cfg := &envoyroutev3.FilterConfig{}
		require.NoError(t, configs[name].UnmarshalTo(cfg))
		return cfg.GetDisabled()
	}

	// Unrelated routes on the same listener keep the canonical pre-AuthN
	// instances, including the provider inherited from the gateway.
	assert.False(t, disabled(t, legacyRoute.GetTypedPerFilterConfig(), headerLegacyName))
	assert.NotContains(t, legacyRoute.GetTypedPerFilterConfig(), headerPostName)

	// The legacy provider stays before AuthN even when another provider on the
	// route opts into metadata. It therefore keeps receiving the Authorization
	// header, and no post-AuthN instance is registered for it.
	assert.False(t, disabled(t, metadataRoute.GetTypedPerFilterConfig(), headerLegacyName))
	assert.True(t, disabled(t, metadataRoute.GetTypedPerFilterConfig(), metadataLegacyName))
	assert.NotContains(t, metadataRoute.GetTypedPerFilterConfig(), headerPostName)
	assert.False(t, disabled(t, metadataRoute.GetTypedPerFilterConfig(), metadataPostName))

	var inheritedPerRoute envoy_ext_authz_v3.ExtAuthzPerRoute
	require.NoError(t, metadataRoute.GetTypedPerFilterConfig()[headerLegacyName].UnmarshalTo(&inheritedPerRoute))
	assert.Equal(t, "payments", inheritedPerRoute.GetCheckSettings().GetContextExtensions()["tenant"])
	assert.Equal(t, "us", inheritedPerRoute.GetCheckSettings().GetContextExtensions()["region"])
	assert.Equal(t, uint32(2048), inheritedPerRoute.GetCheckSettings().GetWithRequestBody().GetMaxRequestBytes())

	// Envoy merges only context_extensions across scopes. The route's
	// context-only override must not inherit the ancestor's body buffer size.
	var overriddenPerRoute envoy_ext_authz_v3.ExtAuthzPerRoute
	require.NoError(t, overrideRoute.GetTypedPerFilterConfig()[headerLegacyName].UnmarshalTo(&overriddenPerRoute))
	assert.Equal(t, "checkout", overriddenPerRoute.GetCheckSettings().GetContextExtensions()["tenant"])
	assert.Equal(t, "us", overriddenPerRoute.GetCheckSettings().GetContextExtensions()["region"])
	assert.Nil(t, overriddenPerRoute.GetCheckSettings().GetWithRequestBody())

	httpFilters, err := plugin.HttpFilters(ir.HttpFiltersContext{}, ir.FilterChainCommon{FilterChainName: filterChain})
	require.NoError(t, err)

	stages := map[string]filters.HTTPFilterStage{}
	var postAuthNFilters filters.StagedHttpFilterList
	for _, staged := range httpFilters {
		name := staged.Filter.GetName()
		if name == headerLegacyName || name == headerPostName || name == metadataLegacyName || name == metadataPostName {
			stages[name] = staged.Stage
			if staged.Stage == filters.AfterStage(filters.AuthNStage) {
				postAuthNFilters = append(postAuthNFilters, staged)
			}
		}
	}

	assert.Equal(t, filters.DuringStage(filters.AuthNStage), stages[headerLegacyName], "legacy providers stay before JWT")
	assert.Equal(t, filters.DuringStage(filters.AuthNStage), stages[metadataLegacyName])
	assert.Equal(t, filters.AfterStage(filters.AuthNStage), stages[metadataPostName])
	assert.NotContains(t, stages, headerPostName, "legacy providers must not get a post-AuthN instance")

	postAuthNFilters.Sort()
	require.Len(t, postAuthNFilters, 1)
	assert.Equal(t, metadataPostName, postAuthNFilters[0].Filter.GetName())
}

func TestMaterializeExtAuthKeepsConflictingProvidersBeforeAuthN(t *testing.T) {
	const filterChain = "test-filter-chain"

	clearProvider := &TrafficPolicyGatewayExtensionIR{
		Name: "ns/clear-provider",
		ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
			Services:        &envoy_ext_authz_v3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{}},
			ClearRouteCache: true,
		},
	}
	metadataProvider := &TrafficPolicyGatewayExtensionIR{
		Name: "ns/metadata-provider",
		ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
			Services:                  &envoy_ext_authz_v3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{}},
			MetadataContextNamespaces: []string{JWTAuthnMetadataNamespace},
		},
	}

	clearLegacyName := extAuthVariantFilterName("ns/clear-provider", false)
	clearPostName := extAuthVariantFilterName("ns/clear-provider", true)
	metadataLegacyName := extAuthVariantFilterName("ns/metadata-provider", false)
	metadataPostName := extAuthVariantFilterName("ns/metadata-provider", true)

	inheritedConfig := &envoy_ext_authz_v3.ExtAuthzPerRoute{
		Override: &envoy_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &envoy_ext_authz_v3.CheckSettings{
				ContextExtensions: map[string]string{"tenant": "payments"},
			},
		},
	}

	plugin := &trafficPolicyPluginGwPass{}
	route := &envoyroutev3.Route{}
	vhost := &envoyroutev3.VirtualHost{Routes: []*envoyroutev3.Route{route}}
	out := &envoyroutev3.RouteConfiguration{VirtualHosts: []*envoyroutev3.VirtualHost{vhost}}

	// The clear-route-cache provider is inherited from the gateway; the route
	// adds a metadata consumer. Their effective set conflicts, so neither
	// provider may move after JWT: the route keeps the legacy pre-AuthN
	// placement and the conflict is reported on the source policies.
	gatewayIR := &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{
		provider:       clearProvider,
		perRouteConfig: inheritedConfig,
	}}}
	routeIR := &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{provider: metadataProvider}}}
	plugin.handleExtAuth(filterChain, &ir.TypedFilterConfigMap{}, gatewayIR)
	plugin.handleExtAuth(filterChain, &ir.TypedFilterConfigMap{}, routeIR)
	plugin.recordExtAuthScope(out, filterChain, gatewayIR)
	plugin.recordExtAuthScope(route, filterChain, routeIR)
	plugin.materializeExtAuth(out)

	configs := route.GetTypedPerFilterConfig()
	require.Contains(t, configs, clearLegacyName)
	require.Contains(t, configs, metadataLegacyName)
	assert.NotContains(t, configs, clearPostName, "a clear-route-cache provider must not get a post-AuthN instance")
	assert.NotContains(t, configs, metadataPostName, "a conflicting metadata provider must stay before AuthN")

	var clearPerRoute envoy_ext_authz_v3.ExtAuthzPerRoute
	require.NoError(t, configs[clearLegacyName].UnmarshalTo(&clearPerRoute))
	assert.Equal(t, "payments", clearPerRoute.GetCheckSettings().GetContextExtensions()["tenant"])

	var metadataLegacyConfig envoyroutev3.FilterConfig
	require.NoError(t, configs[metadataLegacyName].UnmarshalTo(&metadataLegacyConfig))
	assert.False(t, metadataLegacyConfig.GetDisabled(), "the metadata provider must still run before AuthN")

	httpFilters, err := plugin.HttpFilters(ir.HttpFiltersContext{}, ir.FilterChainCommon{FilterChainName: filterChain})
	require.NoError(t, err)
	names := map[string]filters.HTTPFilterStage{}
	for _, staged := range httpFilters {
		names[staged.Filter.GetName()] = staged.Stage
	}
	assert.Equal(t, filters.DuringStage(filters.AuthNStage), names[clearLegacyName])
	assert.Equal(t, filters.DuringStage(filters.AuthNStage), names[metadataLegacyName])
	assert.NotContains(t, names, metadataPostName)
	assert.NotContains(t, names, clearPostName)
}

// TestFinalizeExtAuthConflictingRouteDisablesProvisionalPostAuthN reproduces
// the double-authorization bug: ApplyForRoute writes a post-AuthN enable before
// an inherited clearRouteCache is known. Finalization must be able to replace
// that provisional enable with an explicit disable once the conflict makes the
// canonical (pre-JWT) instance authoritative.
func TestFinalizeExtAuthConflictingRouteDisablesProvisionalPostAuthN(t *testing.T) {
	const filterChain = "test-filter-chain"

	metadata := &TrafficPolicyGatewayExtensionIR{
		Name: "ns/metadata",
		ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
			Services:                  &envoy_ext_authz_v3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{}},
			MetadataContextNamespaces: []string{JWTAuthnMetadataNamespace},
		},
	}
	clearer := &TrafficPolicyGatewayExtensionIR{
		Name: "ns/clearer",
		ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
			Services:        &envoy_ext_authz_v3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{}},
			ClearRouteCache: true,
		},
	}
	metadataIR := &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{provider: metadata}}}
	clearerIR := &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{provider: clearer}}}

	canonicalName := extAuthVariantFilterName("ns/metadata", false)
	postName := extAuthVariantFilterName("ns/metadata", true)

	plugin := &trafficPolicyPluginGwPass{}
	conflicting := &envoyroutev3.Route{}
	modern := &envoyroutev3.Route{}
	// The clearer is attached to the legacy route's virtual host only; the
	// modern route on the same filter chain does not inherit it.
	legacyVhost := &envoyroutev3.VirtualHost{Routes: []*envoyroutev3.Route{conflicting}}
	modernVhost := &envoyroutev3.VirtualHost{Routes: []*envoyroutev3.Route{modern}}
	out := &envoyroutev3.RouteConfiguration{VirtualHosts: []*envoyroutev3.VirtualHost{legacyVhost, modernVhost}}

	plugin.handleExtAuth(filterChain, &ir.TypedFilterConfigMap{}, metadataIR)
	plugin.handleExtAuth(filterChain, &ir.TypedFilterConfigMap{}, clearerIR)
	plugin.recordExtAuthScope(legacyVhost, filterChain, clearerIR)

	// Simulate ApplyForRoute for both routes: at this point only the route's
	// own metadata policy is visible, so both provisionally select post-AuthN.
	for _, route := range []*envoyroutev3.Route{conflicting, modern} {
		typed := ir.TypedFilterConfigMap{}
		state := plugin.recordExtAuthScope(route, filterChain, metadataIR)
		plugin.writeExtAuthProtoConfigs(&typed, state)
		route.TypedPerFilterConfig = typed.ToAnyMap()
		require.False(t, filterConfigIsDisabled(t, route.GetTypedPerFilterConfig()[postName]),
			"provisional route config must enable post-AuthN before finalization")
	}

	plugin.materializeExtAuth(out)

	// The conflicting route must only run the canonical pre-JWT instance; the
	// provisional post-AuthN enable must become an explicit disable because the
	// modern route registers that instance on the shared filter chain.
	assert.False(t, filterConfigIsDisabled(t, conflicting.GetTypedPerFilterConfig()[canonicalName]),
		"conflicting route must enable the canonical instance")
	assert.True(t, filterConfigIsDisabled(t, conflicting.GetTypedPerFilterConfig()[postName]),
		"finalization must disable the provisional post-AuthN selection")

	// The nonconflicting route keeps using the post-AuthN instance.
	assert.True(t, filterConfigIsDisabled(t, modern.GetTypedPerFilterConfig()[canonicalName]))
	assert.False(t, filterConfigIsDisabled(t, modern.GetTypedPerFilterConfig()[postName]))

	httpFilters, err := plugin.HttpFilters(ir.HttpFiltersContext{}, ir.FilterChainCommon{FilterChainName: filterChain})
	require.NoError(t, err)
	registered := false
	for _, staged := range httpFilters {
		if staged.Filter.GetName() == postName {
			registered = true
		}
	}
	assert.True(t, registered, "the nonconflicting route must register the post-AuthN instance")
}

func TestMaterializeExtAuthOrdersMetadataProvidersByWeight(t *testing.T) {
	const filterChain = "test-filter-chain"

	provider := func(name string, weight int32) *TrafficPolicyGatewayExtensionIR {
		return &TrafficPolicyGatewayExtensionIR{
			Name: name,
			ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
				Services: &envoy_ext_authz_v3.ExtAuthz_GrpcService{
					GrpcService: &envoycorev3.GrpcService{},
				},
				MetadataContextNamespaces: []string{JWTAuthnMetadataNamespace},
			},
			PrecedenceWeight: weight,
		}
	}

	low := provider("ns/low-weight", 1)
	high := provider("ns/high-weight", 10)
	routeIR := &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{provider: low}, {provider: high}}}

	plugin := &trafficPolicyPluginGwPass{}
	route := &envoyroutev3.Route{}
	vhost := &envoyroutev3.VirtualHost{Routes: []*envoyroutev3.Route{route}}
	out := &envoyroutev3.RouteConfiguration{VirtualHosts: []*envoyroutev3.VirtualHost{vhost}}

	plugin.handleExtAuth(filterChain, &ir.TypedFilterConfigMap{}, routeIR)
	plugin.recordExtAuthScope(route, filterChain, routeIR)
	plugin.materializeExtAuth(out)

	httpFilters, err := plugin.HttpFilters(ir.HttpFiltersContext{}, ir.FilterChainCommon{FilterChainName: filterChain})
	require.NoError(t, err)
	var postAuthN filters.StagedHttpFilterList
	for _, staged := range httpFilters {
		if staged.Stage == filters.AfterStage(filters.AuthNStage) &&
			strings.HasSuffix(staged.Filter.GetName(), extAuthPostAuthNSuffix) {
			postAuthN = append(postAuthN, staged)
		}
	}
	postAuthN.Sort()
	require.Len(t, postAuthN, 2)
	assert.Equal(t, extAuthVariantFilterName("ns/high-weight", true), postAuthN[0].Filter.GetName())
	assert.Equal(t, extAuthVariantFilterName("ns/low-weight", true), postAuthN[1].Filter.GetName())
}

func TestEffectiveExtAuthPerRouteConfigMatchesEnvoyMerge(t *testing.T) {
	ancestor := &extAuthScopeState{providers: map[string]*perProviderExtAuthConfig{
		"provider": {perRouteConfig: &envoy_ext_authz_v3.ExtAuthzPerRoute{
			Override: &envoy_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
				CheckSettings: &envoy_ext_authz_v3.CheckSettings{
					ContextExtensions:           map[string]string{"tenant": "payments", "region": "us"},
					WithRequestBody:             &envoy_ext_authz_v3.BufferSettings{MaxRequestBytes: 2048},
					EmitClientSpan:              wrapperspb.Bool(true),
					DisableRequestBodyBuffering: true,
				},
			},
		}},
	}}

	t.Run("context-only override does not inherit body settings", func(t *testing.T) {
		route := &extAuthScopeState{providers: map[string]*perProviderExtAuthConfig{
			"provider": {perRouteConfig: &envoy_ext_authz_v3.ExtAuthzPerRoute{
				Override: &envoy_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
					CheckSettings: &envoy_ext_authz_v3.CheckSettings{
						ContextExtensions: map[string]string{"tenant": "checkout"},
					},
				},
			}},
		}}
		result := effectiveExtAuthPerRouteConfig("provider", []*extAuthScopeState{route, ancestor})
		require.NotNil(t, result)
		settings := result.GetCheckSettings()
		require.NotNil(t, settings)
		assert.Equal(t, map[string]string{"tenant": "checkout", "region": "us"}, settings.GetContextExtensions())
		assert.Nil(t, settings.GetWithRequestBody(), "body settings come only from the most specific scope")
		assert.False(t, settings.GetDisableRequestBodyBuffering())
		assert.True(t, settings.GetEmitClientSpan().GetValue(), "emit_client_span is inherited when unset")
	})

	t.Run("disabled false re-enables with default body settings", func(t *testing.T) {
		route := &extAuthScopeState{providers: map[string]*perProviderExtAuthConfig{
			"provider": {perRouteConfig: &envoy_ext_authz_v3.ExtAuthzPerRoute{
				Override: &envoy_ext_authz_v3.ExtAuthzPerRoute_Disabled{Disabled: false},
			}},
		}}
		result := effectiveExtAuthPerRouteConfig("provider", []*extAuthScopeState{route, ancestor})
		require.NotNil(t, result)
		settings := result.GetCheckSettings()
		require.NotNil(t, settings)
		assert.Equal(t, "payments", settings.GetContextExtensions()["tenant"])
		assert.Nil(t, settings.GetWithRequestBody())
		assert.False(t, settings.GetDisableRequestBodyBuffering())
	})

	t.Run("disabled true wins", func(t *testing.T) {
		route := &extAuthScopeState{providers: map[string]*perProviderExtAuthConfig{
			"provider": {perRouteConfig: &envoy_ext_authz_v3.ExtAuthzPerRoute{
				Override: &envoy_ext_authz_v3.ExtAuthzPerRoute_Disabled{Disabled: true},
			}},
		}}
		result := effectiveExtAuthPerRouteConfig("provider", []*extAuthScopeState{route, ancestor})
		require.NotNil(t, result)
		assert.True(t, result.GetDisabled())
	})
}

func TestFinalizeExtAuthWithoutRouteConfigPolicy(t *testing.T) {
	const filterChain = "test-filter-chain"

	legacy := &TrafficPolicyGatewayExtensionIR{
		Name: "ns/legacy",
		ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
			Services: &envoy_ext_authz_v3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{}},
		},
	}
	metadata := &TrafficPolicyGatewayExtensionIR{
		Name: "ns/metadata",
		ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
			Services:                  &envoy_ext_authz_v3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{}},
			MetadataContextNamespaces: []string{JWTAuthnMetadataNamespace},
		},
	}
	legacyIR := &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{provider: legacy}}}
	metadataIR := &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{provider: metadata}}}

	plugin := &trafficPolicyPluginGwPass{}
	route := &envoyroutev3.Route{}
	vhost := &envoyroutev3.VirtualHost{Routes: []*envoyroutev3.Route{route}}
	out := &envoyroutev3.RouteConfiguration{VirtualHosts: []*envoyroutev3.VirtualHost{vhost}}

	vhostTyped := ir.TypedFilterConfigMap{}
	plugin.handleExtAuth(filterChain, &vhostTyped, legacyIR)
	vhostState := plugin.recordExtAuthScope(vhost, filterChain, legacyIR)
	plugin.writeExtAuthProtoConfigs(&vhostTyped, vhostState)
	vhost.TypedPerFilterConfig = vhostTyped.ToAnyMap()

	routeTyped := ir.TypedFilterConfigMap{}
	plugin.handleExtAuth(filterChain, &routeTyped, metadataIR)
	routeState := plugin.recordExtAuthScope(route, filterChain, metadataIR)
	plugin.writeExtAuthProtoConfigs(&routeTyped, routeState)
	route.TypedPerFilterConfig = routeTyped.ToAnyMap()

	// No ApplyRouteConfigPlugin call: the finalizer still combines both scopes.
	plugin.FinalizeRouteConfig(nil, out)

	configs := route.GetTypedPerFilterConfig()
	require.Contains(t, configs, extAuthVariantFilterName("ns/legacy", false))
	require.Contains(t, configs, extAuthVariantFilterName("ns/metadata", true))
	require.Contains(t, configs, extAuthVariantFilterName("ns/metadata", false))
	require.NotContains(t, configs, extAuthVariantFilterName("ns/legacy", true))

	assert.False(t, filterConfigIsDisabled(t, configs[extAuthVariantFilterName("ns/legacy", false)]),
		"the inherited legacy provider runs once on its canonical instance")
	assert.True(t, filterConfigIsDisabled(t, configs[extAuthVariantFilterName("ns/metadata", false)]))
	assert.False(t, filterConfigIsDisabled(t, configs[extAuthVariantFilterName("ns/metadata", true)]),
		"the metadata provider must run exactly once on its post-AuthN instance")
}

func TestMaterializeExtAuthKeepsBackendAndRouteConfigProvidersEnabled(t *testing.T) {
	const (
		filterChain = "test-filter-chain"
		cluster     = "backend-cluster"
	)

	legacy := &TrafficPolicyGatewayExtensionIR{
		Name: "ns/legacy",
		ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
			Services: &envoy_ext_authz_v3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{}},
		},
	}
	metadata := &TrafficPolicyGatewayExtensionIR{
		Name: "ns/backend-metadata",
		ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
			Services:                  &envoy_ext_authz_v3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{}},
			MetadataContextNamespaces: []string{JWTAuthnMetadataNamespace},
		},
	}
	legacyIR := &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{provider: legacy}}}
	backendIR := &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{provider: metadata}}}

	plugin := &trafficPolicyPluginGwPass{}
	route := &envoyroutev3.Route{
		Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
			ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: cluster},
		}},
	}
	vhost := &envoyroutev3.VirtualHost{Routes: []*envoyroutev3.Route{route}}
	out := &envoyroutev3.RouteConfiguration{VirtualHosts: []*envoyroutev3.VirtualHost{vhost}}

	// Single-backend routes share the backend and route typed filter config map.
	typed := ir.TypedFilterConfigMap{}
	plugin.handleExtAuth(filterChain, &typed, legacyIR)
	rcState := plugin.recordExtAuthScope(out, filterChain, legacyIR)
	plugin.writeExtAuthProtoConfigs(&typed, rcState)
	plugin.handleExtAuth(filterChain, &typed, backendIR)
	backendState := plugin.recordExtAuthBackendScope(route, nil, cluster, filterChain, backendIR)
	plugin.writeExtAuthProtoConfigs(&typed, backendState)
	route.TypedPerFilterConfig = typed.ToAnyMap()

	plugin.materializeExtAuth(out)

	configs := route.GetTypedPerFilterConfig()
	assert.False(t, filterConfigIsDisabled(t, configs[extAuthVariantFilterName("ns/legacy", false)]),
		"the gateway provider must still enforce authorization")
	assert.False(t, filterConfigIsDisabled(t, configs[extAuthVariantFilterName("ns/backend-metadata", true)]),
		"the backend provider must not be disabled by the legacy-only route config")
	assert.True(t, filterConfigIsDisabled(t, configs[extAuthVariantFilterName("ns/backend-metadata", false)]))
}

// TestMaterializeExtAuthKeepsBackendPolicyOnOwningRoute proves a backendRef
// policy does not leak to another route that selects the same cluster. The
// backendRef belongs to one route rule, even though both rules share a Service.
func TestMaterializeExtAuthKeepsBackendPolicyOnOwningRoute(t *testing.T) {
	const (
		filterChain = "test-filter-chain"
		cluster     = "shared-service-cluster"
	)

	metadata := &TrafficPolicyGatewayExtensionIR{
		Name: "ns/private-backend",
		ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
			Services:                  &envoy_ext_authz_v3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{}},
			MetadataContextNamespaces: []string{JWTAuthnMetadataNamespace},
		},
	}
	backendIR := &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{provider: metadata}}}

	plugin := &trafficPolicyPluginGwPass{}
	newRoute := func() *envoyroutev3.Route {
		return &envoyroutev3.Route{
			Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
				ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: cluster},
			}},
		}
	}
	privateRoute, publicRoute := newRoute(), newRoute()
	vhost := &envoyroutev3.VirtualHost{Routes: []*envoyroutev3.Route{privateRoute, publicRoute}}
	out := &envoyroutev3.RouteConfiguration{VirtualHosts: []*envoyroutev3.VirtualHost{vhost}}

	typed := ir.TypedFilterConfigMap{}
	plugin.handleExtAuth(filterChain, &typed, backendIR)
	backendState := plugin.recordExtAuthBackendScope(privateRoute, nil, cluster, filterChain, backendIR)
	plugin.writeExtAuthProtoConfigs(&typed, backendState)
	privateRoute.TypedPerFilterConfig = typed.ToAnyMap()

	plugin.materializeExtAuth(out)

	postName := extAuthVariantFilterName("ns/private-backend", true)
	require.Contains(t, privateRoute.GetTypedPerFilterConfig(), postName)
	assert.False(t, filterConfigIsDisabled(t, privateRoute.GetTypedPerFilterConfig()[postName]))
	assert.NotContains(t, publicRoute.GetTypedPerFilterConfig(), postName,
		"a route that shares the Service but not the backendRef policy must stay unauthenticated")
	assert.Empty(t, publicRoute.GetTypedPerFilterConfig())
}

// TestMaterializeExtAuthWritesWeightedBackendPolicyPerCluster proves a backendRef
// policy on one weighted cluster does not apply to its siblings. Route-level
// ext-auth config would be inherited by every weighted cluster.
func TestMaterializeExtAuthWritesWeightedBackendPolicyPerCluster(t *testing.T) {
	const filterChain = "test-filter-chain"

	metadata := &TrafficPolicyGatewayExtensionIR{
		Name: "ns/canary-extauth",
		ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
			Services:                  &envoy_ext_authz_v3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{}},
			MetadataContextNamespaces: []string{JWTAuthnMetadataNamespace},
		},
	}
	backendIR := &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{provider: metadata}}}

	canary := &envoyroutev3.WeightedCluster_ClusterWeight{Name: "canary", Weight: wrapperspb.UInt32(10)}
	stable := &envoyroutev3.WeightedCluster_ClusterWeight{Name: "stable", Weight: wrapperspb.UInt32(90)}
	route := &envoyroutev3.Route{
		Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
			ClusterSpecifier: &envoyroutev3.RouteAction_WeightedClusters{
				WeightedClusters: &envoyroutev3.WeightedCluster{Clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{canary, stable}},
			},
		}},
	}
	vhost := &envoyroutev3.VirtualHost{Routes: []*envoyroutev3.Route{route}}
	out := &envoyroutev3.RouteConfiguration{VirtualHosts: []*envoyroutev3.VirtualHost{vhost}}

	// Only the canary backendRef attaches the ext-auth policy; weighted routes
	// give each backendRef its own typed filter config.
	canaryTyped := ir.TypedFilterConfigMap{}
	plugin := &trafficPolicyPluginGwPass{}
	plugin.handleExtAuth(filterChain, &canaryTyped, backendIR)
	backendState := plugin.recordExtAuthBackendScope(route, canary, canary.GetName(), filterChain, backendIR)
	plugin.writeExtAuthProtoConfigs(&canaryTyped, backendState)
	canary.TypedPerFilterConfig = canaryTyped.ToAnyMap()

	plugin.materializeExtAuth(out)

	postName := extAuthVariantFilterName("ns/canary-extauth", true)
	require.Contains(t, canary.GetTypedPerFilterConfig(), postName)
	assert.False(t, filterConfigIsDisabled(t, canary.GetTypedPerFilterConfig()[postName]))
	assert.NotContains(t, route.GetTypedPerFilterConfig(), postName,
		"a weighted backendRef policy must not be materialized at route scope")
	assert.NotContains(t, stable.GetTypedPerFilterConfig(), postName,
		"the sibling weighted cluster must not inherit the canary backendRef policy")
}

// TestMaterializeExtAuthKeepsContextsForWeightedRefsToSameService reproduces
// the weighted-context bug: two backendRefs to one Service share a cluster
// name but carry different auth contexts. Each weighted entry must keep its
// own context instead of the last one winning at finalization.
func TestMaterializeExtAuthKeepsContextsForWeightedRefsToSameService(t *testing.T) {
	const (
		filterChain   = "test-filter-chain"
		sharedCluster = "kube_default_example-svc_80"
	)

	provider := &TrafficPolicyGatewayExtensionIR{
		Name: "ns/metadata",
		ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
			Services:                  &envoy_ext_authz_v3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{}},
			MetadataContextNamespaces: []string{JWTAuthnMetadataNamespace},
		},
	}
	policyIRFor := func(rollout string) *extAuthIR {
		return &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{
			provider: provider,
			perRouteConfig: &envoy_ext_authz_v3.ExtAuthzPerRoute{
				Override: &envoy_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
					CheckSettings: &envoy_ext_authz_v3.CheckSettings{
						ContextExtensions: map[string]string{"rollout": rollout},
					},
				},
			},
		}}}
	}

	canary := &envoyroutev3.WeightedCluster_ClusterWeight{Name: sharedCluster, Weight: wrapperspb.UInt32(10)}
	stable := &envoyroutev3.WeightedCluster_ClusterWeight{Name: sharedCluster, Weight: wrapperspb.UInt32(90)}
	route := &envoyroutev3.Route{
		Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
			ClusterSpecifier: &envoyroutev3.RouteAction_WeightedClusters{
				WeightedClusters: &envoyroutev3.WeightedCluster{Clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{canary, stable}},
			},
		}},
	}
	vhost := &envoyroutev3.VirtualHost{Routes: []*envoyroutev3.Route{route}}
	out := &envoyroutev3.RouteConfiguration{VirtualHosts: []*envoyroutev3.VirtualHost{vhost}}

	plugin := &trafficPolicyPluginGwPass{}
	byRef := []struct {
		cw      *envoyroutev3.WeightedCluster_ClusterWeight
		rollout string
	}{{canary, "canary"}, {stable, "stable"}}
	for _, ref := range byRef {
		policyIR := policyIRFor(ref.rollout)
		plugin.handleExtAuth(filterChain, &ir.TypedFilterConfigMap{}, policyIR)
		typed := ir.TypedFilterConfigMap{}
		state := plugin.recordExtAuthBackendScope(route, ref.cw, ref.cw.GetName(), filterChain, policyIR)
		plugin.writeExtAuthProtoConfigs(&typed, state)
		ref.cw.TypedPerFilterConfig = typed.ToAnyMap()
	}

	plugin.materializeExtAuth(out)

	postName := extAuthVariantFilterName("ns/metadata", true)
	for _, ref := range byRef {
		configs := ref.cw.GetTypedPerFilterConfig()
		require.Contains(t, configs, postName, "each weighted ref needs its own post-AuthN config")
		var perRoute envoy_ext_authz_v3.ExtAuthzPerRoute
		require.NoError(t, configs[postName].UnmarshalTo(&perRoute))
		assert.Equal(t, ref.rollout, perRoute.GetCheckSettings().GetContextExtensions()["rollout"],
			"the %s ref must keep its own auth context", ref.rollout)
	}
}

func TestMaterializeExtAuthSkipsReplacedRoutes(t *testing.T) {
	const filterChain = "test-filter-chain"

	metadata := &TrafficPolicyGatewayExtensionIR{
		Name: "ns/metadata",
		ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
			Services:                  &envoy_ext_authz_v3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{}},
			MetadataContextNamespaces: []string{JWTAuthnMetadataNamespace},
		},
	}
	routeIR := &extAuthIR{perProviderConfig: []*perProviderExtAuthConfig{{provider: metadata}}}

	plugin := &trafficPolicyPluginGwPass{}
	replacedRoute := &envoyroutev3.Route{}
	liveRoute := &envoyroutev3.Route{}
	vhost := &envoyroutev3.VirtualHost{Routes: []*envoyroutev3.Route{replacedRoute, liveRoute}}
	out := &envoyroutev3.RouteConfiguration{VirtualHosts: []*envoyroutev3.VirtualHost{vhost}}

	typed := ir.TypedFilterConfigMap{}
	plugin.handleExtAuth(filterChain, &typed, routeIR)
	state := plugin.recordExtAuthScope(replacedRoute, filterChain, routeIR)
	plugin.writeExtAuthProtoConfigs(&typed, state)
	replacedRoute.TypedPerFilterConfig = typed.ToAnyMap()
	plugin.recordExtAuthScope(liveRoute, filterChain, routeIR)

	// The translator replaces the invalid route and clears its filter configs.
	plugin.RouteReplaced(replacedRoute)
	replacedRoute.TypedPerFilterConfig = nil

	plugin.materializeExtAuth(out)

	assert.Nil(t, replacedRoute.GetTypedPerFilterConfig(), "finalization must not restore ext-auth on a synthetic 500")
	require.Contains(t, liveRoute.GetTypedPerFilterConfig(), extAuthVariantFilterName("ns/metadata", true))
}

func TestReportEffectiveExtAuthConflict(t *testing.T) {
	ref := &ir.AttachedPolicyRef{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "default", Name: "conflict"}
	clearState := &extAuthScopeState{
		providers: map[string]*perProviderExtAuthConfig{
			"legacy": {provider: &TrafficPolicyGatewayExtensionIR{ExtAuth: &envoy_ext_authz_v3.ExtAuthz{ClearRouteCache: true}}},
		},
		policyRefs: []extAuthPolicyIdentity{newExtAuthPolicyIdentity(ref, 7)},
	}
	metadataState := &extAuthScopeState{
		providers: map[string]*perProviderExtAuthConfig{
			"metadata": {provider: &TrafficPolicyGatewayExtensionIR{ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
				Services:                  &envoy_ext_authz_v3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{}},
				MetadataContextNamespaces: []string{JWTAuthnMetadataNamespace},
			}}},
		},
	}

	rm := reports.NewReportMap()
	plugin := &trafficPolicyPluginGwPass{
		reporter:              reports.NewReporter(&rm),
		reportedExtAuthErrors: map[string]struct{}{},
	}
	plugin.reportEffectiveExtAuthConflict([]*extAuthScopeState{metadataState, clearState})

	policyReport := rm.PolicyReport(reporter.PolicyKey{
		Group: ref.Group, Kind: ref.Kind, Namespace: ref.Namespace, Name: ref.Name,
	})
	require.NotNil(t, policyReport)
	ancestor, ok := policyReport.AncestorRef(reporter.PolicyStatusSummaryAncestorRef()).(*reports.AncestorRefReport)
	require.True(t, ok)
	condition := meta.FindStatusCondition(ancestor.Conditions, string(shared.PolicyConditionAccepted))
	require.NotNil(t, condition)
	assert.Equal(t, metav1.ConditionFalse, condition.Status)
	assert.Equal(t, string(shared.PolicyReasonInvalid), condition.Reason)
	assert.Equal(t, extAuthClearRouteCacheMetadataConflict, condition.Message)
}

// TestReportEffectiveExtAuthConflictSkipsUnrelatedProviders proves that when one
// provider in a merged scope conflicts, only the policies that contributed the
// metadata consumer or the route-cache clearer are marked invalid. A policy that
// contributed an unrelated provider in the same merge stays Accepted.
func TestReportEffectiveExtAuthConflictSkipsUnrelatedProviders(t *testing.T) {
	refFor := func(name string) *ir.AttachedPolicyRef {
		return &ir.AttachedPolicyRef{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "default", Name: name}
	}
	grpcExtAuth := func() *envoy_ext_authz_v3.ExtAuthz {
		return &envoy_ext_authz_v3.ExtAuthz{
			Services: &envoy_ext_authz_v3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{}},
		}
	}
	unrelated := grpcExtAuth()
	metadata := grpcExtAuth()
	metadata.MetadataContextNamespaces = []string{JWTAuthnMetadataNamespace}
	clearer := grpcExtAuth()
	clearer.ClearRouteCache = true

	refs := []*ir.AttachedPolicyRef{refFor("unrelated"), refFor("metadata"), refFor("clearer")}
	providerFor := func(name string, extAuth *envoy_ext_authz_v3.ExtAuthz, ref *ir.AttachedPolicyRef) *perProviderExtAuthConfig {
		return &perProviderExtAuthConfig{
			provider:     &TrafficPolicyGatewayExtensionIR{Name: name, ExtAuth: extAuth},
			mergeOrigins: sets.New(ref.ID()),
		}
	}
	state := &extAuthScopeState{
		providers: map[string]*perProviderExtAuthConfig{
			"unrelated": providerFor("ns/unrelated", unrelated, refs[0]),
			"metadata":  providerFor("ns/metadata", metadata, refs[1]),
			"clearer":   providerFor("ns/clearer", clearer, refs[2]),
		},
		policyRefs: []extAuthPolicyIdentity{
			newExtAuthPolicyIdentity(refs[0], 1),
			newExtAuthPolicyIdentity(refs[1], 2),
			newExtAuthPolicyIdentity(refs[2], 3),
		},
	}

	rm := reports.NewReportMap()
	plugin := &trafficPolicyPluginGwPass{
		reporter:              reports.NewReporter(&rm),
		reportedExtAuthErrors: map[string]struct{}{},
	}
	plugin.reportEffectiveExtAuthConflict([]*extAuthScopeState{state})

	hasConflictError := func(ref *ir.AttachedPolicyRef) bool {
		key := reporter.PolicyKey{Group: ref.Group, Kind: ref.Kind, Namespace: ref.Namespace, Name: ref.Name}
		pr := rm.PolicyReport(key)
		if pr == nil {
			return false
		}
		ancestor, ok := pr.AncestorRef(reporter.PolicyStatusSummaryAncestorRef()).(*reports.AncestorRefReport)
		if !ok {
			return false
		}
		condition := meta.FindStatusCondition(ancestor.Conditions, string(shared.PolicyConditionAccepted))
		return condition != nil && condition.Status == metav1.ConditionFalse
	}

	assert.True(t, hasConflictError(refs[1]), "the metadata policy must be reported invalid")
	assert.True(t, hasConflictError(refs[2]), "the clearRouteCache policy must be reported invalid")
	assert.False(t, hasConflictError(refs[0]), "a policy contributing only an unrelated provider must stay valid")
}

// TestReportEffectiveExtAuthConflictRequiresBothSettings proves a route with only
// a metadata consumer or only a route cache clearer is not reported as invalid,
// and that nil attachment scopes cannot crash finalization.
func TestReportEffectiveExtAuthConflictRequiresBothSettings(t *testing.T) {
	ref := &ir.AttachedPolicyRef{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "default", Name: "single-setting"}
	identity := newExtAuthPolicyIdentity(ref, 3)
	policyKey := reporter.PolicyKey{Group: ref.Group, Kind: ref.Kind, Namespace: ref.Namespace, Name: ref.Name}

	metadata := &extAuthScopeState{
		providers: map[string]*perProviderExtAuthConfig{
			"metadata": {provider: &TrafficPolicyGatewayExtensionIR{ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
				Services:                  &envoy_ext_authz_v3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{}},
				MetadataContextNamespaces: []string{JWTAuthnMetadataNamespace},
			}}},
		},
		policyRefs: []extAuthPolicyIdentity{identity},
	}
	clearer := &extAuthScopeState{
		providers: map[string]*perProviderExtAuthConfig{
			"legacy": {provider: &TrafficPolicyGatewayExtensionIR{ExtAuth: &envoy_ext_authz_v3.ExtAuthz{ClearRouteCache: true}}},
		},
		policyRefs: []extAuthPolicyIdentity{identity},
	}

	for _, tc := range []struct {
		name   string
		states []*extAuthScopeState
	}{
		{name: "metadata only", states: []*extAuthScopeState{nil, metadata}},
		{name: "clearRouteCache only", states: []*extAuthScopeState{nil, clearer}},
		{name: "no scopes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rm := reports.NewReportMap()
			plugin := &trafficPolicyPluginGwPass{
				reporter:              reports.NewReporter(&rm),
				reportedExtAuthErrors: map[string]struct{}{},
			}
			plugin.reportEffectiveExtAuthConflict(tc.states)

			policyReport := rm.PolicyReport(policyKey)
			if policyReport == nil {
				return
			}
			ancestor, ok := policyReport.AncestorRef(reporter.PolicyStatusSummaryAncestorRef()).(*reports.AncestorRefReport)
			if !ok {
				return
			}
			assert.Nil(t, meta.FindStatusCondition(ancestor.Conditions, string(shared.PolicyConditionAccepted)),
				"a single setting must not report an ext-auth conflict")
		})
	}
}

func filterConfigIsDisabled(t *testing.T, config *anypb.Any) bool {
	t.Helper()
	if config == nil {
		return false
	}
	var decoded envoyroutev3.FilterConfig
	require.NoError(t, config.UnmarshalTo(&decoded))
	return decoded.GetDisabled()
}

func TestExtAuthIRValidateRejectsMetadataWithRouteCacheClearer(t *testing.T) {
	grpcWithMetadata := func(clearRouteCache bool) *TrafficPolicyGatewayExtensionIR {
		return &TrafficPolicyGatewayExtensionIR{ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
			Services: &envoy_ext_authz_v3.ExtAuthz_GrpcService{
				GrpcService: &envoycorev3.GrpcService{
					TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
						EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{ClusterName: "ext-authz"},
					},
				},
			},
			MetadataContextNamespaces: []string{JWTAuthnMetadataNamespace},
			ClearRouteCache:           clearRouteCache,
		}}
	}
	httpWithRouteCacheClear := &TrafficPolicyGatewayExtensionIR{ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
		Services:        &envoy_ext_authz_v3.ExtAuthz_HttpService{HttpService: &envoy_ext_authz_v3.HttpService{}},
		ClearRouteCache: true,
	}}

	tests := []struct {
		name        string
		providers   []*TrafficPolicyGatewayExtensionIR
		expectError bool
	}{
		{
			name:      "metadata consumer alone is valid",
			providers: []*TrafficPolicyGatewayExtensionIR{grpcWithMetadata(false)},
		},
		{
			name:      "route cache clearer alone is valid",
			providers: []*TrafficPolicyGatewayExtensionIR{httpWithRouteCacheClear},
		},
		{
			name:        "metadata and route cache clearing in the same provider is invalid",
			providers:   []*TrafficPolicyGatewayExtensionIR{grpcWithMetadata(true)},
			expectError: true,
		},
		{
			name:        "metadata and route cache clearing across merged providers is invalid",
			providers:   []*TrafficPolicyGatewayExtensionIR{grpcWithMetadata(false), httpWithRouteCacheClear},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			irConfig := &extAuthIR{}
			for _, provider := range tc.providers {
				irConfig.perProviderConfig = append(irConfig.perProviderConfig, &perProviderExtAuthConfig{provider: provider})
			}

			err := irConfig.Validate()
			if tc.expectError {
				assert.ErrorContains(t, err, "clearRouteCache cannot be combined with metadataContextNamespaces")
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestExtAuthPolicyPlugin(t *testing.T) {
	t.Run("applies ext auth configuration to route", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{enableAuthMetadata: true}
		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
				extAuth: &extAuthIR{
					perProviderConfig: []*perProviderExtAuthConfig{
						{
							provider: &TrafficPolicyGatewayExtensionIR{
								Name: "test-auth-extension",
								ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
									FailureModeAllow: true,
									WithRequestBody: &envoy_ext_authz_v3.BufferSettings{
										MaxRequestBytes: 1024,
									},
								},
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
		err := plugin.ApplyForRoute(pCtx, outputRoute)

		// Verify
		require.NoError(t, err)
		require.NotNil(t, pCtx.TypedFilterConfig)
		extAuthConfig, ok := pCtx.TypedFilterConfig[extAuthFilterName("test-auth-extension")]
		assert.True(t, ok)
		assert.NotNil(t, extAuthConfig)
		assert.Empty(t, pCtx.TypedFilterConfig[ExtAuthGlobalDisableFilterName])
		assert.NotEmpty(t, pCtx.TypedFilterConfig[ExtAuthEnabledFilterName])
		assert.Contains(t, fmt.Sprintf("%s", pCtx.TypedFilterConfig[ExtAuthEnabledFilterName]),
			`\"key\":\"auth_succeeded\",\"value\":{\"stringValue\":\"true\"}}`, "ext_auth_enabled must set dynamic metadata")
	})

	t.Run("handles disabled ext auth configuration", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{enableAuthMetadata: true}
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
		err := plugin.ApplyForRoute(pCtx, outputRoute)

		// Verify
		require.NoError(t, err)
		// assert.NotNil(t, )
		assert.NotNil(t, pCtx.TypedFilterConfig, pCtx)
		assert.NotEmpty(t, pCtx.TypedFilterConfig[ExtAuthGlobalDisableFilterName])
		assert.NotEmpty(t, pCtx.TypedFilterConfig[ExtAuthEnabledFilterName])
		assert.NotContains(t, fmt.Sprintf("%s", pCtx.TypedFilterConfig[ExtAuthEnabledFilterName]), AuthSucceededMetadataKey, "ext_auth_enabled must not set dynamic metadata if the policy is disabled at the route level")
	})
}
