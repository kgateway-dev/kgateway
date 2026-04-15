package collections

import (
	"testing"

	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func TestGetServedTLSRouteVersions(t *testing.T) {
	t.Run("returns both versions when both are served", func(t *testing.T) {
		client := apiextensionsfake.NewClientset(&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "tlsroutes.gateway.networking.k8s.io"},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
					{Name: wellknown.TLSRouteV1Alpha3Version, Served: true},
					{Name: gwv1.GroupVersion.Version, Served: true},
				},
			},
		})

		require.Equal(t, servedTLSRouteVersions{
			Promoted:          true,
			PreV1:             true,
			PreferredPreV1GVR: wellknown.TLSRouteV1Alpha3GVR,
			Authoritative:     true,
		}, getServedTLSRouteVersions(client))
	})

	t.Run("returns only pre-v1 when promoted v1 is not served", func(t *testing.T) {
		client := apiextensionsfake.NewClientset(&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "tlsroutes.gateway.networking.k8s.io"},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
					{Name: wellknown.TLSRouteV1Alpha3Version, Served: true},
				},
			},
		})

		require.Equal(t, servedTLSRouteVersions{
			PreV1:             true,
			PreferredPreV1GVR: wellknown.TLSRouteV1Alpha3GVR,
			Authoritative:     true,
		}, getServedTLSRouteVersions(client))
	})

	t.Run("prefers v1alpha3 when gateway api v1.4.1 serves both pre-v1 versions", func(t *testing.T) {
		client := apiextensionsfake.NewClientset(&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "tlsroutes.gateway.networking.k8s.io"},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
					{Name: gwv1a2.GroupVersion.Version, Served: true, Storage: false},
					{Name: wellknown.TLSRouteV1Alpha3Version, Served: true, Storage: true},
				},
			},
		})

		require.Equal(t, servedTLSRouteVersions{
			PreV1:             true,
			PreferredPreV1GVR: wellknown.TLSRouteV1Alpha3GVR,
			Authoritative:     true,
		}, getServedTLSRouteVersions(client))
	})

	t.Run("defaults to startup fallback when the CRD is absent", func(t *testing.T) {
		require.Equal(t, servedTLSRouteVersions{
			Promoted:          true,
			PreV1:             true,
			PreferredPreV1GVR: wellknown.TLSRouteV1Alpha3GVR,
		}, getServedTLSRouteVersions(apiextensionsfake.NewClientset()))
	})

	t.Run("defaults to pre-v1 when discovery is unavailable", func(t *testing.T) {
		require.Equal(t, servedTLSRouteVersions{
			Promoted:          true,
			PreV1:             true,
			PreferredPreV1GVR: wellknown.TLSRouteV1Alpha3GVR,
		}, getServedTLSRouteVersions(nil))
	})

	t.Run("falls back to v1alpha2 when that is the only served pre-v1 version", func(t *testing.T) {
		client := apiextensionsfake.NewClientset(&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "tlsroutes.gateway.networking.k8s.io"},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
					{Name: gwv1a2.GroupVersion.Version, Served: true},
				},
			},
		})

		require.Equal(t, servedTLSRouteVersions{
			PreV1:             true,
			PreferredPreV1GVR: schema.GroupVersionResource{Group: wellknown.GatewayGroup, Version: gwv1a2.GroupVersion.Version, Resource: "tlsroutes"},
			Authoritative:     true,
		}, getServedTLSRouteVersions(client))
	})
}

func TestPreV1TLSRouteWatchGVRs(t *testing.T) {
	t.Run("returns no pre-v1 watches when promoted discovery is authoritative", func(t *testing.T) {
		require.Empty(t, preV1TLSRouteWatchGVRs(servedTLSRouteVersions{
			Promoted:          true,
			PreV1:             true,
			PreferredPreV1GVR: wellknown.TLSRouteV1Alpha3GVR,
			Authoritative:     true,
		}))
	})

	t.Run("returns the discovered pre-v1 watch when promoted v1 is not served", func(t *testing.T) {
		require.Equal(t, []schema.GroupVersionResource{tlsRouteV1Alpha2GVR}, preV1TLSRouteWatchGVRs(servedTLSRouteVersions{
			PreV1:             true,
			PreferredPreV1GVR: tlsRouteV1Alpha2GVR,
			Authoritative:     true,
		}))
	})

	t.Run("returns only v1alpha3 when both pre-v1 versions are served authoritatively", func(t *testing.T) {
		require.Equal(t, []schema.GroupVersionResource{wellknown.TLSRouteV1Alpha3GVR}, preV1TLSRouteWatchGVRs(servedTLSRouteVersions{
			PreV1:             true,
			PreferredPreV1GVR: wellknown.TLSRouteV1Alpha3GVR,
			Authoritative:     true,
		}))
	})

	t.Run("returns both pre-v1 fallbacks when discovery is non-authoritative", func(t *testing.T) {
		require.Equal(t, []schema.GroupVersionResource{wellknown.TLSRouteV1Alpha3GVR, tlsRouteV1Alpha2GVR}, preV1TLSRouteWatchGVRs(servedTLSRouteVersions{
			Promoted:          true,
			PreV1:             true,
			PreferredPreV1GVR: wellknown.TLSRouteV1Alpha3GVR,
		}))
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

func TestConvertTLSRouteV1Alpha3ToV1Alpha2(t *testing.T) {
	route := &gwv1a3.TLSRoute{
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
		Status: gwv1.TLSRouteStatus{
			RouteStatus: gwv1.RouteStatus{
				Parents: []gwv1.RouteParentStatus{{
					ParentRef: gwv1.ParentReference{Name: "gateway"},
				}},
			},
		},
	}

	converted := convertTLSRouteV1Alpha3ToV1Alpha2(route)
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
	require.Equal(t, route.Status.RouteStatus, converted.Status.RouteStatus)
}

func TestConvertUnstructuredTLSRouteToV1Alpha2(t *testing.T) {
	route := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": wellknown.TLSRouteV1Alpha3GVK.GroupVersion().String(),
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

	converted := convertUnstructuredTLSRouteToV1Alpha2(route)
	require.NotNil(t, converted)
	require.Equal(t, route.GetName(), converted.Name)
	require.Equal(t, route.GetNamespace(), converted.Namespace)
	require.Equal(t, map[string]string{"app": "test"}, converted.Labels)
	require.Equal(t, gwv1a2.GroupVersion.String(), converted.APIVersion)
	require.Equal(t, wellknown.TLSRouteGVK, converted.GroupVersionKind())
	require.Equal(t, []gwv1a2.Hostname{"example.com"}, converted.Spec.Hostnames)
	require.Len(t, converted.Spec.ParentRefs, 1)
	require.Equal(t, gwv1.SectionName("listener-443"), ptr.Deref(converted.Spec.ParentRefs[0].SectionName, ""))
	require.Len(t, converted.Spec.Rules, 1)
	require.Equal(t, gwv1a2.SectionName("rule-1"), ptr.Deref(converted.Spec.Rules[0].Name, ""))
	require.Len(t, converted.Spec.Rules[0].BackendRefs, 1)
	require.Equal(t, gwv1a2.ObjectName("backend"), converted.Spec.Rules[0].BackendRefs[0].Name)
	require.Equal(t, gwv1a2.PortNumber(443), ptr.Deref(converted.Spec.Rules[0].BackendRefs[0].Port, 0))
}
