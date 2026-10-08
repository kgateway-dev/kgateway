package irtranslator

import (
	"context"
	"errors"
	"strconv"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/conditions"
	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	reportssdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

func routeConfigurationFromBootstrap(t *testing.T, config *envoybootstrapv3.Bootstrap) *envoyroutev3.RouteConfiguration {
	t.Helper()
	listeners := config.GetStaticResources().GetListeners()
	require.Len(t, listeners, 1)
	filterChains := listeners[0].GetFilterChains()
	require.Len(t, filterChains, 1)
	filters := filterChains[0].GetFilters()
	require.NotEmpty(t, filters)
	hcm := &envoyhcm.HttpConnectionManager{}
	require.NoError(t, filters[0].GetTypedConfig().UnmarshalTo(hcm))
	return hcm.GetRouteConfig()
}

func scopeTestVirtualHosts() []*ir.VirtualHost {
	return []*ir.VirtualHost{
		{Name: "one", Hostname: "one.example.com", Rules: []ir.HttpRouteRuleMatchIR{testRouteIR(0, "/one", "shared-cluster")}},
		{Name: "two", Hostname: "two.example.com", Rules: []ir.HttpRouteRuleMatchIR{testRouteIR(0, "/two", "shared-cluster")}},
	}
}

func TestRouteConfigPolicyErrorValidatesFallback(t *testing.T) {
	for _, invalidSharedSettings := range []bool{false, true} {
		t.Run("invalid shared settings="+strconv.FormatBool(invalidSharedSettings), func(t *testing.T) {
			calls := 0
			v := &routeMockValidator{validateFunc: func(_ context.Context, b *envoybootstrapv3.Bootstrap) error {
				calls++
				cfg := routeConfigurationFromBootstrap(t, b)
				require.Len(t, cfg.VirtualHosts, 1, "discarded virtual hosts must not be validated")
				require.Len(t, cfg.VirtualHosts[0].Routes, 1)
				assert.EqualValues(t, 500, cfg.VirtualHosts[0].Routes[0].GetDirectResponse().GetStatus(), "validate only the fallback")
				if invalidSharedSettings && len(cfg.ResponseHeadersToAdd) > 0 {
					return errors.New("invalid shared response header")
				}
				return nil
			}}
			h := testHTTPRouteTranslator(v, apisettings.ValidationStrict)
			h.routeConfigName = "test-config"
			attachRouteConfigPass(h, routeConfigPassFunc{apply: func(cfg *envoyroutev3.RouteConfiguration) {
				cfg.ResponseHeadersToAdd = []*envoycorev3.HeaderValueOption{{Header: &envoycorev3.HeaderValue{Key: "x-test", Value: "shared"}}}
			}})
			for gk, policies := range h.attachedPolicies.Policies {
				h.attachedPolicies.Policies[gk] = append(policies, ir.PolicyAtt{GroupKind: gk, Errors: []error{errors.New("invalid policy")}})
			}
			cfg := h.ComputeRouteConfiguration(t.Context(), scopeTestVirtualHosts())
			assert.Equal(t, "test-config", cfg.Name)
			require.Len(t, cfg.VirtualHosts, 1)
			require.Len(t, cfg.VirtualHosts[0].Routes, 1)
			assert.EqualValues(t, 500, cfg.VirtualHosts[0].Routes[0].GetDirectResponse().GetStatus())
			if invalidSharedSettings {
				assert.Empty(t, cfg.ResponseHeadersToAdd, "invalid shared settings must not survive fallback")
				assert.Equal(t, 2, calls)
			} else {
				assert.Len(t, cfg.ResponseHeadersToAdd, 1, "valid shared settings should survive fallback")
				assert.Equal(t, 1, calls, "validate the fallback once without isolating discarded routes")
			}
		})
	}
}

func TestFinalRouteConfigurationPreservesScopeSettings(t *testing.T) {
	calls := 0
	var validated []*envoyroutev3.RouteConfiguration
	v := &routeMockValidator{validateFunc: func(_ context.Context, b *envoybootstrapv3.Bootstrap) error {
		calls++
		validated = append(validated, routeConfigurationFromBootstrap(t, b))
		return nil
	}}
	h := testHTTPRouteTranslator(v, apisettings.ValidationStrict)
	config, err := utils.MessageToAny(wrapperspb.Bool(true))
	require.NoError(t, err)
	attachRouteConfigPass(h, routeConfigPassFunc{applyContext: func(ctx *ir.RouteConfigContext) {
		ctx.TypedFilterConfig.AddTypedConfig("inherited", wrapperspb.Bool(true))
	}, apply: func(cfg *envoyroutev3.RouteConfiguration) {
		cfg.ResponseHeadersToAdd = []*envoycorev3.HeaderValueOption{{Header: &envoycorev3.HeaderValue{Key: "x-test", Value: "scope"}}}
		for _, vh := range cfg.VirtualHosts {
			vh.RateLimits = []*envoyroutev3.RateLimit{{
				Stage: wrapperspb.UInt32(1),
				Actions: []*envoyroutev3.RateLimit_Action{{ActionSpecifier: &envoyroutev3.RateLimit_Action_GenericKey_{
					GenericKey: &envoyroutev3.RateLimit_Action_GenericKey{DescriptorValue: "limited"},
				}}},
			}}
			vh.TypedPerFilterConfig = map[string]*anypb.Any{"vhost": config}
		}
	}})
	cfg := h.ComputeRouteConfiguration(context.Background(), scopeTestVirtualHosts())
	require.Equal(t, 3, calls, "validate shared settings and each virtual host separately")
	for i, vh := range cfg.VirtualHosts {
		expected := (routeValidationScope{config: cfg, vhost: vh}).configuration(vh.Routes)
		assert.True(t, proto.Equal(expected, validated[i+1]), "validation must retain final routes and inherited settings")
	}
}

func TestFinalRouteConfigurationIsolatesScopeFailures(t *testing.T) {
	config, err := utils.MessageToAny(wrapperspb.Bool(true))
	require.NoError(t, err)
	for _, tc := range []struct {
		name          string
		mutate        func(*envoyroutev3.RouteConfiguration)
		mutateContext func(*ir.RouteConfigContext)
		reject        func(*envoyroutev3.RouteConfiguration) bool
		wholeConfig   bool
	}{
		{
			name: "virtual host rate limit",
			mutate: func(cfg *envoyroutev3.RouteConfiguration) {
				cfg.VirtualHosts[0].RateLimits = []*envoyroutev3.RateLimit{{Stage: wrapperspb.UInt32(11)}}
			},
			reject: func(cfg *envoyroutev3.RouteConfiguration) bool {
				for _, vh := range cfg.VirtualHosts {
					if len(vh.RateLimits) > 0 {
						return true
					}
				}
				return false
			},
		},
		{
			name: "virtual host inherited filter",
			mutate: func(cfg *envoyroutev3.RouteConfiguration) {
				cfg.VirtualHosts[0].TypedPerFilterConfig = map[string]*anypb.Any{"bad-filter": config}
			},
			reject: func(cfg *envoyroutev3.RouteConfiguration) bool {
				for _, vh := range cfg.VirtualHosts {
					if vh.TypedPerFilterConfig["bad-filter"] != nil {
						return true
					}
				}
				return false
			},
		},
		{
			name: "empty virtual host rate limit",
			mutate: func(cfg *envoyroutev3.RouteConfiguration) {
				cfg.VirtualHosts[0].Routes = nil
				cfg.VirtualHosts[0].RateLimits = []*envoyroutev3.RateLimit{{Stage: wrapperspb.UInt32(11)}}
			},
			reject: func(cfg *envoyroutev3.RouteConfiguration) bool {
				for _, vh := range cfg.VirtualHosts {
					if len(vh.RateLimits) > 0 {
						return true
					}
				}
				return false
			},
		},
		{
			name:   "route configuration inherited filter",
			mutate: func(*envoyroutev3.RouteConfiguration) {},
			mutateContext: func(ctx *ir.RouteConfigContext) {
				ctx.TypedFilterConfig.AddTypedConfig("bad-filter", wrapperspb.Bool(true))
			},
			reject:      func(cfg *envoyroutev3.RouteConfiguration) bool { return cfg.TypedPerFilterConfig["bad-filter"] != nil },
			wholeConfig: true,
		},
		{
			name: "route configuration headers",
			mutate: func(cfg *envoyroutev3.RouteConfiguration) {
				cfg.ResponseHeadersToAdd = []*envoycorev3.HeaderValueOption{{Header: &envoycorev3.HeaderValue{Key: "bad-header"}}}
			},
			reject:      func(cfg *envoyroutev3.RouteConfiguration) bool { return len(cfg.ResponseHeadersToAdd) > 0 },
			wholeConfig: true,
		},
		{
			name:   "duplicate domains across virtual hosts",
			mutate: func(cfg *envoyroutev3.RouteConfiguration) { cfg.VirtualHosts[1].Domains = cfg.VirtualHosts[0].Domains },
			reject: func(cfg *envoyroutev3.RouteConfiguration) bool {
				return len(cfg.VirtualHosts) == 2 && cfg.VirtualHosts[0].Domains[0] == cfg.VirtualHosts[1].Domains[0]
			},
			wholeConfig: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := &routeMockValidator{validateFunc: func(_ context.Context, b *envoybootstrapv3.Bootstrap) error {
				if tc.reject(routeConfigurationFromBootstrap(t, b)) {
					return errors.New("invalid scope configuration")
				}
				return nil
			}}
			h := testHTTPRouteTranslator(v, apisettings.ValidationStrict)
			h.routeConfigName = "original-route-config"
			attachRouteConfigPass(h, routeConfigPassFunc{applyContext: tc.mutateContext, apply: func(cfg *envoyroutev3.RouteConfiguration) {
				cfg.VirtualHosts[0].Domains = append(cfg.VirtualHosts[0].Domains, "alias.example.com")
				tc.mutate(cfg)
			}})
			inputs := scopeTestVirtualHosts()
			rm := reports.NewReportMap()
			h.reporter = reports.NewReporter(&rm)
			for _, vh := range inputs {
				vh.ParentRef = ir.Listener{Parent: &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: vh.Name}}, Listener: gwv1.Listener{Name: "http"}}
			}
			cfg := h.ComputeRouteConfiguration(context.Background(), inputs)
			for i, vh := range inputs {
				gw := vh.ParentRef.Parent.(*gwv1.Gateway)
				report := rm.Gateway(gw)
				if report == nil {
					require.False(t, i == 0 || tc.wholeConfig, "affected listener must have a report")
					continue
				}
				listener := report.Listener(&vh.ParentRef.Listener).(*reports.ListenerReport)
				condition := meta.FindStatusCondition(listener.Status.Conditions, string(gwv1.ListenerConditionAccepted))
				if i == 0 || tc.wholeConfig {
					require.NotNil(t, condition, "affected listener must report replacement")
					assert.Equal(t, string(reportssdk.ListenerReplacedReason), condition.Reason)
					assert.Equal(t, metav1.ConditionFalse, condition.Status)
				} else if condition != nil {
					assert.NotEqual(t, string(reportssdk.ListenerReplacedReason), condition.Reason, "healthy listener must not report replacement")
				}
			}
			assert.False(t, tc.reject(cfg), "invalid scope settings must not survive fallback")
			assert.Equal(t, h.routeConfigName, cfg.Name, "RDS identity must survive fallback")
			if tc.wholeConfig {
				require.Len(t, cfg.VirtualHosts, 1)
				assert.Equal(t, []string{"*"}, cfg.VirtualHosts[0].Domains)
			} else {
				require.Len(t, cfg.VirtualHosts, 2)
				assert.Equal(t, []string{"one.example.com", "alias.example.com"}, cfg.VirtualHosts[0].Domains)
				assert.Equal(t, "shared-cluster", cfg.VirtualHosts[1].Routes[0].GetRoute().GetCluster(), "healthy virtual host must remain unchanged")
			}
			assert.EqualValues(t, 500, cfg.VirtualHosts[0].Routes[0].GetDirectResponse().GetStatus())
		})
	}
}

