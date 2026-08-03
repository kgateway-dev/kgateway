package trafficpolicy

import (
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	kgateway "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func TestConstructRequestMirror(t *testing.T) {
	t.Run("unset leaves IR nil", func(t *testing.T) {
		out := &trafficPolicySpecIr{}

		constructRequestMirror(kgateway.TrafficPolicySpec{}, out)

		assert.Nil(t, out.requestMirror)
	})

	for _, tt := range []struct {
		name  string
		value bool
	}{
		{name: "true is copied", value: true},
		{name: "false is copied", value: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := tt.value
			out := &trafficPolicySpecIr{}

			constructRequestMirror(kgateway.TrafficPolicySpec{
				RequestMirror: &kgateway.RequestMirrorPolicy{
					DisableShadowHostSuffixAppend: &input,
				},
			}, out)

			require.NotNil(t, out.requestMirror)
			require.NotNil(t, out.requestMirror.disableShadowHostSuffixAppend)
			assert.Equal(t, tt.value, *out.requestMirror.disableShadowHostSuffixAppend)

			input = !tt.value
			assert.Equal(t, tt.value, *out.requestMirror.disableShadowHostSuffixAppend)
		})
	}

	t.Run("hostRewriteLiteral is copied", func(t *testing.T) {
		input := "shadow.example:8080"
		out := &trafficPolicySpecIr{}

		constructRequestMirror(kgateway.TrafficPolicySpec{
			RequestMirror: &kgateway.RequestMirrorPolicy{
				HostRewriteLiteral: &input,
			},
		}, out)

		require.NotNil(t, out.requestMirror)
		assert.Nil(t, out.requestMirror.disableShadowHostSuffixAppend)
		require.NotNil(t, out.requestMirror.hostRewriteLiteral)
		assert.Equal(t, "shadow.example:8080", *out.requestMirror.hostRewriteLiteral)

		input = "mutated"
		assert.Equal(t, "shadow.example:8080", *out.requestMirror.hostRewriteLiteral)
	})
}

func TestRequestMirrorIREquals(t *testing.T) {
	tests := []struct {
		name     string
		left     *requestMirrorIR
		right    *requestMirrorIR
		expected bool
	}{
		{name: "both nil", expected: true},
		{name: "nil and non-nil", right: requestMirrorIRWithValue(true)},
		{name: "both fields nil", left: &requestMirrorIR{}, right: &requestMirrorIR{}, expected: true},
		{name: "same value", left: requestMirrorIRWithValue(true), right: requestMirrorIRWithValue(true), expected: true},
		{name: "different values", left: requestMirrorIRWithValue(true), right: requestMirrorIRWithValue(false)},
		{name: "same literal", left: requestMirrorIRWithLiteral("a"), right: requestMirrorIRWithLiteral("a"), expected: true},
		{name: "different literals", left: requestMirrorIRWithLiteral("a"), right: requestMirrorIRWithLiteral("b")},
		{name: "literal set vs unset", left: requestMirrorIRWithLiteral("a"), right: &requestMirrorIR{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.left.Equals(tt.right))
			assert.Equal(t, tt.expected, tt.right.Equals(tt.left))
		})
	}
}

