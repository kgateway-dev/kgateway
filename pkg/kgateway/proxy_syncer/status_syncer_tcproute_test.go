package proxy_syncer

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func TestGetTCPRouteForStatus(t *testing.T) {
	t.Run("prefers promoted v1 tcproute when present", func(t *testing.T) {
		kubeClient := newFakeTCPRouteClient(t, &gwv1.TCPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "route",
				Namespace: "default",
			},
		})

		route, err := getTCPRouteForStatus(context.Background(), kubeClient, types.NamespacedName{Name: "route", Namespace: "default"})
		require.NoError(t, err)
		require.IsType(t, &gwv1.TCPRoute{}, route)
	})

	t.Run("returns not found when promoted route is absent", func(t *testing.T) {
		kubeClient := newFakeTCPRouteClient(t)

		route, err := getTCPRouteForStatus(context.Background(), kubeClient, types.NamespacedName{Name: "route", Namespace: "default"})
		require.True(t, apierrors.IsNotFound(err))
		require.Nil(t, route)
	})

	t.Run("falls back to v1alpha2 tcproute when promoted version is not served", func(t *testing.T) {
		kubeClient := &tcpRouteFallbackGetter{
			v1Err: noTCPRouteKindMatch(gwv1.GroupVersion.Version),
			v1alpha2Route: &gwv1a2.TCPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "route",
					Namespace: "default",
				},
			},
		}

		route, err := getTCPRouteForStatus(context.Background(), kubeClient, types.NamespacedName{Name: "route", Namespace: "default"})
		require.NoError(t, err)
		require.IsType(t, &gwv1a2.TCPRoute{}, route)
	})

	t.Run("does not fall back when promoted route is served but absent", func(t *testing.T) {
		kubeClient := newFakeTCPRouteClient(t, &gwv1a2.TCPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "route",
				Namespace: "default",
			},
		})

		route, err := getTCPRouteForStatus(context.Background(), kubeClient, types.NamespacedName{Name: "missing", Namespace: "default"})
		require.True(t, apierrors.IsNotFound(err))
		require.Nil(t, route)
	})
}

func newFakeTCPRouteClient(t *testing.T, routes ...ctrlclient.Object) ctrlclient.Client {
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

type tcpRouteFallbackGetter struct {
	v1Err         error
	v1alpha2Route *gwv1a2.TCPRoute
}

func (g *tcpRouteFallbackGetter) Get(_ context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object, _ ...ctrlclient.GetOption) error {
	switch route := obj.(type) {
	case *gwv1.TCPRoute:
		if g.v1Err != nil {
			return g.v1Err
		}
		return apierrors.NewNotFound(tcpRouteGroupResource(), key.Name)
	case *gwv1a2.TCPRoute:
		if g.v1alpha2Route == nil {
			return apierrors.NewNotFound(tcpRouteGroupResource(), key.Name)
		}
		g.v1alpha2Route.DeepCopyInto(route)
		return nil
	default:
		return fmt.Errorf("unexpected TCPRoute status lookup type %T", obj)
	}
}

func noTCPRouteKindMatch(version string) error {
	return &apimeta.NoKindMatchError{
		GroupKind:        schema.GroupKind{Group: wellknown.GatewayGroup, Kind: wellknown.TCPRouteKind},
		SearchedVersions: []string{version},
	}
}

func tcpRouteGroupResource() schema.GroupResource {
	return schema.GroupResource{Group: wellknown.GatewayGroup, Resource: "tcproutes"}
}