// A valid route may need a named extension from its parent RouteConfiguration.
// An unrelated failure must not strip that dependency during vhost or route isolation.
func TestRouteIsolationPreservesSharedDependencies(t *testing.T) {
	const pluginName = "shared-selector"
	isolatedDependentRoute := false
	v := &routeMockValidator{validateFunc: func(_ context.Context, b *envoybootstrapv3.Bootstrap) error {
		cfg := routeConfigurationFromBootstrap(t, b)
		declared := false
		for _, plugin := range cfg.ClusterSpecifierPlugins {
			if plugin.GetExtension().GetName() == pluginName {
				declared = true
			}
		}
		var bad bool
		for _, vh := range cfg.VirtualHosts {
			for _, route := range vh.Routes {
				if route.GetRoute().GetClusterSpecifierPlugin() == pluginName {
					require.True(t, declared, "isolating a route must retain its named cluster specifier plugin")
					require.NotNil(t, cfg.TypedPerFilterConfig["shared"], "RC inheritance must survive isolation")
					require.NotNil(t, vh.TypedPerFilterConfig["inherited"], "vhost inheritance must survive route isolation")
					if len(cfg.VirtualHosts) == 1 && len(vh.Routes) == 1 {
						isolatedDependentRoute = true
					}
				}
				bad = bad || route.GetRoute().GetCluster() == "invalid"
			}
		}
		if bad {
			return errors.New("invalid route action")
		}
		return nil
	}}
	h := testHTTPRouteTranslator(v, apisettings.ValidationStrict)
	inherited, err := utils.MessageToAny(wrapperspb.Bool(true))
	require.NoError(t, err)
	attachRouteConfigPass(h, routeConfigPassFunc{
		applyContext: func(ctx *ir.RouteConfigContext) {
			ctx.TypedFilterConfig.AddTypedConfig("shared", wrapperspb.Bool(true))
		},
		apply: func(cfg *envoyroutev3.RouteConfiguration) {
			cfg.ClusterSpecifierPlugins = []*envoyroutev3.ClusterSpecifierPlugin{{Extension: &envoycorev3.TypedExtensionConfig{Name: pluginName}}}
			for _, vh := range cfg.VirtualHosts {
				vh.TypedPerFilterConfig = map[string]*anypb.Any{"inherited": inherited}
				vh.Routes[0].GetRoute().ClusterSpecifier = &envoyroutev3.RouteAction_ClusterSpecifierPlugin{ClusterSpecifierPlugin: pluginName}
			}
		},
	})
	inputs := scopeTestVirtualHosts()
	inputs[0].Rules = append(inputs[0].Rules, testRouteIR(1, "/bad", "invalid"))
	cfg := h.ComputeRouteConfiguration(context.Background(), inputs)
	require.True(t, isolatedDependentRoute, "exercise individual route validation as well as vhost validation")
	require.Len(t, cfg.VirtualHosts, 2)
	require.Len(t, cfg.VirtualHosts[0].Routes, 2)
	for _, vh := range cfg.VirtualHosts {
		assert.Equal(t, pluginName, vh.Routes[0].GetRoute().GetClusterSpecifierPlugin(), "healthy dependent route must retain its action")
	}
	assert.EqualValues(t, 500, cfg.VirtualHosts[0].Routes[1].GetDirectResponse().GetStatus())
}

