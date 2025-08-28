package metrics_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	. "github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func setupTest() {
	ResetMetrics()
}

func TestCollectionMetricEventHandler(t *testing.T) {
	const (
		testNamespace = "ns"
		testName      = "test"
		testGateway   = "test-gateway"
	)

	testCases := []struct {
		name      string
		namespace string
		parent    string
		resource  string
		inputs    []any
	}{
		{
			name:      "PolicyWrapper",
			namespace: testNamespace,
			parent:    "",
			resource:  "Policy",
			inputs: []any{
				ir.PolicyWrapper{
					ObjectSource: ir.ObjectSource{
						Kind:      "Policy",
						Name:      "test",
						Namespace: testNamespace,
					},
				},
			},
		},
		{
			name:      "HTTPRoute",
			namespace: testNamespace,
			parent:    testGateway,
			resource:  "HTTPRoute",
			inputs: []any{
				&gwv1.HTTPRoute{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNamespace,
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{{
								Name: testGateway,
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
		{
			name:      "TCPRoute",
			namespace: testNamespace,
			parent:    testGateway,
			resource:  "TCPRoute",
			inputs: []any{
				&gwv1a2.TCPRoute{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNamespace,
					},
					Spec: gwv1a2.TCPRouteSpec{
						CommonRouteSpec: gwv1a2.CommonRouteSpec{
							ParentRefs: []gwv1a2.ParentReference{{
								Name: testGateway,
								Kind: ptr.To(gwv1.Kind("Gateway")),
							}},
						},
						Rules: []gwv1a2.TCPRouteRule{{
							Name: ptr.To(gwv1a2.SectionName("test-rule")),
						}},
					},
				},
			},
		},
		{
			name:      "TLSRoute",
			namespace: testNamespace,
			parent:    testGateway,
			resource:  "TLSRoute",
			inputs: []any{
				&gwv1a2.TLSRoute{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNamespace,
					},
					Spec: gwv1a2.TLSRouteSpec{
						CommonRouteSpec: gwv1a2.CommonRouteSpec{
							ParentRefs: []gwv1a2.ParentReference{{
								Name: testGateway,
								Kind: ptr.To(gwv1.Kind("Gateway")),
							}},
						},
						Rules: []gwv1a2.TLSRouteRule{{
							Name: ptr.To(gwv1a2.SectionName("test-rule")),
						}},
					},
				},
			},
		},
		{
			name:      "GRPCRoute",
			namespace: testNamespace,
			parent:    testGateway,
			resource:  "GRPCRoute",
			inputs: []any{
				&gwv1.GRPCRoute{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNamespace,
					},
					Spec: gwv1.GRPCRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{{
								Name: testGateway,
								Kind: ptr.To(gwv1.Kind("Gateway")),
							}},
						},
						Hostnames: []gwv1.Hostname{"example.com"},
						Rules: []gwv1.GRPCRouteRule{{
							Matches: []gwv1.GRPCRouteMatch{{
								Method: &gwv1.GRPCMethodMatch{
									Type: ptr.To(gwv1.GRPCMethodMatchType("exact")),
								},
							}},
						}},
					},
				},
			},
		},
		{
			name:      "Gateway",
			namespace: testNamespace,
			parent:    testGateway,
			resource:  "Gateway",
			inputs: []any{
				&gwv1.Gateway{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      testGateway,
						Namespace: testNamespace,
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "kgateway",
					},
				},
			},
		},
		{
			name:      "XListenerSet",
			namespace: testNamespace,
			parent:    testGateway,
			resource:  "XListenerSet",
			inputs: []any{
				&gwxv1a1.XListenerSet{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNamespace,
						Labels:    map[string]string{"a": "b"},
					},
					Spec: gwxv1a1.ListenerSetSpec{
						ParentRef: gwxv1a1.ParentGatewayReference{
							Name:      testGateway,
							Kind:      ptr.To(gwxv1a1.Kind("Gateway")),
							Namespace: ptr.To(gwxv1a1.Namespace(testNamespace)),
						},
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

			setupMetricEventHandler(tc.inputs[0], mock, done)

			<-done

			gathered := metricstest.MustGatherMetrics(t)

			gathered.AssertMetric("kgateway_resources_managed", &metricstest.ExpectedMetric{
				Labels: []metrics.Label{
					{Name: "namespace", Value: tc.namespace},
					{Name: "parent", Value: tc.parent},
					{Name: "resource", Value: tc.resource},
				},
				Value: 1,
			})
		})
	}
}

