package krtcollections_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	. "github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
)

func setupTest() {
	ResetMetrics()
}

func TestNewCollectionRecorder(t *testing.T) {
	setupTest()

	collectionName := "test-collection"
	m := NewCollectionMetricsRecorder(collectionName)

	finishFunc := m.TransformStart()
	finishFunc(nil)
	m.SetResources(CollectionResourcesMetricLabels{Namespace: "default", Name: "test", Resource: "route"}, 5)

	expectedMetrics := []string{
		"kgateway_collection_transforms_total",
		"kgateway_collection_transform_duration_seconds",
		"kgateway_collection_resources",
	}

	currentMetrics := metricstest.MustGatherMetrics(t)
	for _, expected := range expectedMetrics {
		currentMetrics.AssertMetricExists(expected)
	}
}

func TestTransformStart_Success(t *testing.T) {
	setupTest()

	m := NewCollectionMetricsRecorder("test-collection")

	finishFunc := m.TransformStart()

	currentMetrics := metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetric("kgateway_collection_transforms_running", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "collection", Value: "test-collection"},
		},
		Value: 1,
	})

	finishFunc(nil)

	currentMetrics = metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetric("kgateway_collection_transforms_running", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "collection", Value: "test-collection"},
		},
		Value: 0,
	})

	currentMetrics.AssertMetric("kgateway_collection_transforms_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "result", Value: "success"},
			{Name: "collection", Value: "test-collection"},
		},
		Value: 1,
	})

	currentMetrics.AssertMetricLabels("kgateway_collection_transform_duration_seconds", []metrics.Label{
		{Name: "collection", Value: "test-collection"},
	})
	currentMetrics.AssertHistogramPopulated("kgateway_collection_transform_duration_seconds")
}

func TestTransformStart_Error(t *testing.T) {
	setupTest()

	m := NewCollectionMetricsRecorder("test-collection")

	finishFunc := m.TransformStart()

	currentMetrics := metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetric("kgateway_collection_transforms_running", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "collection", Value: "test-collection"},
		},
		Value: 1,
	})

	finishFunc(assert.AnError)

	currentMetrics = metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetric("kgateway_collection_transforms_running", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "collection", Value: "test-collection"},
		},
		Value: 0,
	})

	currentMetrics.AssertMetric("kgateway_collection_transforms_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "collection", Value: "test-collection"},
			{Name: "result", Value: "error"},
		},
		Value: 1,
	})

	currentMetrics.AssertMetricLabels("kgateway_collection_transform_duration_seconds", []metrics.Label{
		{Name: "collection", Value: "test-collection"},
	})
	currentMetrics.AssertHistogramPopulated("kgateway_collection_transform_duration_seconds")
}

func TestCollectionResources(t *testing.T) {
	setupTest()

	m := NewCollectionMetricsRecorder("test-collection")

	// Test SetResources.
	m.SetResources(CollectionResourcesMetricLabels{Namespace: "default", Name: "test", Resource: "route"}, 5)
	m.SetResources(CollectionResourcesMetricLabels{Namespace: "kube-system", Name: "test", Resource: "gateway"}, 3)

	expectedGatewayLabels := []metrics.Label{
		{Name: "collection", Value: "test-collection"},
		{Name: "name", Value: "test"},
		{Name: "namespace", Value: "kube-system"},
		{Name: "resource", Value: "gateway"},
	}

	expectedRouteLabels := []metrics.Label{
		{Name: "collection", Value: "test-collection"},
		{Name: "name", Value: "test"},
		{Name: "namespace", Value: "default"},
		{Name: "resource", Value: "route"},
	}
	currentMetrics := metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetrics("kgateway_collection_resources", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: expectedRouteLabels,
			Value:  5,
		},
		&metricstest.ExpectedMetric{
			Labels: expectedGatewayLabels,
			Value:  3,
		},
	})

	// Test IncResources.
	m.IncResources(CollectionResourcesMetricLabels{Namespace: "default", Name: "test", Resource: "route"})

	currentMetrics = metricstest.MustGatherMetrics(t)
	currentMetrics.AssertMetrics("kgateway_collection_resources", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: expectedRouteLabels,
			Value:  6,
		},
		&metricstest.ExpectedMetric{
			Labels: expectedGatewayLabels,
			Value:  3,
		},
	})

	// Test DecResources.
	m.DecResources(CollectionResourcesMetricLabels{Namespace: "default", Name: "test", Resource: "route"})

	currentMetrics = metricstest.MustGatherMetrics(t)
	currentMetrics.AssertMetrics("kgateway_collection_resources", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: expectedRouteLabels,
			Value:  5,
		},
		&metricstest.ExpectedMetric{
			Labels: expectedGatewayLabels,
			Value:  3,
		},
	})
}