func TestFinalRouteValidationReportsSourceRoute(t *testing.T) {
	const marker = "invalid-for-status"
	for _, tc := range []struct {
		name       string
		mutate     func(*envoyroutev3.RouteConfiguration) *envoyroutev3.Route
		associated bool
	}{
		{name: "in-place mutation", associated: true, mutate: func(cfg *envoyroutev3.RouteConfiguration) *envoyroutev3.Route { return cfg.VirtualHosts[0].Routes[1] }},
		{name: "renamed original pointer", associated: true, mutate: func(cfg *envoyroutev3.RouteConfiguration) *envoyroutev3.Route {
			routes := cfg.VirtualHosts[0].Routes
			routes[1].Name = routes[0].Name
			return routes[1]
		}},
		{name: "cloned route", associated: true, mutate: func(cfg *envoyroutev3.RouteConfiguration) *envoyroutev3.Route {
			vh := cfg.VirtualHosts[0]
			vh.Routes[1] = proto.CloneOf(vh.Routes[1])
			return vh.Routes[1]
		}},
		{name: "cloned virtual host", associated: true, mutate: func(cfg *envoyroutev3.RouteConfiguration) *envoyroutev3.Route {
			cfg.VirtualHosts[0] = proto.CloneOf(cfg.VirtualHosts[0])
			return cfg.VirtualHosts[0].Routes[1]
		}},
		{name: "appended unrelated route", mutate: func(cfg *envoyroutev3.RouteConfiguration) *envoyroutev3.Route {
			vh := cfg.VirtualHosts[0]
			route := proto.CloneOf(vh.Routes[1])
			route.Name = "new-route"
			vh.Routes = append(vh.Routes, route)
			return route
		}},
		{name: "appended duplicate name", mutate: func(cfg *envoyroutev3.RouteConfiguration) *envoyroutev3.Route {
			vh := cfg.VirtualHosts[0]
			route := proto.CloneOf(vh.Routes[1])
			vh.Routes = append(vh.Routes, route)
			return route
		}},
		{name: "ambiguous original names", mutate: func(cfg *envoyroutev3.RouteConfiguration) *envoyroutev3.Route {
			vh := cfg.VirtualHosts[0]
			vh.Routes[0].Name = "duplicate"
			vh.Routes[1].Name = "duplicate"
			route := proto.CloneOf(vh.Routes[1])
			vh.Routes = []*envoyroutev3.Route{route}
			return route
		}},
		{name: "appended unrelated virtual host", mutate: func(cfg *envoyroutev3.RouteConfiguration) *envoyroutev3.Route {
			vh := proto.CloneOf(cfg.VirtualHosts[0])
			vh.Name = "new-vhost"
			vh.Domains = []string{"new.example.com"}
			cfg.VirtualHosts = append(cfg.VirtualHosts, vh)
			return vh.Routes[1]
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rm := reports.NewReportMap()
			h := testHTTPRouteTranslator(invalidClusterValidator(t, marker), apisettings.ValidationStrict)
			h.reporter = reports.NewReporter(&rm)
			inputs := []*ir.VirtualHost{{Name: "vhost", Hostname: "example.com"}}
			parent := gwv1.ParentReference{Name: "gateway"}
			for i, name := range []string{"healthy", "target"} {
				rule := testRouteIR(i*7, "/"+name, "cluster-"+name)
				rule.Parent = &ir.HttpRouteIR{
					ObjectSource: ir.ObjectSource{Name: name, Namespace: "default", Kind: "HTTPRoute"},
					SourceObject: &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}},
				}
				rule.ParentRef = parent
				inputs[0].Rules = append(inputs[0].Rules, rule)
			}
			attachRouteConfigPass(h, routeConfigPassFunc{apply: func(cfg *envoyroutev3.RouteConfiguration) {
				tc.mutate(cfg).GetRoute().ClusterSpecifier = &envoyroutev3.RouteAction_Cluster{Cluster: marker}
			}})
			cfg := h.ComputeRouteConfiguration(t.Context(), inputs)
			replacements := 0
			for _, vh := range cfg.VirtualHosts {
				for _, route := range vh.Routes {
					if route.GetDirectResponse().GetStatus() == 500 {
						replacements++
					}
				}
			}
			require.Equal(t, 1, replacements, "invalid output must be isolated even without a source association")
			require.Len(t, rm.HTTPRoutes, 2, "plugin-added output must not invent a Kubernetes source")
			for _, name := range []string{"healthy", "target"} {
				report := rm.HTTPRoutes[types.NamespacedName{Namespace: "default", Name: name}]
				require.NotNil(t, report)
				require.Len(t, report.Parents, 1)
				for _, refReport := range report.Parents {
					condition := meta.FindStatusCondition(refReport.Conditions, conditions.KgatewayConditionProgrammed)
					if tc.associated && name == "target" {
						require.NotNil(t, condition, "source route must report failed programming")
						assert.Equal(t, metav1.ConditionFalse, condition.Status)
						assert.Equal(t, string(reportssdk.RouteRuleReplacedReason), condition.Reason)
						assert.Contains(t, condition.Message, "Replaced Rule (7)", "report the originating rule index")
					} else {
						assert.Nil(t, condition, "unaffected sources must not be blamed for invalid plugin output")
					}
				}
			}
		})
	}
}

