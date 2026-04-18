package collections

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/test"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	extfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func TestDelayedTLSRouteV1Alpha3InformerReportsSyncedWithoutCRD_Issue13661(t *testing.T) {
	stop := test.NewStop(t)
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	apiclient.RegisterTypes()

	client := kube.NewFakeClient()
	inf := newDelayedTypedInformer(client, wellknown.TLSRouteV1Alpha3GVR, func() kclient.Informer[*gwv1a3.TLSRoute] {
		return kclient.NewFiltered[*gwv1a3.TLSRoute](client, kclient.Filter{})
	})
	inf.Start(stop)

	require.True(t, inf.HasSynced(), "missing v1alpha3 TLSRoute CRDs should not block startup")
	require.Empty(t, inf.List(metav1.NamespaceAll, labels.Everything()))
}

func TestDelayedTLSRouteV1Alpha3InformerBypassesCrdWatcherFilter_Issue13735(t *testing.T) {
	stop := test.NewStop(t)
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	apiclient.RegisterTypes()

	client := kube.NewFakeClient()
	makeGatewayAPIV141TLSRouteCRD(t, client)

	_, err := client.GatewayAPI().GatewayV1alpha3().TLSRoutes("default").Create(
		context.Background(),
		&gwv1a3.TLSRoute{
			TypeMeta: metav1.TypeMeta{
				APIVersion: wellknown.TLSRouteV1Alpha3GVK.GroupVersion().String(),
				Kind:       wellknown.TLSRouteKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "v1alpha3-route",
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

	require.False(t, client.CrdWatcher().KnownOrCallback(wellknown.TLSRouteV1Alpha3GVR, func(<-chan struct{}) {}),
		"Gateway API v1.4.x v1alpha3 TLSRoute should be filtered from CrdWatcher known state")

	inf := newDelayedTypedInformer(client, wellknown.TLSRouteV1Alpha3GVR, func() kclient.Informer[*gwv1a3.TLSRoute] {
		return kclient.NewFiltered[*gwv1a3.TLSRoute](client, kclient.Filter{})
	})
	inf.Start(stop)

	require.Eventually(t, inf.HasSynced, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		return len(inf.List("default", labels.Everything())) == 1
	}, time.Second, 10*time.Millisecond, "v1alpha3 TLSRoute should still be discoverable through the typed informer path")
}

func TestDelayedTLSRouteV1Alpha3InformerUsesOptimisticTypedWatchWhenCRDDiscoveryErrors(t *testing.T) {
	stop := test.NewStop(t)
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	apiclient.RegisterTypes()

	client := kube.NewFakeClient()
	makeGatewayAPIV141TLSRouteCRD(t, client)

	extClient, ok := client.Ext().(*extfake.Clientset)
	require.True(t, ok)
	extClient.PrependReactor("get", "customresourcedefinitions", func(action k8stesting.Action) (bool, runtime.Object, error) {
		getAction, ok := action.(k8stesting.GetAction)
		if !ok {
			return false, nil, nil
		}
		if getAction.GetName() != "tlsroutes.gateway.networking.k8s.io" {
			return false, nil, nil
		}
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Group: "apiextensions.k8s.io", Resource: "customresourcedefinitions"},
			getAction.GetName(),
			errors.New("rbac denied"),
		)
	})

	_, err := client.GatewayAPI().GatewayV1alpha3().TLSRoutes("default").Create(
		context.Background(),
		&gwv1a3.TLSRoute{
			TypeMeta: metav1.TypeMeta{
				APIVersion: wellknown.TLSRouteV1Alpha3GVK.GroupVersion().String(),
				Kind:       wellknown.TLSRouteKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "v1alpha3-route",
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

	inf := newDelayedTypedInformer(client, wellknown.TLSRouteV1Alpha3GVR, func() kclient.Informer[*gwv1a3.TLSRoute] {
		return kclient.NewFiltered[*gwv1a3.TLSRoute](client, kclient.Filter{})
	})
	inf.Start(stop)

	require.Eventually(t, inf.HasSynced, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		return len(inf.List("default", labels.Everything())) == 1
	}, time.Second, 10*time.Millisecond, "non-authoritative CRD discovery must not suppress TLSRoute watches")
}
