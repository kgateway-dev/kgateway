package mirror_test

import (
	"context"
	"testing"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/mirror"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/mirror/mocks"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

//go:generate mockgen -destination mocks/mock_queries.go -package mocks github.com/solo-io/gloo/projects/gateway2/query GatewayQueries

func TestSingleMirror(t *testing.T) {
	g := gomega.NewWithT(t)
	ctrl := gomock.NewController(t)
	queries := mocks.NewMockGatewayQueries(ctrl)
	rt := &gwv1.HTTPRoute{}
	ctx := &filterplugins.RouteContext{
		Ctx:     context.Background(),
		Queries: queries,
		Route:   rt,
	}
	filter := gwv1.HTTPRouteFilter{
		Type: gwv1.HTTPRouteFilterRequestMirror,
		RequestMirror: &gwv1.HTTPRequestMirrorFilter{
			BackendRef: gwv1.BackendObjectReference{
				Name: "foo",
				Port: ptr(gwv1.PortNumber(8080)),
			},
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
	}
	queries.EXPECT().ObjToFrom(rt).Return(nil)
	queries.EXPECT().GetBackendForRef(ctx.Ctx, gomock.Any(), &filter.RequestMirror.BackendRef).Return(svc, nil)
	plugin := mirror.NewPlugin()
	outputRoute := &routev3.Route{
		Action: &routev3.Route_Route{
			Route: &routev3.RouteAction{},
		},
	}
	plugin.ApplyFilter(ctx, filter, outputRoute)

	g.Expect(outputRoute.GetRoute().RequestMirrorPolicies).To(gomega.HaveLen(1))
	g.Expect(outputRoute.GetRoute().RequestMirrorPolicies[0].Cluster).To(gomega.Equal("bar-foo-8080"))
}

func TestMultipleMirrors(t *testing.T) {
	g := gomega.NewWithT(t)
	ctrl := gomock.NewController(t)
	queries := mocks.NewMockGatewayQueries(ctrl)
	rt := &gwv1.HTTPRoute{}
	ctx := &filterplugins.RouteContext{
		Ctx:     context.Background(),
		Queries: queries,
		Route:   rt,
	}
	filter1 := gwv1.HTTPRouteFilter{
		Type: gwv1.HTTPRouteFilterRequestMirror,
		RequestMirror: &gwv1.HTTPRequestMirrorFilter{
			BackendRef: gwv1.BackendObjectReference{
				Name: "foo",
				Port: ptr(gwv1.PortNumber(8080)),
			},
		},
	}
	filter2 := gwv1.HTTPRouteFilter{
		Type: gwv1.HTTPRouteFilterRequestMirror,
		RequestMirror: &gwv1.HTTPRequestMirrorFilter{
			BackendRef: gwv1.BackendObjectReference{
				Name: "bar",
				Port: ptr(gwv1.PortNumber(8080)),
			},
		},
	}
	svc1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
	}
	svc2 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
	}
	queries.EXPECT().ObjToFrom(rt).Return(nil)
	queries.EXPECT().GetBackendForRef(ctx.Ctx, gomock.Any(), &filter1.RequestMirror.BackendRef).Return(svc1, nil)
	queries.EXPECT().ObjToFrom(rt).Return(nil)
	queries.EXPECT().GetBackendForRef(ctx.Ctx, gomock.Any(), &filter2.RequestMirror.BackendRef).Return(svc2, nil)

	plugin := mirror.NewPlugin()
	outputRoute := &routev3.Route{
		Action: &routev3.Route_Route{
			Route: &routev3.RouteAction{},
		},
	}
	plugin.ApplyFilter(ctx, filter1, outputRoute)
	plugin.ApplyFilter(ctx, filter2, outputRoute)

	g.Expect(outputRoute.GetRoute().RequestMirrorPolicies).To(gomega.HaveLen(2))
	g.Expect(outputRoute.GetRoute().RequestMirrorPolicies[0].Cluster).To(gomega.Equal("bar-foo-8080"))
	g.Expect(outputRoute.GetRoute().RequestMirrorPolicies[1].Cluster).To(gomega.Equal("foo-bar-8080"))
}

func ptr[T any](i T) *T {
	return &i
}
