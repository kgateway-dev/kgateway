package collections

import (
	"testing"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/config/schema/gvr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func TestGetServedTCPRouteVersions(t *testing.T) {
	t.Run("returns both versions when both are served", func(t *testing.T) {
		client := apiextensionsfake.NewClientset(&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: wellknown.TCPRouteCRDName},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
					{Name: gwv1a2.GroupVersion.Version, Served: true},
					{Name: gwv1.GroupVersion.Version, Served: true},
				},
			},
		})

		require.Equal(t, servedTCPRouteVersions{
			Promoted:      true,
			PreV1:         true,
			Authoritative: true,
		}, getServedTCPRouteVersions(client))
	})

	t.Run("returns only pre-v1 when promoted v1 is not served", func(t *testing.T) {
		client := apiextensionsfake.NewClientset(&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: wellknown.TCPRouteCRDName},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
					{Name: gwv1a2.GroupVersion.Version, Served: true},
				},
			},
		})

		require.Equal(t, servedTCPRouteVersions{
			PreV1:         true,
			Authoritative: true,
		}, getServedTCPRouteVersions(client))
	})

	t.Run("returns only promoted when v1alpha2 is no longer served", func(t *testing.T) {
		client := apiextensionsfake.NewClientset(&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: wellknown.TCPRouteCRDName},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
					{Name: gwv1a2.GroupVersion.Version, Served: false},
					{Name: gwv1.GroupVersion.Version, Served: true},
				},
			},
		})

		require.Equal(t, servedTCPRouteVersions{
			Promoted:      true,
			Authoritative: true,
		}, getServedTCPRouteVersions(client))
	})

	t.Run("defaults to startup fallback when the CRD is absent", func(t *testing.T) {
		require.Equal(t, servedTCPRouteVersions{
			Promoted: true,
			PreV1:    true,
		}, getServedTCPRouteVersions(apiextensionsfake.NewClientset()))
	})

	t.Run("defaults to startup fallback when discovery is unavailable", func(t *testing.T) {
		require.Equal(t, servedTCPRouteVersions{
			Promoted: true,
			PreV1:    true,
		}, getServedTCPRouteVersions(nil))
	})
}

func TestPreV1TCPRouteWatchGVRs(t *testing.T) {
	t.Run("returns no pre-v1 watches when promoted discovery is authoritative", func(t *testing.T) {
		require.Empty(t, preV1TCPRouteWatchGVRs(servedTCPRouteVersions{
			Promoted:      true,
			PreV1:         true,
			Authoritative: true,
		}))
	})

	t.Run("returns no pre-v1 watches when pre-v1 is not served", func(t *testing.T) {
		require.Empty(t, preV1TCPRouteWatchGVRs(servedTCPRouteVersions{
			Promoted:      true,
			Authoritative: true,
		}))
	})

	t.Run("returns the pre-v1 watch when promoted v1 is not served", func(t *testing.T) {
		require.Equal(t, []schema.GroupVersionResource{gvr.TCPRoute}, preV1TCPRouteWatchGVRs(servedTCPRouteVersions{
			PreV1:         true,
			Authoritative: true,
		}))
	})

	t.Run("returns the pre-v1 fallback when discovery is non-authoritative", func(t *testing.T) {
		require.Equal(t, []schema.GroupVersionResource{gvr.TCPRoute}, preV1TCPRouteWatchGVRs(servedTCPRouteVersions{
			Promoted: true,
			PreV1:    true,
		}))
	})
}

func TestConvertTCPRouteV1ToV1Alpha2(t *testing.T) {
	route := &gwv1.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp-route",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: gwv1.TCPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{
					Name:        "gateway",
					SectionName: new(gwv1.SectionName("listener-8080")),
				}},
			},
			Rules: []gwv1.TCPRouteRule{{
				Name: new(gwv1.SectionName("rule-1")),
				BackendRefs: []gwv1.BackendRef{{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name: "backend",
						Port: new(gwv1.PortNumber(8080)),
					},
				}},
			}},
		},
	}

	converted := convertTCPRouteV1ToV1Alpha2(route)
	require.NotNil(t, converted)
	require.Equal(t, route.Name, converted.Name)
	require.Equal(t, route.Namespace, converted.Namespace)
	require.Equal(t, route.Labels, converted.Labels)
	require.Equal(t, gwv1a2.GroupVersion.String(), converted.APIVersion)
	require.Equal(t, route.Spec.ParentRefs, converted.Spec.ParentRefs)
	require.Len(t, converted.Spec.Rules, 1)
	require.Equal(t, gwv1a2.SectionName("rule-1"), ptr.Deref(converted.Spec.Rules[0].Name, ""))
	require.Len(t, converted.Spec.Rules[0].BackendRefs, 1)
	require.Equal(t, gwv1a2.ObjectName("backend"), converted.Spec.Rules[0].BackendRefs[0].Name)
	require.Equal(t, gwv1a2.PortNumber(8080), ptr.Deref(converted.Spec.Rules[0].BackendRefs[0].Port, 0))
}

func TestConvertTCPRouteV1ToV1Alpha2Nil(t *testing.T) {
	require.Nil(t, convertTCPRouteV1ToV1Alpha2(nil))
}