func TestCheapRouteFailuresRemainBatched(t *testing.T) {
	for _, count := range []int{10, 1000} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			calls := 0
			v := &routeMockValidator{validateFunc: func(_ context.Context, b *envoybootstrapv3.Bootstrap) error {
				calls++
				routes := routesFromValidationBootstrap(t, b)
				require.LessOrEqual(t, calls, 3)
				require.Len(t, routes, []int{2, count, count - 1}[calls-1], "validate shared settings, then each repaired virtual host")
				for _, route := range routes {
					require.NoError(t, validateRoutePreEnvoy(route, apisettings.ValidationStrict), "every cheap failure must be repaired before batch validation")
				}
				return nil
			}}
			h := testHTTPRouteTranslator(v, apisettings.ValidationStrict)
			inputs := scopeTestVirtualHosts()
			for _, vh := range inputs {
				vh.Rules = nil
				for i := range count {
					vh.Rules = append(vh.Rules, testRouteIR(i, "/route-"+strconv.Itoa(i), "shared-cluster"))
				}
			}
			attachRouteConfigPass(h, routeConfigPassFunc{apply: func(cfg *envoyroutev3.RouteConfiguration) {
				cfg.VirtualHosts[0].Routes[0].GetRoute().PrefixRewrite = "/bad//rewrite"
				cfg.VirtualHosts[1].Routes[0].Match = &envoyroutev3.RouteMatch{PathSpecifier: &envoyroutev3.RouteMatch_SafeRegex{SafeRegex: &matcherv3.RegexMatcher{Regex: "["}}}
			}})
			cfg := h.ComputeRouteConfiguration(t.Context(), inputs)
			require.Equal(t, 3, calls, "cheap failures must not cause one Envoy invocation per healthy route")
			require.Len(t, cfg.VirtualHosts, 2)
			require.Len(t, cfg.VirtualHosts[0].Routes, count)
			require.Len(t, cfg.VirtualHosts[1].Routes, count-1)
			assert.EqualValues(t, 500, cfg.VirtualHosts[0].Routes[0].GetDirectResponse().GetStatus())
			for _, route := range cfg.VirtualHosts[0].Routes[1:] {
				require.NotNil(t, route.GetRoute(), "healthy siblings must keep their forwarding action")
			}
		})
	}
}

