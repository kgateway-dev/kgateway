package collections

import (
	"context"
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
		}, getServedTCPRouteVersions(context.Background(), client))
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
		}, getServedTCPRouteVersions(context.Background(), client))
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
		}, getServedTCPRouteVersions(context.Background(), client))
	})

	t.Run("defaults to startup fallback when the CRD is absent", func(t *testing.T) {
		require.Equal(t, servedTCPRouteVersions{
			Promoted: true,
			PreV1:    true,
		}, getServedTCPRouteVersions(context.Background(), apiextensionsfake.NewClientset()))
	})

	t.Run("defaults to startup fallback when discovery is unavailable", func(t *testing.T) {
		require.Equal(t, servedTCPRouteVersions{
			Promoted: true,
			PreV1:    true,
		}, getServedTCPRouteVersions(context.Background(), nil))
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
		Status: gwv1.TCPRouteStatus{
			RouteStatus: gwv1.RouteStatus{
				Parents: []gwv1.RouteParentStatus{{
					ParentRef:      gwv1.ParentReference{Name: "gateway"},
					ControllerName: "test-controller",
					Conditions: []metav1.Condition{{
						Type:   string(gwv1.RouteConditionAccepted),
						Status: metav1.ConditionTrue,
						Reason: string(gwv1.RouteReasonAccepted),
					}},
				}},
			},
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
	require.Equal(t, route.Status.RouteStatus, converted.Status.RouteStatus,
		"status must be preserved: the declarative status writer diffs live status on the converted object")
}

func TestConvertTCPRouteV1ToV1Alpha2Nil(t *testing.T) {
	require.Nil(t, convertTCPRouteV1ToV1Alpha2(nil))
}

func TestTCPRouteWriteGVRs(t *testing.T) {
	testCases := []struct {
		name       string
		versions   servedTCPRouteVersions
		watchPreV1 bool
		want       []schema.GroupVersionResource
	}{
		{
			name:     "promoted v1 served",
			versions: servedTCPRouteVersions{Promoted: true, Authoritative: true},
			want:     []schema.GroupVersionResource{wellknown.TCPRouteV1GVR},
		},
		{
			name:     "both versions served prefers v1",
			versions: servedTCPRouteVersions{Promoted: true, PreV1: true, Authoritative: true},
			want:     []schema.GroupVersionResource{wellknown.TCPRouteV1GVR},
		},
		{
			name:     "only pre-v1 served",
			versions: servedTCPRouteVersions{PreV1: true, Authoritative: true},
			want:     []schema.GroupVersionResource{wellknown.TCPRouteGVR},
		},
		{
			name:     "no served versions falls back to promoted v1",
			versions: servedTCPRouteVersions{Authoritative: true},
			want:     []schema.GroupVersionResource{wellknown.TCPRouteV1GVR},
		},
		{
			// The startup guess must not be permanent: a CRD installed later that only
			// serves v1alpha2 would otherwise get every write sent through the never-served
			// v1 client, which silently drops them.
			name:       "discovery fallback keeps every watched version as a candidate",
			versions:   fallbackTCPRouteVersions(),
			watchPreV1: true,
			want:       []schema.GroupVersionResource{wellknown.TCPRouteV1GVR, wellknown.TCPRouteGVR},
		},
		{
			// Without the experimental watch there is no pre-v1 informer, so a pre-v1
			// candidate could never hold the object.
			name:     "discovery fallback lists only v1 when pre-v1 is not watched",
			versions: fallbackTCPRouteVersions(),
			want:     []schema.GroupVersionResource{wellknown.TCPRouteV1GVR},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tcpRouteWriteGVRs(tc.versions, tc.watchPreV1))
		})
	}
}
