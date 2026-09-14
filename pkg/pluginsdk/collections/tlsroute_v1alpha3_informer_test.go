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
	fakediscovery "k8s.io/client-go/discovery/fake"
	k8stesting "k8s.io/client-go/testing"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func TestDelayedTLSRouteV1Alpha3InformerReportsSyncedWithoutCRD_Issue13661(t *testing.T) {
	stop := test.NewStop(t)
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	apiclient.RegisterTypes()

	client := kube.NewFakeClient()
	inf := newDelayedTypedInformer(context.Background(), client, wellknown.TLSRouteV1Alpha3GVR, func() kclient.Informer[*gwv1a3.TLSRoute] {
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

	inf := newDelayedTypedInformer(context.Background(), client, wellknown.TLSRouteV1Alpha3GVR, func() kclient.Informer[*gwv1a3.TLSRoute] {
		return kclient.NewFiltered[*gwv1a3.TLSRoute](client, kclient.Filter{})
	})
	inf.Start(stop)

	require.Eventually(t, inf.HasSynced, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		return len(inf.List("default", labels.Everything())) == 1
	}, time.Second, 10*time.Millisecond, "v1alpha3 TLSRoute should still be discoverable through the typed informer path")
}

// An RBAC gap on customresourcedefinitions used to be the worst case: the CRD read failed,
// the informer started anyway, and if the version happened not to be served its initial list
// could only 404 — never syncing, and hanging the control plane's cache barrier behind it.
// The discovery API answers the same question through an endpoint every authenticated client
// can reach, so the watch now starts for the right reason instead of on a guess.
func TestDelayedTLSRouteV1Alpha3InformerRecoversThroughDiscoveryWhenCRDReadIsDenied(t *testing.T) {
	stop := test.NewStop(t)
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	apiclient.RegisterTypes()

	client := kube.NewFakeClient()
	makeGatewayAPIV141TLSRouteCRD(t, client)
	denyCRDReads(t, client)
	serveInDiscovery(t, client, wellknown.TLSRouteV1Alpha3GVR)
	createV1Alpha3TLSRoute(t, client)

	client.RunAndWait(stop)

	inf := newDelayedTypedInformer(context.Background(), client, wellknown.TLSRouteV1Alpha3GVR, func() kclient.Informer[*gwv1a3.TLSRoute] {
		return kclient.NewFiltered[*gwv1a3.TLSRoute](client, kclient.Filter{})
	})
	inf.Start(stop)

	require.Eventually(t, inf.HasSynced, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		return len(inf.List("default", labels.Everything())) == 1
	}, time.Second, 10*time.Millisecond, "the discovery fallback should recover the watch the CRD read could not authorize")
}

// When neither reader can answer, the informer parks. That does cost the kind its watch until
// one of them recovers — the poll loop keeps asking — but the alternative is starting a watch
// on an unverified version, and if that version is not served its list 404s forever while
// HasSynced holds the whole cache barrier. A kind that is late is recoverable; a control plane
// that never finishes syncing is not.
func TestDelayedTLSRouteV1Alpha3InformerParksWhenNoReaderCanAnswer(t *testing.T) {
	stop := test.NewStop(t)
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	apiclient.RegisterTypes()

	client := kube.NewFakeClient()
	makeGatewayAPIV141TLSRouteCRD(t, client)
	denyCRDReads(t, client)
	createV1Alpha3TLSRoute(t, client)

	client.RunAndWait(stop)

	inf := newDelayedTypedInformer(context.Background(), client, wellknown.TLSRouteV1Alpha3GVR, func() kclient.Informer[*gwv1a3.TLSRoute] {
		return kclient.NewFiltered[*gwv1a3.TLSRoute](client, kclient.Filter{})
	})
	inf.Start(stop)

	require.True(t, inf.HasSynced(), "an unresolved version must not hold the cache barrier")
	require.Empty(t, inf.List("default", labels.Everything()))
}

func denyCRDReads(t *testing.T, client kube.Client) {
	t.Helper()

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
}

func serveInDiscovery(t *testing.T, client kube.Client, gvr schema.GroupVersionResource) {
	t.Helper()

	disco, ok := client.Kube().Discovery().(*fakediscovery.FakeDiscovery)
	require.True(t, ok)
	disco.Resources = append(disco.Resources, &metav1.APIResourceList{
		GroupVersion: gvr.GroupVersion().String(),
		APIResources: []metav1.APIResource{{Name: gvr.Resource}},
	})
}

func createV1Alpha3TLSRoute(t *testing.T, client kube.Client) {
	t.Helper()

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
}

// Promoted TLSRoute goes through newDelayedTypedInformer rather than istio's
// kclient.NewDelayedInformer because istio's CRD watcher keys readiness on
// <resource>.<group> and ignores the version: it would report v1 as ready off a CRD that
// serves no v1, start a real informer against an endpoint the API server does not serve, and
// never sync — blocking every collection gated on it, up to the proxy syncer cache barrier.
//
// Istio guards that with minimumVersionFilter, whose hardcoded minimum for tlsroutes is the
// Gateway API release where v1 appeared, so ordinary old installs are filtered out entirely
// (see the v1.4.1 case above). This pins the window that filter leaves open — a new enough
// CRD bundle that nonetheless does not serve v1 — where only checking the served version
// keeps startup unblocked.
func TestDelayedTLSRouteV1InformerDoesNotBlockWhenV1IsNotServed(t *testing.T) {
	stop := test.NewStop(t)
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	apiclient.RegisterTypes()

	client := kube.NewFakeClient()
	makeCRDWithVersions(t, client, wellknown.TLSRouteV1GVR, "v1.5.0", []apiextensionsv1.CustomResourceDefinitionVersion{
		{Name: gwv1a2.GroupVersion.Version, Served: true, Storage: true},
		{Name: gwv1.GroupVersion.Version, Served: false},
	})
	client.RunAndWait(stop)

	require.True(t, client.CrdWatcher().KnownOrCallback(wellknown.TLSRouteV1GVR, func(<-chan struct{}) {}),
		"istio's watcher ignores the version, so a new enough CRD bundle makes it report v1 as known "+
			"even though v1 is not served; the istio delayed informer would start a watch that never syncs")

	inf := newDelayedTypedInformer(context.Background(), client, wellknown.TLSRouteV1GVR, func() kclient.Informer[*gwv1.TLSRoute] {
		return kclient.NewFiltered[*gwv1.TLSRoute](client, kclient.Filter{})
	})
	inf.Start(stop)

	require.True(t, inf.HasSynced(), "an unserved promoted version must not block the cache barrier")
	require.Empty(t, inf.List(metav1.NamespaceAll, labels.Everything()))
}