func TestCheapRouteDropPreservesSourceAttribution(t *testing.T) {
	const marker = "invalid-appended-route"
	rm := reports.NewReportMap()
	h := testHTTPRouteTranslator(invalidClusterValidator(t, marker), apisettings.ValidationStrict)
	h.reporter = reports.NewReporter(&rm)
	rule := testRouteIR(7, "/original", "cluster")
	rule.Parent = &ir.HttpRouteIR{
		ObjectSource: ir.ObjectSource{Name: "original", Namespace: "default", Kind: "HTTPRoute"},
		SourceObject: &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "original", Namespace: "default"}},
	}
	rule.ParentRef = gwv1.ParentReference{Name: "gateway"}
	attachRouteConfigPass(h, routeConfigPassFunc{apply: func(cfg *envoyroutev3.RouteConfiguration) {
		vh := cfg.VirtualHosts[0]
		original := vh.Routes[0]
		appended := proto.CloneOf(original)
		appended.GetRoute().ClusterSpecifier = &envoyroutev3.RouteAction_Cluster{Cluster: marker}
		original.Match = &envoyroutev3.RouteMatch{PathSpecifier: &envoyroutev3.RouteMatch_SafeRegex{SafeRegex: &matcherv3.RegexMatcher{Regex: "["}}}
		vh.Routes = append(vh.Routes, appended)
	}})
	cfg := h.ComputeRouteConfiguration(t.Context(), []*ir.VirtualHost{{Name: "vhost", Hostname: "example.com", Rules: []ir.HttpRouteRuleMatchIR{rule}}})
	require.Len(t, cfg.VirtualHosts[0].Routes, 1, "drop the original and retain the replaced appended route")
	require.EqualValues(t, 500, cfg.VirtualHosts[0].Routes[0].GetDirectResponse().GetStatus())
	report := rm.HTTPRoutes[types.NamespacedName{Name: "original", Namespace: "default"}]
	require.NotNil(t, report)
	require.Len(t, report.Parents, 1)
	for _, parent := range report.Parents {
		condition := meta.FindStatusCondition(parent.Conditions, conditions.KgatewayConditionProgrammed)
		require.NotNil(t, condition)
		assert.Equal(t, string(reportssdk.RouteRuleDroppedReason), condition.Reason, "the appended clone must not overwrite the original route's dropped status")
		assert.Contains(t, condition.Message, "Dropped Rule (7)")
	}
}

