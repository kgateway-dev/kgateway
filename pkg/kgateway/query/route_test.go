package query_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestClonePreservesHostnameOverrides(t *testing.T) {
	original := &query.RouteInfo{
		Object: &ir.HttpRouteIR{
			ObjectSource: ir.ObjectSource{
				Name:      "test-route",
				Namespace: "default",
			},
		},
		HostnameOverrides: []string{"foo.com", "bar.com"},
	}

	clone := original.Clone()

	// The clone must have the same HostnameOverrides values.
	assert.Equal(t, original.HostnameOverrides, clone.HostnameOverrides,
		"clone should preserve HostnameOverrides")

	// Mutating the clone's HostnameOverrides must not affect the original.
	clone.HostnameOverrides[0] = "mutated.com"
	assert.Equal(t, "foo.com", original.HostnameOverrides[0],
		"mutating clone's HostnameOverrides should not affect original")
}

func TestGetRouteChain(t *testing.T) {
	tests := []struct {
		name         string
		parent       *ir.HttpRouteIR
		routes       []client.Object
		wantChildren int
		wantErr      error
	}{
		{
			name: "wildcard delegation without resolution errors",
			parent: &ir.HttpRouteIR{
				ObjectSource: ir.ObjectSource{
					Name:      "parent",
					Namespace: "default",
				},
				Rules: []ir.HttpRouteRuleIR{
					{
						Backends: []ir.HttpBackendOrDelegate{
							{
								Delegate: &ir.ObjectSource{
									Group:     "gateways.networking.k8s.io",
									Kind:      "HTTPRoute",
									Name:      "*",
									Namespace: "default",
								},
							},
						},
					},
				},
			},
			routes: []client.Object{
				// cyclic reference to self via implicit attachment
				&gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "parent",
						Namespace: "default",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Group: new(gwv1.Group("gateway.networking.k8s.io")),
									Kind:  new(gwv1.Kind("Gateway")),
									Name:  "gateway",
								},
							},
						},
					},
				},
				&gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "child1",
						Namespace: "default",
					},
					Spec: gwv1.HTTPRouteSpec{
						// No ParentRefs, implicit attachment
					},
				},
				&gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "child2",
						Namespace: "default",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Group: new(gwv1.Group("gateway.networking.k8s.io")),
									Kind:  new(gwv1.Kind("HTTPRoute")),
									Name:  "parent",
								},
							},
						},
					},
				},
				&gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "child3",
						Namespace: "default",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Group: new(gwv1.Group("gateway.networking.k8s.io")),
									Kind:  new(gwv1.Kind("HTTPRoute")),
									Name:  "invalid", // invalid parent
								},
							},
						},
					},
				},
				&gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "parent2",
						Namespace: "default",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Group: new(gwv1.Group("gateway.networking.k8s.io")),
									Kind:  new(gwv1.Kind("Gateway")),
									Name:  "gateway", // invalid parent
								},
							},
						},
					},
				},
			},
			wantChildren: 2,
			wantErr:      nil,
		},
		{
			name: "wildcard delegation resulting in cyclic reference to parent route",
			parent: &ir.HttpRouteIR{
				ObjectSource: ir.ObjectSource{
					Name:      "parent",
					Namespace: "default",
				},
				Rules: []ir.HttpRouteRuleIR{
					{
						Backends: []ir.HttpBackendOrDelegate{
							{
								Delegate: &ir.ObjectSource{
									Group:     "gateways.networking.k8s.io",
									Kind:      "HTTPRoute",
									Name:      "*",
									Namespace: "default",
								},
							},
						},
					},
				},
			},
			routes: []client.Object{
				// cyclic reference to self via implicit attachment
				&gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "parent",
						Namespace: "default",
					},
				},
				&gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "child1",
						Namespace: "default",
					},
					Spec: gwv1.HTTPRouteSpec{
						// No ParentRefs, implicit attachment
					},
				},
				&gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "child2",
						Namespace: "default",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Group: new(gwv1.Group("gateway.networking.k8s.io")),
									Kind:  new(gwv1.Kind("HTTPRoute")),
									Name:  "parent",
								},
							},
						},
					},
				},
				&gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "child3",
						Namespace: "default",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Group: new(gwv1.Group("gateway.networking.k8s.io")),
									Kind:  new(gwv1.Kind("HTTPRoute")),
									Name:  "invalid", // invalid parent
								},
							},
						},
					},
				},
				&gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "parent2",
						Namespace: "default",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Group: new(gwv1.Group("gateway.networking.k8s.io")),
									Kind:  new(gwv1.Kind("Gateway")),
									Name:  "gateway", // invalid parent
								},
							},
						},
					},
				},
			},
			wantChildren: 2,
			wantErr:      query.ErrCyclicReference,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)

			q := newQueries(t, tt.routes...)
			routeInfo := q.GetRouteChain(krt.TestingDummyContext{}, context.TODO(), tt.parent, nil, gwv1.ParentReference{})
			a.NotNil(routeInfo)

			children, err := routeInfo.GetChildrenForRef(*tt.parent.Rules[0].Backends[0].Delegate)
			if tt.wantErr != nil {
				a.Error(err)
			} else {
				a.NoError(err)
			}
			a.Len(children, tt.wantChildren)
		})
	}
}

