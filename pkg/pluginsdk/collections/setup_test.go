package collections

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func TestListenerSetObjectTransform(t *testing.T) {
	t.Run("stamps the promoted gvk and strips managed fields", func(t *testing.T) {
		ls := &gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{
			Name:          "ls",
			Namespace:     "default",
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubectl"}},
		}}
		out, err := ListenerSetObjectTransform(ls)
		require.NoError(t, err)
		require.Same(t, ls, out, "the transform runs before the object is cached, so it mutates in place")
		require.Equal(t, wellknown.ListenerSetGVK, ls.GroupVersionKind())
		require.Nil(t, ls.ManagedFields)
	})

	t.Run("keeps a gvk that is already set", func(t *testing.T) {
		ls := &gwv1.ListenerSet{}
		ls.SetGroupVersionKind(wellknown.XListenerSetGVK)
		_, err := ListenerSetObjectTransform(ls)
		require.NoError(t, err)
		require.Equal(t, wellknown.XListenerSetGVK, ls.GroupVersionKind())
	})

	t.Run("passes other objects through", func(t *testing.T) {
		tombstone := cache.DeletedFinalStateUnknown{Key: "default/ls", Obj: &gwv1.ListenerSet{}}
		out, err := ListenerSetObjectTransform(tombstone)
		require.NoError(t, err)
		require.Equal(t, tombstone, out)
	})
}

func TestPromotedListenerSetInformerStampsGVK(t *testing.T) {
	stop := test.NewStop(t)
	apiclient.RegisterTypes()

	client := kube.NewFakeClient()
	// Created without TypeMeta, which is how a typed informer decodes it.
	_, err := client.GatewayAPI().GatewayV1().ListenerSets("default").Create(context.Background(),
		&gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default"}},
		metav1.CreateOptions{})
	require.NoError(t, err)

	inf := kclient.NewFiltered[*gwv1.ListenerSet](client, kclient.Filter{ObjectTransform: ListenerSetObjectTransform})
	client.RunAndWait(stop)

	listed := inf.List("default", labels.Everything())
	require.Len(t, listed, 1)
	require.Equal(t, wellknown.ListenerSetGVK, listed[0].GroupVersionKind())
}

func TestConvertLegacyXListenerSetToV1(t *testing.T) {
	t.Run("converts legacy xlistenerset and preserves legacy gvk", func(t *testing.T) {
		converted := convertLegacyXListenerSetToV1(&unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": metav1.GroupVersion{
					Group:   wellknown.XListenerSetGroup,
					Version: wellknown.XListenerSetGVK.Version,
				}.String(),
				"kind": wellknown.XListenerSetKind,
				"metadata": map[string]any{
					"name":      "legacy-listenerset",
					"namespace": "default",
				},
				"spec": map[string]any{
					"parentRef": map[string]any{
						"group":     wellknown.GatewayGroup,
						"kind":      wellknown.GatewayKind,
						"name":      "gateway",
						"namespace": "default",
					},
					"listeners": []any{map[string]any{
						"name":     "listener-8443",
						"port":     int64(8443),
						"protocol": "HTTPS",
					}},
				},
			},
		})
		require.NotNil(t, converted)
		require.Equal(t, wellknown.XListenerSetGVK, converted.GroupVersionKind())
		require.Equal(t, "legacy-listenerset", converted.Name)
		require.Equal(t, "default", converted.Namespace)
		require.Equal(t, gwv1.ObjectName("gateway"), converted.Spec.ParentRef.Name)
		require.Len(t, converted.Spec.Listeners, 1)
		require.Equal(t, gwv1.SectionName("listener-8443"), converted.Spec.Listeners[0].Name)
		require.Equal(t, gwv1.PortNumber(8443), converted.Spec.Listeners[0].Port)
		require.Equal(t, gwv1.HTTPSProtocolType, converted.Spec.Listeners[0].Protocol)
	})

	t.Run("returns nil for invalid legacy payloads", func(t *testing.T) {
		converted := convertLegacyXListenerSetToV1(&unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": metav1.GroupVersion{
					Group:   wellknown.XListenerSetGroup,
					Version: wellknown.XListenerSetGVK.Version,
				}.String(),
				"kind": wellknown.XListenerSetKind,
				"metadata": map[string]any{
					"name":      "broken-listenerset",
					"namespace": "default",
				},
				"spec": map[string]any{
					"listeners": "not-a-list",
				},
			},
		})

		require.Nil(t, converted)
	})
}
