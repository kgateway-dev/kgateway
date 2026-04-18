package proxy_syncer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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

	t.Run("falls back to v1alpha2 tlsroute when promoted route is absent", func(t *testing.T) {
		kubeClient := newFakeTLSRouteClient(t, &gwv1a2.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "route",
				Namespace: "default",
			},
		})

		route, err := getTLSRouteForStatus(context.Background(), kubeClient, types.NamespacedName{Name: "route", Namespace: "default"})
		require.NoError(t, err)
		require.IsType(t, &gwv1a2.TLSRoute{}, route)
	})

	t.Run("falls back to v1alpha3 tlsroute when promoted route is absent", func(t *testing.T) {
		kubeClient := newFakeTLSRouteClient(t, tlsRouteV1Alpha3Unstructured("route"))

		route, err := getTLSRouteForStatus(context.Background(), kubeClient, types.NamespacedName{Name: "route", Namespace: "default"})
		require.NoError(t, err)
		require.IsType(t, &unstructured.Unstructured{}, route)
		require.Equal(t, wellknown.TLSRouteV1Alpha3GVK, route.GetObjectKind().GroupVersionKind())
	})
}

func TestUpdateUnstructuredTLSRouteStatus(t *testing.T) {
	kubeClient := newFakeTLSRouteClient(t, tlsRouteV1Alpha3Unstructured("route"))

	routeObj, err := getTLSRouteForStatus(context.Background(), kubeClient, types.NamespacedName{Name: "route", Namespace: "default"})
	require.NoError(t, err)

	routeStatus := gwv1.RouteStatus{
		Parents: []gwv1.RouteParentStatus{{
			ParentRef:      gwv1.ParentReference{Name: "gateway"},
			ControllerName: gwv1.GatewayController("kgateway.dev/kgateway"),
		}},
	}

	require.NoError(t, updateUnstructuredTLSRouteStatus(context.Background(), kubeClient.Status(), routeObj.(*unstructured.Unstructured), routeStatus))

	updated := &unstructured.Unstructured{}
	updated.SetGroupVersionKind(wellknown.TLSRouteV1Alpha3GVK)
	require.NoError(t, kubeClient.Get(context.Background(), types.NamespacedName{Name: "route", Namespace: "default"}, updated))

	converted := collections.ConvertUnstructuredTLSRouteToV1Alpha2ForStatus(updated)
	require.NotNil(t, converted)
	require.Equal(t, routeStatus, converted.Status.RouteStatus)
}

func newFakeTLSRouteClient(t *testing.T, route ctrlclient.Object) ctrlclient.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	gwv1.Install(scheme)
	gwv1a2.Install(scheme)

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(route).
		WithObjects(route).
		Build()
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
