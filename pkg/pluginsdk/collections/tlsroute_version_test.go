package collections

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

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
					SectionName: new(gwv1.SectionName("listener-443")),
				}},
			},
			Hostnames: []gwv1.Hostname{"example.com"},
			Rules: []gwv1.TLSRouteRule{{
				Name: new(gwv1.SectionName("rule-1")),
				BackendRefs: []gwv1.BackendRef{{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name: "backend",
						Port: new(gwv1.PortNumber(443)),
					},
				}},
			}},
		},
		Status: gwv1.TLSRouteStatus{
			RouteStatus: gwv1.RouteStatus{
				Parents: []gwv1.RouteParentStatus{{
					ControllerName: "kgateway.dev/kgateway",
					ParentRef: gwv1.ParentReference{
						Name: "gateway",
					},
				}},
			},
		},
	}

	converted := convertTLSRouteV1ToV1Alpha2(route)
	require.NotNil(t, converted)
	require.Equal(t, route.Name, converted.Name)
	require.Equal(t, route.Namespace, converted.Namespace)
	require.Equal(t, route.Labels, converted.Labels)
	require.Equal(t, gwv1.GroupVersion.String(), converted.APIVersion,
		"the served API version must survive normalization: it is the version status is written back through")
	require.Equal(t, route.Spec.ParentRefs, converted.Spec.ParentRefs)
	require.Equal(t, []gwv1a2.Hostname{"example.com"}, converted.Spec.Hostnames)
	require.Equal(t, route.Status.RouteStatus, converted.Status.RouteStatus)
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
					SectionName: new(gwv1.SectionName("listener-443")),
				}},
			},
			Hostnames: []gwv1.Hostname{"example.com"},
			Rules: []gwv1.TLSRouteRule{{
				Name: new(gwv1.SectionName("rule-1")),
				BackendRefs: []gwv1.BackendRef{{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name: "backend",
						Port: new(gwv1.PortNumber(443)),
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
	require.Equal(t, gwv1a3.GroupVersion.String(), converted.APIVersion,
		"the served API version must survive normalization: it is the version status is written back through")
	require.Equal(t, route.Spec.ParentRefs, converted.Spec.ParentRefs)
	require.Equal(t, []gwv1a2.Hostname{"example.com"}, converted.Spec.Hostnames)
	require.Len(t, converted.Spec.Rules, 1)
	require.Equal(t, gwv1a2.SectionName("rule-1"), ptr.Deref(converted.Spec.Rules[0].Name, ""))
	require.Len(t, converted.Spec.Rules[0].BackendRefs, 1)
	require.Equal(t, gwv1a2.ObjectName("backend"), converted.Spec.Rules[0].BackendRefs[0].Name)
	require.Equal(t, gwv1a2.PortNumber(443), ptr.Deref(converted.Spec.Rules[0].BackendRefs[0].Port, 0))
	require.Equal(t, route.Status.RouteStatus, converted.Status.RouteStatus)
}

// The status queue keys a normalized route by the GVK its TypeMeta reports, and dispatches
// the write to the writer registered under that GVK. Normalizing the API version away is
// therefore not cosmetic: it is what previously forced the write path to probe every
// candidate version's informer to work out which client could persist the object.
func TestNormalizedTLSRoutesReportTheServedGroupVersionKind(t *testing.T) {
	fromV1 := convertTLSRouteV1ToV1Alpha2(&gwv1.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
	})
	require.Equal(t, wellknown.TLSRouteV1GVK, fromV1.GetObjectKind().GroupVersionKind())

	fromV1Alpha3 := convertTLSRouteV1Alpha3ToV1Alpha2(&gwv1a3.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
	})
	require.Equal(t, wellknown.TLSRouteV1Alpha3GVK, fromV1Alpha3.GetObjectKind().GroupVersionKind())

	// A v1alpha2 route is served, not converted, so it arrives with empty TypeMeta and the
	// registration's fallback GVK has to name v1alpha2 for the dispatch to land.
	require.Equal(t, gwv1a2.GroupVersion.Version, wellknown.TLSRouteGVK.Version)
}
