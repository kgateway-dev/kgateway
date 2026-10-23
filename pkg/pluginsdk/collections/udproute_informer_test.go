package collections

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/test"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// Gateway API v1.5 ships a UDPRoute CRD that serves only v1alpha2. The v1 informer must not
// start a watch against it, or it never syncs and blocks controller startup.
func TestDelayedUDPRouteInformerDoesNotBlockWhenV1IsNotServed(t *testing.T) {
	stop := test.NewStop(t)
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	apiclient.RegisterTypes()

	client := kube.NewFakeClient()
	makeCRDWithVersions(t, client, wellknown.UDPRouteGVR, "v1.5.1", []apiextensionsv1.CustomResourceDefinitionVersion{
		{Name: gwv1a2.GroupVersion.Version, Served: true, Storage: true},
	})
	client.RunAndWait(stop)

	require.True(t, client.CrdWatcher().KnownOrCallback(wellknown.UDPRouteGVR, func(<-chan struct{}) {}),
		"istio's watcher ignores the version, so it reports v1 as known and its delayed informer would start a watch that never syncs")

	inf := newDelayedTypedInformer(context.Background(), client, wellknown.UDPRouteGVR, func() kclient.Informer[*gwv1.UDPRoute] {
		return kclient.NewFiltered[*gwv1.UDPRoute](client, kclient.Filter{})
	})
	inf.Start(stop)

	require.True(t, inf.HasSynced(), "a UDPRoute CRD without v1 must not block the cache barrier")
	require.Empty(t, inf.List(metav1.NamespaceAll, labels.Everything()))
}
