package trafficpolicy

import (
	"regexp"
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestStatPrefixIREquals(t *testing.T) {
	tests := []struct {
		name     string
		a        *statPrefixIR
		b        *statPrefixIR
		expected bool
	}{
		{name: "both nil are equal", a: nil, b: nil, expected: true},
		{name: "nil vs non-nil are not equal", a: nil, b: &statPrefixIR{template: "x"}, expected: false},
		{name: "non-nil vs nil are not equal", a: &statPrefixIR{template: "x"}, b: nil, expected: false},
		{name: "same template is equal", a: &statPrefixIR{template: "x"}, b: &statPrefixIR{template: "x"}, expected: true},
		{name: "different template is not equal", a: &statPrefixIR{template: "x"}, b: &statPrefixIR{template: "y"}, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.a.Equals(tt.b)
			assert.Equal(t, tt.expected, result)
			assert.Equal(t, result, tt.b.Equals(tt.a), "Equals should be symmetric")
		})
	}
}

// statPrefixCRDPattern mirrors the regex in the TrafficPolicy StatPrefix field's
// `+kubebuilder:validation:Pattern` marker. The Envoy stat prefix syntax is
// validated at admission by that pattern rather than in Go, so this test guards
// the pattern against regressions. Keep it in sync with the marker in
// api/v1alpha1/kgateway/traffic_policy_types.go.
const statPrefixCRDPattern = `^([a-zA-Z0-9_%.-]|\{\{\s*(route_name|route_namespace|rule_name)\s*\}\})+$`

func TestStatPrefixCRDPattern(t *testing.T) {
	re := regexp.MustCompile(statPrefixCRDPattern)

	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "literal is valid", value: "my-static-prefix", valid: true},
		{name: "safe literal punctuation is valid", value: "svc_1.route-2%", valid: true},
		{name: "supported vars are valid", value: "{{route_namespace}}.{{route_name}}.{{rule_name}}", valid: true},
		{name: "whitespace inside braces is valid", value: "{{ route_name }}", valid: true},
		{name: "empty is invalid", value: "", valid: false},
		{name: "unsupported var is invalid", value: "{{gateway_name}}", valid: false},
		{name: "typo var with hyphen is invalid", value: "{{route-name}}", valid: false},
		{name: "whitespace in variable name is invalid", value: "{{ route name }}", valid: false},
		{name: "unmatched brace is invalid", value: "{{route_name}", valid: false},
		{name: "stray braces are invalid", value: "foo{}bar", valid: false},
		{name: "literal whitespace is invalid", value: "foo bar", valid: false},
		{name: "path separator is invalid", value: "foo/bar", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, re.MatchString(tt.value))
		})
	}
}

func TestConstructStatPrefix(t *testing.T) {
	t.Run("unset leaves IR nil", func(t *testing.T) {
		out := &trafficPolicySpecIr{}
		constructStatPrefix(kgateway.TrafficPolicySpec{}, out)
		assert.Nil(t, out.statPrefix)
	})

	t.Run("set copies template verbatim", func(t *testing.T) {
		out := &trafficPolicySpecIr{}
		tmpl := "{{route_name}}"
		constructStatPrefix(kgateway.TrafficPolicySpec{StatPrefix: &tmpl}, out)
		require.NotNil(t, out.statPrefix)
		assert.Equal(t, "{{route_name}}", out.statPrefix.template)
	})
}

