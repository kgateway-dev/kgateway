package collections

import (
	"testing"

	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func TestGetServedTLSRouteVersions(t *testing.T) {
	t.Run("returns both versions when both are served", func(t *testing.T) {
		client := apiextensionsfake.NewClientset(&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "tlsroutes.gateway.networking.k8s.io"},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
					{Name: gwv1a2.GroupVersion.Version, Served: true},
					{Name: gwv1.GroupVersion.Version, Served: true},
				},
			},
		})

		require.Equal(t, servedTLSRouteVersions{Promoted: true, Legacy: true, Authoritative: true}, getServedTLSRouteVersions(client))
	})

	t.Run("returns only legacy when promoted v1 is not served", func(t *testing.T) {
		client := apiextensionsfake.NewClientset(&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "tlsroutes.gateway.networking.k8s.io"},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
					{Name: gwv1a2.GroupVersion.Version, Served: true},
				},
			},
		})

		require.Equal(t, servedTLSRouteVersions{Legacy: true, Authoritative: true}, getServedTLSRouteVersions(client))
	})

	t.Run("defaults to legacy when discovery is unavailable", func(t *testing.T) {
		require.Equal(t, servedTLSRouteVersions{Promoted: true, Legacy: true}, getServedTLSRouteVersions(nil))
	})
}

func TestConvertTLSRouteV1ToV1Alpha2(t *testing.T) {
	route := &gwv1.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-route",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: gwv1.TLSRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{
					Name:        "gateway",
					SectionName: ptr.To(gwv1.SectionName("listener-443")),
				}},
			},
			Hostnames: []gwv1.Hostname{"example.com"},
			Rules: []gwv1.TLSRouteRule{{
				Name: ptr.To(gwv1.SectionName("rule-1")),
				BackendRefs: []gwv1.BackendRef{{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name: "backend",
						Port: ptr.To(gwv1.PortNumber(443)),
					},
				}},
			}},
		},
	}

	converted := convertTLSRouteV1ToV1Alpha2(route)
	require.NotNil(t, converted)
	require.Equal(t, route.Name, converted.Name)
	require.Equal(t, route.Namespace, converted.Namespace)
	require.Equal(t, route.Labels, converted.Labels)
	require.Equal(t, gwv1a2.GroupVersion.String(), converted.APIVersion)
	require.Equal(t, route.Spec.ParentRefs, converted.Spec.ParentRefs)
	require.Equal(t, []gwv1a2.Hostname{"example.com"}, converted.Spec.Hostnames)
	require.Len(t, converted.Spec.Rules, 1)
	require.Equal(t, gwv1a2.SectionName("rule-1"), ptr.Deref(converted.Spec.Rules[0].Name, ""))
	require.Len(t, converted.Spec.Rules[0].BackendRefs, 1)
	require.Equal(t, gwv1a2.ObjectName("backend"), converted.Spec.Rules[0].BackendRefs[0].Name)
	require.Equal(t, gwv1a2.PortNumber(443), ptr.Deref(converted.Spec.Rules[0].BackendRefs[0].Port, 0))
}

func TestConvertLegacyTLSRouteToV1Alpha2(t *testing.T) {
	route := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": gwv1a2.GroupVersion.String(),
			"kind":       wellknown.TLSRouteKind,
			"metadata": map[string]any{
				"name":      "tls-route",
				"namespace": "default",
				"labels": map[string]any{
					"app": "test",
				},
			},
			"spec": map[string]any{
				"parentRefs": []any{
					map[string]any{
						"name":        "gateway",
						"sectionName": "listener-443",
					},
				},
				"hostnames": []any{"example.com"},
				"rules": []any{
					map[string]any{
						"name": "rule-1",
						"backendRefs": []any{
							map[string]any{
								"name": "backend",
								"port": int64(443),
							},
						},
					},
				},
			},
		},
	}

	converted := convertLegacyTLSRouteToV1Alpha2(route)
	require.NotNil(t, converted)
	require.Equal(t, route.GetName(), converted.Name)
	require.Equal(t, route.GetNamespace(), converted.Namespace)
	require.Equal(t, map[string]string{"app": "test"}, converted.Labels)
	require.Equal(t, gwv1a2.GroupVersion.String(), converted.APIVersion)
	require.Equal(t, []gwv1a2.Hostname{"example.com"}, converted.Spec.Hostnames)
	require.Len(t, converted.Spec.ParentRefs, 1)
	require.Equal(t, gwv1.SectionName("listener-443"), ptr.Deref(converted.Spec.ParentRefs[0].SectionName, ""))
	require.Len(t, converted.Spec.Rules, 1)
	require.Equal(t, gwv1a2.SectionName("rule-1"), ptr.Deref(converted.Spec.Rules[0].Name, ""))
	require.Len(t, converted.Spec.Rules[0].BackendRefs, 1)
	require.Equal(t, gwv1a2.ObjectName("backend"), converted.Spec.Rules[0].BackendRefs[0].Name)
	require.Equal(t, gwv1a2.PortNumber(443), ptr.Deref(converted.Spec.Rules[0].BackendRefs[0].Port, 0))
}
