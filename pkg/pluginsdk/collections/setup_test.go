package collections

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

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