func TestApplyStatPrefix(t *testing.T) {
	plugin := &trafficPolicyPluginGwPass{}

	routeCtx := func(name, ns, rule string) *ir.RouteContext {
		return &ir.RouteContext{
			In: ir.HttpRouteRuleMatchIR{
				Parent:   &ir.HttpRouteIR{ObjectSource: ir.ObjectSource{Name: name, Namespace: ns}},
				RuleName: rule,
			},
		}
	}

	newRoute := func() *envoyroutev3.Route {
		return &envoyroutev3.Route{
			Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}},
		}
	}

	t.Run("resolves all template variables", func(t *testing.T) {
		policy := &TrafficPolicy{spec: trafficPolicySpecIr{
			statPrefix: &statPrefixIR{template: "{{ route_namespace }}.{{ route_name }}.{{ rule_name }}"},
		}}
		pCtx := routeCtx("my-route", "my-ns", "rule0")
		pCtx.Policy = policy
		out := newRoute()

		require.NoError(t, plugin.ApplyForRoute(pCtx, out))
		assert.Equal(t, "my-ns.my-route.rule0", out.GetStatPrefix())
	})

	t.Run("literal prefix is set verbatim", func(t *testing.T) {
		policy := &TrafficPolicy{spec: trafficPolicySpecIr{statPrefix: &statPrefixIR{template: "static"}}}
		pCtx := routeCtx("my-route", "my-ns", "rule0")
		pCtx.Policy = policy
		out := newRoute()

		require.NoError(t, plugin.ApplyForRoute(pCtx, out))
		assert.Equal(t, "static", out.GetStatPrefix())
	})

	t.Run("empty trailing rule name drops the trailing dot segment", func(t *testing.T) {
		policy := &TrafficPolicy{spec: trafficPolicySpecIr{
			statPrefix: &statPrefixIR{template: "{{route_namespace}}.{{route_name}}.{{rule_name}}"},
		}}
		pCtx := routeCtx("my-route", "my-ns", "")
		pCtx.Policy = policy
		out := newRoute()

		require.NoError(t, plugin.ApplyForRoute(pCtx, out))
		assert.Equal(t, "my-ns.my-route", out.GetStatPrefix())
	})

	t.Run("empty rule name in the middle drops the empty dot segment", func(t *testing.T) {
		policy := &TrafficPolicy{spec: trafficPolicySpecIr{
			statPrefix: &statPrefixIR{template: "{{route_name}}.{{rule_name}}.suffix"},
		}}
		pCtx := routeCtx("my-route", "my-ns", "")
		pCtx.Policy = policy
		out := newRoute()

		require.NoError(t, plugin.ApplyForRoute(pCtx, out))
		assert.Equal(t, "my-route.suffix", out.GetStatPrefix())
	})

	t.Run("non-dot separators are left intact around an empty rule name", func(t *testing.T) {
		// '-' is an ordinary character within a stat segment (not the '.'
		// segment delimiter), so it is preserved verbatim.
		policy := &TrafficPolicy{spec: trafficPolicySpecIr{
			statPrefix: &statPrefixIR{template: "{{route_name}}-{{rule_name}}"},
		}}
		pCtx := routeCtx("my-route", "my-ns", "")
		pCtx.Policy = policy
		out := newRoute()

		require.NoError(t, plugin.ApplyForRoute(pCtx, out))
		assert.Equal(t, "my-route-", out.GetStatPrefix())
	})

	t.Run("nil stat prefix leaves route untouched", func(t *testing.T) {
		policy := &TrafficPolicy{spec: trafficPolicySpecIr{statPrefix: nil}}
		pCtx := routeCtx("my-route", "my-ns", "rule0")
		pCtx.Policy = policy
		out := newRoute()

		require.NoError(t, plugin.ApplyForRoute(pCtx, out))
		assert.Empty(t, out.GetStatPrefix())
	})

	t.Run("applies to direct-response route without a RouteAction", func(t *testing.T) {
		policy := &TrafficPolicy{spec: trafficPolicySpecIr{statPrefix: &statPrefixIR{template: "{{route_name}}"}}}
		pCtx := routeCtx("my-route", "my-ns", "rule0")
		pCtx.Policy = policy
		out := &envoyroutev3.Route{
			Action: &envoyroutev3.Route_DirectResponse{
				DirectResponse: &envoyroutev3.DirectResponseAction{Status: 200},
			},
		}

		require.NoError(t, plugin.ApplyForRoute(pCtx, out))
		assert.Equal(t, "my-route", out.GetStatPrefix())
	})
}
