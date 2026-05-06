package proxy_syncer

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

func TestGetTLSRouteForStatus(t *testing.T) {
	t.Run("prefers promoted v1 tlsroute when present", func(t *testing.T) {
		kubeClient := newFakeTLSRouteClient(t, &gwv1.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "route",
				Namespace: "default",
			},
		})

		route, err := getTLSRouteForStatus(context.Background(), kubeClient, types.NamespacedName{Name: "route", Namespace: "default"})
		require.NoError(t, err)
		require.IsType(t, &gwv1.TLSRoute{}, route)
	})

	t.Run("returns not found when promoted route is absent", func(t *testing.T) {
		kubeClient := newFakeTLSRouteClient(t)

		route, err := getTLSRouteForStatus(context.Background(), kubeClient, types.NamespacedName{Name: "route", Namespace: "default"})
		require.True(t, apierrors.IsNotFound(err))
		require.Nil(t, route)
	})

	t.Run("falls back to v1alpha2 tlsroute when promoted versions are not served", func(t *testing.T) {
		kubeClient := &tlsRouteFallbackGetter{
			v1Err:       noTLSRouteKindMatch(gwv1.GroupVersion.Version),
			v1alpha3Err: noTLSRouteKindMatch(wellknown.TLSRouteV1Alpha3Version),
			v1alpha2Route: &gwv1a2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "route",
					Namespace: "default",
				},
			},
		}

		route, err := getTLSRouteForStatus(context.Background(), kubeClient, types.NamespacedName{Name: "route", Namespace: "default"})
		require.NoError(t, err)
		require.IsType(t, &gwv1a2.TLSRoute{}, route)
	})

	t.Run("falls back to v1alpha3 tlsroute when promoted route is not served", func(t *testing.T) {
		kubeClient := &tlsRouteFallbackGetter{
			v1Err:         noTLSRouteKindMatch(gwv1.GroupVersion.Version),
			v1alpha3Route: tlsRouteV1Alpha3Unstructured("route"),
		}

		route, err := getTLSRouteForStatus(context.Background(), kubeClient, types.NamespacedName{Name: "route", Namespace: "default"})
		require.NoError(t, err)
		require.IsType(t, &unstructured.Unstructured{}, route)
		require.Equal(t, wellknown.TLSRouteV1Alpha3GVK, route.GetObjectKind().GroupVersionKind())
	})

	t.Run("does not fall back when promoted route is served but absent", func(t *testing.T) {
		kubeClient := newFakeTLSRouteClient(t, &gwv1a2.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "route",
				Namespace: "default",
			},
		})

		route, err := getTLSRouteForStatus(context.Background(), kubeClient, types.NamespacedName{Name: "missing", Namespace: "default"})
		require.True(t, apierrors.IsNotFound(err))
		require.Nil(t, route)
	})
}

func TestUpdateUnstructuredTLSRouteStatus(t *testing.T) {
	routeObj := tlsRouteV1Alpha3Unstructured("route")
	kubeClient := newFakeTLSRouteClient(t, routeObj)

	routeStatus := gwv1.RouteStatus{
		Parents: []gwv1.RouteParentStatus{{
			ParentRef:      gwv1.ParentReference{Name: "gateway"},
			ControllerName: gwv1.GatewayController("kgateway.dev/kgateway"),
		}},
	}

	require.NoError(t, updateUnstructuredTLSRouteStatus(context.Background(), kubeClient.Status(), routeObj, routeStatus))

	updated := &unstructured.Unstructured{}
	updated.SetGroupVersionKind(wellknown.TLSRouteV1Alpha3GVK)
	require.NoError(t, kubeClient.Get(context.Background(), types.NamespacedName{Name: "route", Namespace: "default"}, updated))

	converted := collections.ConvertUnstructuredTLSRouteToV1Alpha2ForStatus(updated)
	require.NotNil(t, converted)
	require.Equal(t, routeStatus, converted.Status.RouteStatus)
}

func newFakeTLSRouteClient(t *testing.T, routes ...ctrlclient.Object) ctrlclient.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	gwv1.Install(scheme)
	gwv1a2.Install(scheme)

	builder := fake.NewClientBuilder().WithScheme(scheme)
	if len(routes) > 0 {
		builder = builder.WithStatusSubresource(routes...).WithObjects(routes...)
	}
	return builder.Build()
}

func tlsRouteV1Alpha3Unstructured(name string) *unstructured.Unstructured {
	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(wellknown.TLSRouteV1Alpha3GVK)
	route.SetName(name)
	route.SetNamespace("default")
	route.Object["spec"] = map[string]any{
		"parentRefs": []any{
			map[string]any{
				"name": "gateway",
			},
		},
	}
	return route
}

type tlsRouteFallbackGetter struct {
	v1Err         error
	v1alpha3Err   error
	v1alpha3Route *unstructured.Unstructured
	v1alpha2Route *gwv1a2.TLSRoute
}

func (g *tlsRouteFallbackGetter) Get(_ context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object, _ ...ctrlclient.GetOption) error {
	switch route := obj.(type) {
	case *gwv1.TLSRoute:
		if g.v1Err != nil {
			return g.v1Err
		}
		return apierrors.NewNotFound(tlsRouteGroupResource(), key.Name)
	case *unstructured.Unstructured:
		if route.GroupVersionKind() != wellknown.TLSRouteV1Alpha3GVK {
			return fmt.Errorf("unexpected unstructured GVK %s", route.GroupVersionKind())
		}
		if g.v1alpha3Err != nil {
			return g.v1alpha3Err
		}
		if g.v1alpha3Route == nil {
			return apierrors.NewNotFound(tlsRouteGroupResource(), key.Name)
		}
		g.v1alpha3Route.DeepCopyInto(route)
		return nil
	case *gwv1a2.TLSRoute:
		if g.v1alpha2Route == nil {
			return apierrors.NewNotFound(tlsRouteGroupResource(), key.Name)
		}
		g.v1alpha2Route.DeepCopyInto(route)
		return nil
	default:
		return fmt.Errorf("unexpected TLSRoute status lookup type %T", obj)
	}
}

func noTLSRouteKindMatch(version string) error {
	return &apimeta.NoKindMatchError{
		GroupKind:        schema.GroupKind{Group: wellknown.GatewayGroup, Kind: wellknown.TLSRouteKind},
		SearchedVersions: []string{version},
	}
}

func tlsRouteGroupResource() schema.GroupResource {
	return schema.GroupResource{Group: wellknown.GatewayGroup, Resource: "tlsroutes"}
}
