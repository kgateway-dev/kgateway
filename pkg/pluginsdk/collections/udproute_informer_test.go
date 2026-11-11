package collections

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/test"
	"istio.io/istio/pkg/util/sets"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// Gateway API v1.5 ships a UDPRoute CRD that serves only v1alpha2. The v1 informer must not
// start a watch against it, or it never syncs and blocks controller startup.
func TestDelayedUDPRouteInformerDoesNotBlockWhenV1IsNotServed(t *testing.T) {
	stop := test.NewStop(t)
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme.MustGet())
	apiclient.RegisterTypes()

	client := kube.NewFakeClient()
	makeCRDWithVersions(t, client, wellknown.UDPRouteGVR, "v1.5.1", []apiextensionsv1.CustomResourceDefinitionVersion{
		{Name: gwv1a2.GroupVersion.Version, Served: true, Storage: true},
	})
	client.RunAndWait(stop)

	inf := newDelayedTypedInformer(context.Background(), client, wellknown.UDPRouteGVR, func() kclient.Informer[*gwv1.UDPRoute] {
		return kclient.NewFiltered[*gwv1.UDPRoute](client, kclient.Filter{})
	})
	inf.Start(stop)

	require.True(t, inf.HasSynced(), "a UDPRoute CRD without v1 must not block the cache barrier")
	require.Empty(t, inf.List(metav1.NamespaceAll, labels.Everything()))
}

func TestServesOnlyOtherVersions(t *testing.T) {
	cases := []struct {
		name   string
		served sets.Set[string]
		want   bool
	}{
		{name: "versions unreadable", served: nil, want: false},
		{name: "CRD absent", served: sets.New[string](), want: false},
		{name: "v1 served", served: sets.New(gwv1a2.GroupVersion.Version, gwv1.GroupVersion.Version), want: false},
		{name: "only v1alpha2 served", served: sets.New(gwv1a2.GroupVersion.Version), want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, servesOnlyOtherVersions(tc.served, wellknown.UDPRouteGVR))
		})
	}
}

func TestUDPRouteWriteVersionsEmptyWhenV1Unserved(t *testing.T) {
	require.Equal(t, []schema.GroupVersionResource{wellknown.UDPRouteGVR}, (&CommonCollections{}).UDPRouteWriteVersions(),
		"a CommonCollections built without InitCollections writes through v1")
	require.Empty(t, (&CommonCollections{udpRouteV1Unserved: true}).UDPRouteWriteVersions(),
		"no UDPRoute status writer when the CRD does not serve v1")
}
