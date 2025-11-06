package query_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

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
									Group: ptr.To(gwv1.Group("gateway.networking.k8s.io")),
									Kind:  ptr.To(gwv1.Kind("Gateway")),
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
									Group: ptr.To(gwv1.Group("gateway.networking.k8s.io")),
									Kind:  ptr.To(gwv1.Kind("HTTPRoute")),
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
									Group: ptr.To(gwv1.Group("gateway.networking.k8s.io")),
									Kind:  ptr.To(gwv1.Kind("HTTPRoute")),
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
									Group: ptr.To(gwv1.Group("gateway.networking.k8s.io")),
									Kind:  ptr.To(gwv1.Kind("Gateway")),
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
									Group: ptr.To(gwv1.Group("gateway.networking.k8s.io")),
									Kind:  ptr.To(gwv1.Kind("HTTPRoute")),
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
									Group: ptr.To(gwv1.Group("gateway.networking.k8s.io")),
									Kind:  ptr.To(gwv1.Kind("HTTPRoute")),
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
									Group: ptr.To(gwv1.Group("gateway.networking.k8s.io")),
									Kind:  ptr.To(gwv1.Kind("Gateway")),
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