func TestTransformStartNotActive(t *testing.T) {
	metrics.SetActive(false)
	defer metrics.SetActive(true)

	setupTest()

	m := NewCollectionMetricsRecorder("test-collection")

	finishFunc := m.TransformStart()
	time.Sleep(10 * time.Millisecond)
	finishFunc(nil)

	currentMetrics := metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetricNotExists("kgateway_collection_transforms_total")
	currentMetrics.AssertMetricNotExists("kgateway_collection_transform_duration_seconds")
	currentMetrics.AssertMetricNotExists("kgateway_collection_transforms_running")
	currentMetrics.AssertMetricNotExists("kgateway_collection_resources")
}

func TestGatewaysCollectionMetrics(t *testing.T) {
	testCases := []struct {
		name   string
		inputs []any
	}{
		{
			name: "NewGatewayIndex",
			inputs: []any{
				NamespaceMetadata{
					Name: "ns",
				},
				&gwv1.GatewayClass{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "",
					},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: "test",
					},
				},
				&gwv1.Gateway{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gateway",
						Namespace: "ns",
						Labels:    map[string]string{"a": "b"},
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "test",
						Listeners: []gwv1.Listener{{
							Name:     "test-listener",
							Port:     80,
							Protocol: gwv1.HTTPProtocolType,
						}},
					},
				},
				&gwxv1a1.XListenerSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "ns",
					},
					Spec: gwxv1a1.ListenerSetSpec{
						Listeners: []gwxv1a1.ListenerEntry{{
							Name: "test-listener",
							AllowedRoutes: &gwxv1a1.AllowedRoutes{
								Namespaces: &gwv1.RouteNamespaces{
									From: ptr.To(gwv1.NamespacesFromSame),
								},
							},
						}},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ResetMetrics()

			mock := krttest.NewMock(t, tc.inputs)
			mockNs := krttest.GetMockCollection[NamespaceMetadata](mock)
			mockGwcs := krttest.GetMockCollection[*gwv1.GatewayClass](mock)
			mockGws := krttest.GetMockCollection[*gwv1.Gateway](mock)
			mockLss := krttest.GetMockCollection[*gwxv1a1.XListenerSet](mock)

			idx := NewGatewayIndex(krtutil.KrtOptions{}, "test",
				NewPolicyIndex(krtutil.KrtOptions{}, nil, settings.Settings{}), mockGws, mockLss, mockGwcs, mockNs)
			idx.Gateways.WaitUntilSynced(context.Background().Done())

			time.Sleep(5 * time.Millisecond) // Allow some time for events to process.

			gathered := metricstest.MustGatherMetrics(t)

			gathered.AssertMetric("kgateway_collection_transforms_total", &metricstest.ExpectedMetric{
				Labels: []metrics.Label{
					{Name: "collection", Value: "Gateways"},
					{Name: "result", Value: "success"},
				},
				Value: 1,
			})

			gathered.AssertMetric("kgateway_collection_transforms_running", &metricstest.ExpectedMetric{
				Labels: []metrics.Label{
					{Name: "collection", Value: "Gateways"},
				},
				Value: 0,
			})

			gathered.AssertMetrics("kgateway_collection_resources", []metricstest.ExpectMetric{
				&metricstest.ExpectedMetric{
					Labels: []metrics.Label{
						{Name: "collection", Value: "Gateways"},
						{Name: "name", Value: "test-gateway"},
						{Name: "namespace", Value: "ns"},
						{Name: "resource", Value: "Gateway"},
					},
					Value: 1,
				},
				&metricstest.ExpectedMetric{
					Labels: []metrics.Label{
						{Name: "collection", Value: "Gateways"},
						{Name: "name", Value: "test-gateway"},
						{Name: "namespace", Value: "ns"},
						{Name: "resource", Value: "Listeners"},
					},
					Value: 1,
				},
			})
		})
	}
}

