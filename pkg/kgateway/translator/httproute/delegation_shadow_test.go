package httproute

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	statusreports "github.com/kgateway-dev/kgateway/v2/pkg/reports"
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

func TestReportShadowedDelegatedMatchesSkipsSingleDelegation(t *testing.T) {
	routeIR := func(ns, name string) *ir.HttpRouteIR {
		return &ir.HttpRouteIR{
			ObjectSource: ir.ObjectSource{Group: gwv1.GroupName, Kind: "HTTPRoute", Namespace: ns, Name: name},
			SourceObject: &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name}},
		}
	}
	parent := routeIR("infra", "parent")
	child := routeIR("a", "child")
	gatewayRef := gwv1.ParentReference{Name: "gw"}
	childRef := gwv1.ParentReference{Kind: new(gwv1.Kind("HTTPRoute")), Name: "parent", Namespace: new(gwv1.Namespace("infra"))}

	// Both copies of the child rule are delegated by the same parent rule, as when one
	// rule's backendRefs select the same child twice: both copies send traffic to the
	// same child rule, so nothing is misrouted.
	delegating := &ir.HttpRouteRuleMatchIR{Parent: parent, ParentRef: gatewayRef, Delegates: true}
	leaf := ir.HttpRouteRuleMatchIR{
		Parent:           child,
		ParentRef:        childRef,
		DelegatingParent: delegating,
		Match:            gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: new(gwv1.PathMatchPathPrefix), Value: new("/a/1")}},
	}

	reportMap := statusreports.NewReportMap()
	reportShadowedDelegatedMatches(&query.RouteInfo{Object: parent}, []ir.HttpRouteRuleMatchIR{leaf, leaf}, statusreports.NewReporter(&reportMap))

	for nn, rr := range reportMap.HTTPRoutes {
		for ref, prr := range rr.Parents {
			if len(prr.Conditions) > 0 {
				t.Errorf("HTTPRoute %v parentRef %v: got conditions %v, want none", nn, ref, prr.Conditions)
			}
		}
	}
}