func TestRouteKeyMatches(t *testing.T) {
	parents := []client.Object{
		nil,
		&gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "gw"}},
		&gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "ls"}},
		&gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Namespace: "other", Name: "ls"}},
		&gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Name: "ls"}},
	}
	listenerNames := []string{"https", "http", ""}

	keys := []string{"", "listener~80", "default/ls", "default/ls/", "default//https", "default/lsx/https", "default/ls/https/extra", "xdefault/ls/https"}
	for _, p := range parents {
		for _, ln := range listenerNames {
			keys = append(keys, query.GenerateRouteKey(p, ln))
		}
	}

	for _, p := range parents {
		for _, ln := range listenerNames {
			want := query.GenerateRouteKey(p, ln)
			for _, key := range keys {
				assert.Equal(t, key == want, query.RouteKeyMatches(key, p, ln),
					"RouteKeyMatches(%q, %T, %q) must agree with GenerateRouteKey %q", key, p, ln, want)
			}
		}
	}
}

// UniqueRouteName is the Envoy route name that policies and stats key on, so it must stay
// byte-identical to the fmt.Sprintf format it replaced.
func TestUniqueRouteNameMatchesSprintfFormat(t *testing.T) {
	sprintfName := func(kind, name, namespace string, ruleIdx, matchIdx int, ruleName string) string {
		if ruleName == "" {
			return fmt.Sprintf("%s-%s-%s-%d-%d", strings.ToLower(kind), name, namespace, ruleIdx, matchIdx)
		}
		return fmt.Sprintf("%s-%s-%s-%d-%d-%s", strings.ToLower(kind), name, namespace, ruleIdx, matchIdx, ruleName)
	}

	kinds := []string{"HTTPRoute", "GRPCRoute", "TCPRoute", "TLSRoute", "CustomRoute", "lowercaseroute", "ÜnicodeRoute", ""}
	indexes := [][2]int{{0, 0}, {1, 9}, {10, 99}, {100, 12345}}
	for _, kind := range kinds {
		for _, ruleName := range []string{"", "rule-a"} {
			for _, idx := range indexes {
				route := &query.RouteInfo{
					Object: &ir.HttpRouteIR{
						ObjectSource: ir.ObjectSource{Kind: kind, Name: "my-route", Namespace: "my-ns"},
					},
				}
				want := sprintfName(kind, "my-route", "my-ns", idx[0], idx[1], ruleName)
				assert.Equal(t, want, route.UniqueRouteName(idx[0], idx[1], ruleName),
					"kind %q, rule name %q, indexes %v", kind, ruleName, idx)
			}
		}
	}
}

func BenchmarkUniqueRouteName(b *testing.B) {
	route := &query.RouteInfo{
		Object: &ir.HttpRouteIR{
			ObjectSource: ir.ObjectSource{Kind: "HTTPRoute", Name: "ls-12-route-7", Namespace: "bench-ns-31"},
		},
	}
	for _, ruleName := range []string{"", "rule-a"} {
		b.Run(fmt.Sprintf("ruleName=%q", ruleName), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = route.UniqueRouteName(3, 1, ruleName)
			}
		})
	}
}
