package proxy_syncer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
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

	t.Run("falls back to legacy v1alpha2 tlsroute when promoted route is absent", func(t *testing.T) {
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