func TestK8SEndpointsCollectionMetrics(t *testing.T) {
	testCases := []struct {
		name   string
		inputs []any
	}{
		{
			name: "NewK8sEndpoints",
			inputs: []any{
				ir.BackendObjectIR{
					ObjectSource: ir.ObjectSource{
						Kind:      "Service",
						Namespace: "ns",
						Name:      "test",
					},
					Port:              80,
					CanonicalHostname: "test.ns.svc.cluster.local",
					Obj: &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test",
							Namespace: "ns",
						},
						Spec: corev1.ServiceSpec{
							Ports: []corev1.ServicePort{{
								Port: 80,
							}},
						},
					},
				},
				&discoveryv1.EndpointSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "ns",
					},
					Endpoints: []discoveryv1.Endpoint{{
						Addresses: []string{"test"},
					}},
					Ports: []discoveryv1.EndpointPort{{
						Port: ptr.To[int32](80),
					}},
				},
				LocalityPod{
					Named: krt.Named{
						Name:      "test",
						Namespace: "ns",
					},
					Locality: ir.PodLocality{
						Zone:    "zone1",
						Region:  "region1",
						Subzone: "subzone1",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ResetMetrics()

			mock := krttest.NewMock(t, tc.inputs)
			mockBackends := krttest.GetMockCollection[ir.BackendObjectIR](mock)
			mockEndpointSlices := krttest.GetMockCollection[*discoveryv1.EndpointSlice](mock)
			mockPods := krttest.GetMockCollection[LocalityPod](mock)

			c := NewK8sEndpoints(context.Background(), EndpointsInputs{
				Backends:       mockBackends,
				EndpointSlices: mockEndpointSlices,
				EndpointSlicesByService: krt.NewIndex(mockEndpointSlices, func(e *discoveryv1.EndpointSlice) []types.NamespacedName {
					return []types.NamespacedName{{
						Name:      e.Name,
						Namespace: e.Namespace,
					}}
				}),
				Pods:    mockPods,
				KrtOpts: krtutil.KrtOptions{},
				EndpointsSettings: EndpointsSettings{
					EnableAutoMtls: true,
				},
			})

			c.WaitUntilSynced(context.Background().Done())

			time.Sleep(5 * time.Millisecond) // Allow some time for events to process.

			gathered := metricstest.MustGatherMetrics(t)

			gathered.AssertMetric("kgateway_collection_transforms_total", &metricstest.ExpectedMetric{
				Labels: []metrics.Label{
					{Name: "collection", Value: "K8sEndpoints"},
					{Name: "result", Value: "success"},
				},
				Value: 1,
			})

			gathered.AssertMetric("kgateway_collection_transforms_running", &metricstest.ExpectedMetric{
				Labels: []metrics.Label{
					{Name: "collection", Value: "K8sEndpoints"},
				},
				Value: 0,
			})

			gathered.AssertMetricsLabels("kgateway_collection_transform_duration_seconds", [][]metrics.Label{{
				{Name: "collection", Value: "K8sEndpoints"},
			}})

			gathered.AssertMetric("kgateway_collection_resources", &metricstest.ExpectedMetric{
				Labels: []metrics.Label{
					{Name: "collection", Value: "K8sEndpoints"},
					{Name: "name", Value: "test"},
					{Name: "namespace", Value: "ns"},
					{Name: "resource", Value: "Endpoints"},
				},
				Value: 1,
			})
		})
	}
}
