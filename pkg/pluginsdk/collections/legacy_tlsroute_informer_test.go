package collections

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/test"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func TestDelayedLegacyTLSRouteInformerReportsSyncedWithoutCRD_Issue13661(t *testing.T) {
	stop := test.NewStop(t)
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	apiclient.RegisterTypes()

	client := kube.NewFakeClient()
	inf := newDelayedTypedInformer(client, wellknown.LegacyTLSRouteGVR, func() kclient.Informer[*gwv1a3.TLSRoute] {
		return kclient.NewFiltered[*gwv1a3.TLSRoute](client, kclient.Filter{})
	})
	inf.Start(stop)

	require.True(t, inf.HasSynced(), "missing legacy TLSRoute CRDs should not block startup")
	require.Empty(t, inf.List(metav1.NamespaceAll, labels.Everything()))
}

func TestDelayedLegacyTLSRouteInformerBypassesCrdWatcherFilterForLegacyTLSRoute_Issue13735(t *testing.T) {
	stop := test.NewStop(t)
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	apiclient.RegisterTypes()

	client := kube.NewFakeClient()
	makeServedCRD(t, client, wellknown.LegacyTLSRouteGVR, "v1.4.1")

	_, err := client.GatewayAPI().GatewayV1alpha3().TLSRoutes("default").Create(
		context.Background(),
		&gwv1a3.TLSRoute{
			TypeMeta: metav1.TypeMeta{
				APIVersion: wellknown.LegacyTLSRouteGVK.GroupVersion().String(),
				Kind:       wellknown.TLSRouteKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "legacy-route",
				Namespace: "default",
			},
			Spec: gwv1.TLSRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{{
						Name: "gateway",
					}},
				},
				Hostnames: []gwv1.Hostname{"example.com"},
			},
		},
		metav1.CreateOptions{},
	)
	require.NoError(t, err)

	client.RunAndWait(stop)

	require.False(t, client.CrdWatcher().KnownOrCallback(wellknown.LegacyTLSRouteGVR, func(<-chan struct{}) {}),
		"Gateway API v1.4.x legacy TLSRoute should be filtered from CrdWatcher known state")

	inf := newDelayedTypedInformer(client, wellknown.LegacyTLSRouteGVR, func() kclient.Informer[*gwv1a3.TLSRoute] {
		return kclient.NewFiltered[*gwv1a3.TLSRoute](client, kclient.Filter{})
	})
	inf.Start(stop)

	require.Eventually(t, inf.HasSynced, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		return len(inf.List("default", labels.Everything())) == 1
	}, time.Second, 10*time.Millisecond, "legacy TLSRoute should still be discoverable through the typed informer path")
}