func setupMetricEventHandler[T any](_ T, mock *krttest.MockCollection, done chan struct{}) {
	mockPolicyWrappers := krttest.GetMockCollection[T](mock)

	eventHandler := GetResourceMetricEventHandler[T]()

	metrics.RegisterEvents(mockPolicyWrappers, func(o krt.Event[T]) {
		eventHandler(o)

		done <- struct{}{}
	})
}

func TestResourceSync(t *testing.T) {
	setupTest()

	details := ResourceSyncDetails{
		Gateway:      "test-gateway",
		Namespace:    "test-namespace",
		ResourceType: "Gateway",
		ResourceName: "test-gateway",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	StartResourceSyncMetricsProcessing(ctx)

	// Test for resource status sync metrics.
	StartResourceStatusSync(ResourceSyncDetails{
		Gateway:      details.Gateway,
		Namespace:    details.Namespace,
		ResourceType: details.ResourceType,
		ResourceName: details.ResourceName,
	})

	EndResourceStatusSync(details)

	gathered := metricstest.MustGatherMetricsContext(ctx, t,
		"kgateway_resources_status_syncs_started_total",
		"kgateway_resources_status_syncs_completed_total",
		"kgateway_resources_status_sync_duration_seconds",
	)

	gathered.AssertMetric("kgateway_resources_status_syncs_started_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "gateway", Value: details.Gateway},
			{Name: "namespace", Value: details.Namespace},
			{Name: "resource", Value: details.ResourceType},
		},
		Value: 1,
	})

	gathered.AssertMetric("kgateway_resources_status_syncs_completed_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "gateway", Value: details.Gateway},
			{Name: "namespace", Value: details.Namespace},
			{Name: "resource", Value: details.ResourceType},
		},
		Value: 1,
	})

	gathered.AssertMetricsLabels("kgateway_resources_status_sync_duration_seconds", [][]metrics.Label{{
		{Name: "gateway", Value: details.Gateway},
		{Name: "namespace", Value: details.Namespace},
		{Name: "resource", Value: details.ResourceType},
	}})
	gathered.AssertHistogramPopulated("kgateway_resources_status_sync_duration_seconds")

	// Test for resource XDS snapshot sync metrics.
	StartResourceXDSSync(ResourceSyncDetails{
		Gateway:      details.Gateway,
		Namespace:    details.Namespace,
		ResourceType: details.ResourceType,
		ResourceName: details.ResourceName,
	})

	EndResourceXDSSync(details)

	gathered = metricstest.MustGatherMetricsContext(ctx, t,
		"kgateway_xds_snapshot_syncs_total",
		"kgateway_xds_snapshot_sync_duration_seconds",
	)

	gathered.AssertMetric("kgateway_xds_snapshot_syncs_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "gateway", Value: details.Gateway},
			{Name: "namespace", Value: details.Namespace},
		},
		Value: 1,
	})

	gathered.AssertMetricsLabels("kgateway_xds_snapshot_sync_duration_seconds", [][]metrics.Label{{
		{Name: "gateway", Value: details.Gateway},
		{Name: "namespace", Value: details.Namespace},
	}})
	gathered.AssertHistogramPopulated("kgateway_xds_snapshot_sync_duration_seconds")
}

func TestSyncChannelFull(t *testing.T) {
	setupTest()

	details := ResourceSyncDetails{
		Gateway:      "test-gateway",
		Namespace:    "test-namespace",
		ResourceType: "test",
		ResourceName: "test-resource",
	}

	for i := 0; i < 1024; i++ {
		success := EndResourceXDSSync(details)
		assert.True(t, success)
	}

	// Channel will be full. Validate that EndResourceXDSSync returns an error and that the
	// kgateway_resources_updates_dropped_total metric is incremented.
	c := make(chan struct{})
	defer close(c)

	overflowCount := 0
	numOverflows := 20

	for overflowCount < numOverflows {
		success := EndResourceXDSSync(details)
		assert.False(t, success)

		overflowCount++

		currentMetrics := metricstest.MustGatherMetrics(t)
		currentMetrics.AssertMetric("kgateway_resources_updates_dropped_total", &metricstest.ExpectedMetric{
			Labels: []metrics.Label{},
			Value:  float64(overflowCount),
		})
	}
}