func TestRouteValidationCacheReusesUnchangedVirtualHosts(t *testing.T) {
	var validated []*envoyroutev3.RouteConfiguration
	inner := &routeMockValidator{validateFunc: func(_ context.Context, b *envoybootstrapv3.Bootstrap) error {
		validated = append(validated, routeConfigurationFromBootstrap(t, b))
		return nil
	}}
	h := testHTTPRouteTranslator(validator.NewCaching(inner, 100), apisettings.ValidationStrict)
	inputs := scopeTestVirtualHosts()
	h.ComputeRouteConfiguration(t.Context(), inputs)
	require.Len(t, validated, 3)
	validated = nil
	inputs[0].Rules[0].Backends[0].Backend.ClusterName = "changed-cluster"
	h.ComputeRouteConfiguration(t.Context(), inputs)
	require.Len(t, validated, 1, "route edits should miss only the affected virtual host's cache entry")
	require.Len(t, validated[0].VirtualHosts, 1)
	assert.Equal(t, "one", validated[0].VirtualHosts[0].Name)
	validated = nil
	attachRouteConfigPass(h, routeConfigPassFunc{apply: func(cfg *envoyroutev3.RouteConfiguration) {
		cfg.ResponseHeadersToAdd = []*envoycorev3.HeaderValueOption{{Header: &envoycorev3.HeaderValue{Key: "x-new", Value: "shared"}}}
	}})
	h.ComputeRouteConfiguration(t.Context(), inputs)
	require.Len(t, validated, 3, "shared settings changes must invalidate all dependent scopes")
}

