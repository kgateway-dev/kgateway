package collections

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

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
