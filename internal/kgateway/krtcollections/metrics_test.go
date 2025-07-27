package krtcollections_test

import (
	"testing"

	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

func setupTest() {
	ResetMetrics()
}

func TestCollectionMetrics(t *testing.T) {
	testCases := []struct {
		name   string
		inputs []any
	}{
		{
			name: "HTTPRoute",
			inputs: []any{
				&gwv1.HTTPRoute{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "ns",
						Labels:    map[string]string{"a": "b"},
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{{
								Name: "test-gateway",
								Kind: ptr.To(gwv1.Kind("Gateway")),
							}},
						},
						Hostnames: []gwv1.Hostname{"example.com"},
						Rules: []gwv1.HTTPRouteRule{{
							Matches: []gwv1.HTTPRouteMatch{{
								Path: &gwv1.HTTPPathMatch{
									Type:  ptr.To(gwv1.PathMatchPathPrefix),
									Value: ptr.To("/"),
								},
							}},
						}},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setupTest()

			done := make(chan struct{})
			mock := krttest.NewMock(t, tc.inputs)
			mockHTTPRoutes := krttest.GetMockCollection[*gwv1.HTTPRoute](mock)

			eventHandler := GetResourceMetricEventHandler[*gwv1.HTTPRoute]()

			metrics.RegisterEvents(mockHTTPRoutes, func(o krt.Event[*gwv1.HTTPRoute]) {
				eventHandler(o)

				done <- struct{}{}
			})

			<-done

			gathered := metricstest.MustGatherMetrics(t)

			gathered.AssertMetric("kgateway_resources_managed", &metricstest.ExpectedMetric{
				Labels: []metrics.Label{
					{Name: "namespace", Value: "ns"},
					{Name: "parent", Value: "test-gateway"},
					{Name: "resource", Value: "HTTPRoute"},
				},
				Value: 1,
			})
		})
	}
}