func TestRouteValidationScopeDoesNotMutateSource(t *testing.T) {
	cfg := &envoyroutev3.RouteConfiguration{VirtualHosts: []*envoyroutev3.VirtualHost{{
		Name: "source", Domains: []string{"example.com"}, Routes: []*envoyroutev3.Route{newRouteWithPrefix("/original")},
	}}}
	before := proto.CloneOf(cfg)
	scope := routeValidationScope{config: cfg, vhost: cfg.VirtualHosts[0]}
	isolated := scope.configuration([]*envoyroutev3.Route{newRouteWithPrefix("/isolated")})
	isolated.VirtualHosts[0].Domains[0] = "changed.example.com"
	assert.True(t, proto.Equal(before, cfg), "isolating routes must not mutate or alias inherited source settings")
}

func BenchmarkRouteValidationScopeConfiguration(b *testing.B) {
	for _, count := range []int{1, 5000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			vh := &envoyroutev3.VirtualHost{Name: "vhost", Domains: []string{"example.com"}}
			for range count {
				vh.Routes = append(vh.Routes, newRouteWithPrefix("/"))
			}
			cfg := &envoyroutev3.RouteConfiguration{VirtualHosts: []*envoyroutev3.VirtualHost{vh}}
			scope := routeValidationScope{config: cfg, vhost: vh}
			selected := vh.Routes[:1]
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				scope.configuration(selected)
			}
		})
	}
}
