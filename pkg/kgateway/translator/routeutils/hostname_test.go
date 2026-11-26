package routeutils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestCompareHostnameSpecificity(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{name: "exact before wildcard", a: "abc.example.com", b: "*.example.com", want: -1},
		{name: "longer wildcard before shorter wildcard", a: "*.example.com", b: "*.com", want: -1},
		{name: "longer exact before shorter exact", a: "abc.example.com", b: "example.com", want: -1},
		{name: "short exact before long wildcard", a: "a.io", b: "*.very.long.example.com", want: -1},
		{name: "wildcard before no hostname", a: "*.com", b: "", want: -1},
		{name: "less specific sorts after", a: "*.example.com", b: "abc.example.com", want: 1},
		{name: "identical", a: "abc.example.com", b: "abc.example.com", want: 0},
		{name: "both empty", a: "", b: "", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CompareHostnameSpecificity(tt.a, tt.b))
		})
	}
}

func TestMostSpecificRouteHostname(t *testing.T) {
	tests := []struct {
		name           string
		routeHostnames []string
		host           string
		want           string
	}{
		{name: "no route hostnames", host: "abc.example.com", want: ""},
		{name: "exact hostname", routeHostnames: []string{"abc.example.com"}, host: "abc.example.com", want: "abc.example.com"},
		{name: "wildcard covering exact host", routeHostnames: []string{"*.example.com"}, host: "abc.example.com", want: "*.example.com"},
		{name: "wildcard covering wildcard host", routeHostnames: []string{"*.com"}, host: "*.example.com", want: "*.com"},
		{
			name:           "most specific covering hostname wins",
			routeHostnames: []string{"*.com", "abc.example.com", "*.example.com"},
			host:           "abc.example.com",
			want:           "abc.example.com",
		},
		{name: "non-covering hostnames are ignored", routeHostnames: []string{"xyz.example.com", "*.org"}, host: "abc.example.com", want: ""},
		{name: "wildcard does not cover its own suffix", routeHostnames: []string{"*.example.com"}, host: "example.com", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, MostSpecificRouteHostname(tt.routeHostnames, tt.host))
		})
	}
}

func TestCompareToRouteHostnameBeforeMatch(t *testing.T) {
	older, newer := defaultRt(), defaultRtB()
	newer.CreationTimestamp.Time = older.CreationTimestamp.Add(1)

	wildcardWithLongerPath := &SortableRoute{
		Route:         ir.HttpRouteRuleMatchIR{Match: prefixMatcher("/foo")},
		RouteObject:   older,
		RouteHostname: "*.example.com",
	}
	exact := &SortableRoute{
		Route:         ir.HttpRouteRuleMatchIR{Match: defaultMatcher()},
		RouteObject:   newer,
		RouteHostname: "abc.example.com",
	}
	assert.Equal(t, -1, exact.CompareTo(wildcardWithLongerPath), "the more specific route hostname should sort first")
	assert.Equal(t, 1, wildcardWithLongerPath.CompareTo(exact), "the less specific route hostname should sort last")

	exact.Route.PrecedenceWeight = -1
	assert.Equal(t, 1, exact.CompareTo(wildcardWithLongerPath), "precedence weight should be compared before route hostname")
}

// A route without hostnames is the least specific; it does not inherit the
// listener hostname (kubernetes-sigs/gateway-api#5392). On a foo.com listener,
// a route with hostnames [foo.com] and PathPrefix / sorts before a route
// without hostnames and PathPrefix /api.
func TestCompareToRouteWithoutHostnamesIsLeastSpecific(t *testing.T) {
	noHostnames := &SortableRoute{
		Route:       ir.HttpRouteRuleMatchIR{Match: prefixMatcher("/api")},
		RouteObject: defaultRt(),
	}
	withHostname := &SortableRoute{
		Route:         ir.HttpRouteRuleMatchIR{Match: defaultMatcher()},
		RouteObject:   defaultRtB(),
		RouteHostname: MostSpecificRouteHostname([]string{"foo.com"}, "foo.com"),
	}
	assert.Equal(t, -1, withHostname.CompareTo(noHostnames), "the route with hostnames should sort first despite its shorter path")
	assert.Equal(t, 1, noHostnames.CompareTo(withHostname), "the route without hostnames should sort last")
}
