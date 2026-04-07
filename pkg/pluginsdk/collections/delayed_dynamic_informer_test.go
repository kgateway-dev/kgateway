package collections

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	istiogvr "istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/kclient/clienttest"
	"istio.io/istio/pkg/test"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	extfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/gateway-api/pkg/consts"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func TestDelayedDynamicUnstructuredInformerReportsSyncedWithoutTLSRouteCRD_Issue13661(t *testing.T) {
	stop := test.NewStop(t)
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	client := kube.NewFakeClient()

	inf := newDelayedDynamicUnstructuredInformer(client, wellknown.LegacyTLSRouteGVR, kclient.Filter{})
	inf.Start(stop)

	require.True(t, inf.HasSynced(), "missing legacy TLSRoute CRDs should not block startup")
	require.Empty(t, inf.List(metav1.NamespaceAll, labels.Everything()))
}

func TestDelayedDynamicUnstructuredInformerBypassesCrdWatcherFilterForLegacyTLSRoute_Issue13735(t *testing.T) {
	stop := test.NewStop(t)
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	client := kube.NewFakeClient()
	makeServedCRD(t, client, wellknown.LegacyTLSRouteGVR, "v1.4.1")

	_, err := client.Dynamic().Resource(wellknown.LegacyTLSRouteGVR).Namespace("default").Create(
		context.Background(),
		&unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": wellknown.LegacyTLSRouteGVK.GroupVersion().String(),
				"kind":       wellknown.TLSRouteKind,
				"metadata": map[string]any{
					"name":      "legacy-route",
					"namespace": "default",
				},
				"spec": map[string]any{
					"parentRefs": []any{
						map[string]any{
							"name": "gateway",
						},
					},
					"hostnames": []any{"example.com"},
				},
			},
		},
		metav1.CreateOptions{},
	)
	require.NoError(t, err)

	client.RunAndWait(stop)

	require.False(t, client.CrdWatcher().KnownOrCallback(wellknown.LegacyTLSRouteGVR, func(<-chan struct{}) {}),
		"Gateway API v1.4.x legacy TLSRoute should be filtered from CrdWatcher known state")

	inf := newDelayedDynamicUnstructuredInformer(client, wellknown.LegacyTLSRouteGVR, kclient.Filter{})
	inf.Start(stop)

	require.Eventually(t, inf.HasSynced, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		return len(inf.List("default", labels.Everything())) == 1
	}, time.Second, 10*time.Millisecond, "legacy TLSRoute should still be discoverable through the dynamic informer path")
}

func TestCrdServesVersionWithNilClientIsNonAuthoritative(t *testing.T) {
	served, err := crdServesVersion(nil, wellknown.LegacyTLSRouteGVR)
	require.NoError(t, err)
	require.False(t, served)
}

func makeServedCRD(t *testing.T, client kube.Client, resource schema.GroupVersionResource, bundleVersion string) {
	t.Helper()

	clienttest.MakeCRDWithAnnotations(t, client, resource, map[string]string{
		consts.BundleVersionAnnotation: bundleVersion,
	})

	extClient, ok := client.Ext().(*extfake.Clientset)
	require.True(t, ok)

	err := extClient.Tracker().Add(&apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: resource.Resource + "." + resource.Group,
			Annotations: map[string]string{
				consts.BundleVersionAnnotation: bundleVersion,
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: resource.Group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural: resource.Resource,
				Kind:   wellknown.TLSRouteKind,
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name:    resource.Version,
				Served:  true,
				Storage: true,
			}},
		},
	})
	if apierrors.IsAlreadyExists(err) {
		err = extClient.Tracker().Update(istiogvr.CustomResourceDefinition, &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: resource.Resource + "." + resource.Group,
				Annotations: map[string]string{
					consts.BundleVersionAnnotation: bundleVersion,
				},
			},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Group: resource.Group,
				Names: apiextensionsv1.CustomResourceDefinitionNames{
					Plural: resource.Resource,
					Kind:   wellknown.TLSRouteKind,
				},
				Scope: apiextensionsv1.NamespaceScoped,
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
					Name:    resource.Version,
					Served:  true,
					Storage: true,
				}},
			},
		}, "")
	}
	require.NoError(t, err)
}