func TestApplyRequestMirror(t *testing.T) {
	t.Run("true updates a single mirror", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{}
		mirror := &envoyroutev3.RouteAction_RequestMirrorPolicy{}
		route := routeWithMirrors(mirror)

		plugin.applyRequestMirror(requestMirrorIRWithValue(true), route)

		assert.True(t, mirror.DisableShadowHostSuffixAppend)
	})

	t.Run("all non-nil mirrors are updated", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{}
		first := &envoyroutev3.RouteAction_RequestMirrorPolicy{}
		second := &envoyroutev3.RouteAction_RequestMirrorPolicy{}
		route := routeWithMirrors(first, nil, second)

		plugin.applyRequestMirror(requestMirrorIRWithValue(true), route)

		assert.True(t, first.DisableShadowHostSuffixAppend)
		assert.True(t, second.DisableShadowHostSuffixAppend)
	})

	t.Run("explicit false updates the mirror", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{}
		mirror := &envoyroutev3.RouteAction_RequestMirrorPolicy{DisableShadowHostSuffixAppend: true}
		route := routeWithMirrors(mirror)

		plugin.applyRequestMirror(requestMirrorIRWithValue(false), route)

		assert.False(t, mirror.DisableShadowHostSuffixAppend)
	})

	t.Run("no mirrors is a clean no-op", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{}
		route := routeWithMirrors()

		plugin.applyRequestMirror(requestMirrorIRWithValue(true), route)

		assert.Empty(t, route.GetRoute().GetRequestMirrorPolicies())
		assert.Contains(t, plugin.requestMirrorConfigured, route.GetName())
	})

	t.Run("unset value does not modify mirrors", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{}
		mirror := &envoyroutev3.RouteAction_RequestMirrorPolicy{DisableShadowHostSuffixAppend: true}
		route := routeWithMirrors(mirror)

		plugin.applyRequestMirror(&requestMirrorIR{}, route)

		assert.True(t, mirror.DisableShadowHostSuffixAppend)
		assert.Nil(t, plugin.requestMirrorConfigured)
	})

	t.Run("more specific false blocks later true", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{}
		mirror := &envoyroutev3.RouteAction_RequestMirrorPolicy{DisableShadowHostSuffixAppend: true}
		route := routeWithMirrors(mirror)

		plugin.applyRequestMirror(requestMirrorIRWithValue(false), route)
		plugin.applyRequestMirror(requestMirrorIRWithValue(true), route)

		assert.False(t, mirror.DisableShadowHostSuffixAppend)
	})

	t.Run("distinct routes are tracked independently", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{}
		mirrorA := &envoyroutev3.RouteAction_RequestMirrorPolicy{}
		mirrorB := &envoyroutev3.RouteAction_RequestMirrorPolicy{}
		routeA := namedRouteWithMirrors("route-a", mirrorA)
		routeB := namedRouteWithMirrors("route-b", mirrorB)

		// Configuring route-a must not mark route-b, so route-b still takes its own value.
		plugin.applyRequestMirror(requestMirrorIRWithValue(false), routeA)
		plugin.applyRequestMirror(requestMirrorIRWithValue(true), routeB)

		assert.False(t, mirrorA.DisableShadowHostSuffixAppend)
		assert.True(t, mirrorB.DisableShadowHostSuffixAppend)
	})

	t.Run("literal-only policy applies the literal", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{}
		mirror := &envoyroutev3.RouteAction_RequestMirrorPolicy{}
		route := routeWithMirrors(mirror)

		plugin.applyRequestMirror(requestMirrorIRWithLiteral("shadow.example:8080"), route)

		assert.Equal(t, "shadow.example:8080", mirror.HostRewriteLiteral)
		assert.False(t, mirror.DisableShadowHostSuffixAppend)
	})

	t.Run("both fields are applied independently", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{}
		// Start at true so an explicit false must be actively written, not just left as the zero value.
		mirror := &envoyroutev3.RouteAction_RequestMirrorPolicy{DisableShadowHostSuffixAppend: true}
		route := routeWithMirrors(mirror)

		disable := false
		literal := "shadow.example:8080"
		plugin.applyRequestMirror(&requestMirrorIR{
			disableShadowHostSuffixAppend: &disable,
			hostRewriteLiteral:            &literal,
		}, route)

		assert.Equal(t, "shadow.example:8080", mirror.HostRewriteLiteral)
		assert.False(t, mirror.DisableShadowHostSuffixAppend)
	})
}

func requestMirrorIRWithValue(value bool) *requestMirrorIR {
	return &requestMirrorIR{disableShadowHostSuffixAppend: &value}
}

func requestMirrorIRWithLiteral(value string) *requestMirrorIR {
	return &requestMirrorIR{hostRewriteLiteral: &value}
}

func routeWithMirrors(mirrors ...*envoyroutev3.RouteAction_RequestMirrorPolicy) *envoyroutev3.Route {
	return namedRouteWithMirrors("test-route", mirrors...)
}

func namedRouteWithMirrors(name string, mirrors ...*envoyroutev3.RouteAction_RequestMirrorPolicy) *envoyroutev3.Route {
	return &envoyroutev3.Route{
		Name: name,
		Action: &envoyroutev3.Route_Route{
			Route: &envoyroutev3.RouteAction{RequestMirrorPolicies: mirrors},
		},
	}
}
