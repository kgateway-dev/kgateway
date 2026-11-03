package httproute

import (
	"testing"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestMatchKey(t *testing.T) {
	prefix := func(v string) *gwv1.HTTPPathMatch {
		return &gwv1.HTTPPathMatch{Type: new(gwv1.PathMatchPathPrefix), Value: new(v)}
	}
	header := func(name, value string) gwv1.HTTPHeaderMatch {
		return gwv1.HTTPHeaderMatch{Name: gwv1.HTTPHeaderName(name), Value: value}
	}

	tests := []struct {
		name string
		a, b gwv1.HTTPRouteMatch
		same bool
	}{
		{
			name: "unset path is a PathPrefix on /",
			a:    gwv1.HTTPRouteMatch{},
			b:    gwv1.HTTPRouteMatch{Path: prefix("/")},
			same: true,
		},
		{
			name: "header order and name case do not matter",
			a:    gwv1.HTTPRouteMatch{Path: prefix("/a"), Headers: []gwv1.HTTPHeaderMatch{header("X-A", "1"), header("x-b", "2")}},
			b: gwv1.HTTPRouteMatch{Path: prefix("/a"), Headers: []gwv1.HTTPHeaderMatch{
				header("X-B", "2"), {Type: new(gwv1.HeaderMatchExact), Name: "x-a", Value: "1"},
			}},
			same: true,
		},
		{
			name: "different paths",
			a:    gwv1.HTTPRouteMatch{Path: prefix("/a")},
			b:    gwv1.HTTPRouteMatch{Path: prefix("/b")},
		},
		{
			name: "different path types",
			a:    gwv1.HTTPRouteMatch{Path: prefix("/a")},
			b:    gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: new(gwv1.PathMatchExact), Value: new("/a")}},
		},
		{
			name: "method set on one only",
			a:    gwv1.HTTPRouteMatch{Path: prefix("/a")},
			b:    gwv1.HTTPRouteMatch{Path: prefix("/a"), Method: new(gwv1.HTTPMethodGet)},
		},
		{
			name: "header values differ",
			a:    gwv1.HTTPRouteMatch{Path: prefix("/a"), Headers: []gwv1.HTTPHeaderMatch{header("x-a", "1")}},
			b:    gwv1.HTTPRouteMatch{Path: prefix("/a"), Headers: []gwv1.HTTPHeaderMatch{header("x-a", "2")}},
		},
		{
			name: "query param names are case-sensitive",
			a:    gwv1.HTTPRouteMatch{Path: prefix("/a"), QueryParams: []gwv1.HTTPQueryParamMatch{{Name: "q", Value: "1"}}},
			b:    gwv1.HTTPRouteMatch{Path: prefix("/a"), QueryParams: []gwv1.HTTPQueryParamMatch{{Name: "Q", Value: "1"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchKey(tt.a) == matchKey(tt.b); got != tt.same {
				t.Errorf("matchKey(a) == matchKey(b) = %v, want %v\na: %s\nb: %s", got, tt.same, matchKey(tt.a), matchKey(tt.b))
			}
		})
	}
}
