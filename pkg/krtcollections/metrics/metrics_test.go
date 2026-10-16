package metrics_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	. "github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/metrics"
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
								Kind: new(gwv1.Kind("Gateway")),
							}},
						},
						Hostnames: []gwv1.Hostname{"example.com"},
						Rules: []gwv1.HTTPRouteRule{{
							Matches: []gwv1.HTTPRouteMatch{{
								Path: &gwv1.HTTPPathMatch{
									Type:  new(gwv1.PathMatchPathPrefix),
									Value: new("/"),
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
								Kind: new(gwv1.Kind("Gateway")),
							}},
						},
						Rules: []gwv1a2.TCPRouteRule{{
							Name: new(gwv1a2.SectionName("test-rule")),
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
								Kind: new(gwv1.Kind("Gateway")),
							}},
						},
						Rules: []gwv1a2.TLSRouteRule{{
							Name: new(gwv1a2.SectionName("test-rule")),
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
								Kind: new(gwv1.Kind("Gateway")),
							}},
						},
						Hostnames: []gwv1.Hostname{"example.com"},
						Rules: []gwv1.GRPCRouteRule{{
							Matches: []gwv1.GRPCRouteMatch{{
								Method: &gwv1.GRPCMethodMatch{
									Type: new(gwv1.GRPCMethodMatchType("exact")),
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
				&gwv1.ListenerSet{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNamespace,
						Labels:    map[string]string{"a": "b"},
					},
					Spec: gwv1.ListenerSetSpec{
						ParentRef: gwv1.ParentGatewayReference{
							Name:      testGateway,
							Kind:      new(gwv1.Kind("Gateway")),
							Namespace: new(gwv1.Namespace(testNamespace)),
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
		Gateway:          "test-gateway",
		GatewayNamespace: "test-namespace",
		Namespace:        "test-namespace",
		ResourceType:     "Gateway",
		ResourceName:     "test-gateway",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	StartResourceSyncMetricsProcessing(ctx)

	// Test for resource status sync metrics.
	StartResourceStatusSync(details)

	EndResourceStatusSync(details)

	gathered := metricstest.MustGatherMetricsContext(ctx, t,
		"kgateway_resources_status_syncs_started_total",
		"kgateway_resources_status_syncs_completed_total",
		"kgateway_resources_status_sync_duration_seconds",
	)

	gathered.AssertMetricsInclude("kgateway_resources_status_syncs_started_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "gateway", Value: details.Gateway},
				{Name: "gateway_namespace", Value: details.GatewayNamespace},
				{Name: "namespace", Value: details.Namespace},
				{Name: "resource", Value: details.ResourceType},
			},
			Value: 1,
		},
	})

	gathered.AssertMetricsInclude("kgateway_resources_status_syncs_completed_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "gateway", Value: details.Gateway},
				{Name: "gateway_namespace", Value: details.GatewayNamespace},
				{Name: "namespace", Value: details.Namespace},
				{Name: "resource", Value: details.ResourceType},
			},
			Value: 1,
		},
	})

	gathered.AssertMetricsLabels("kgateway_resources_status_sync_duration_seconds", [][]metrics.Label{{
		{Name: "gateway", Value: details.Gateway},
		{Name: "gateway_namespace", Value: details.GatewayNamespace},
		{Name: "namespace", Value: details.Namespace},
		{Name: "resource", Value: details.ResourceType},
	}})
	gathered.AssertHistogramPopulated("kgateway_resources_status_sync_duration_seconds")

	// Test for resource XDS snapshot sync metrics.
	StartResourceXDSSync(details)

	EndResourceXDSSync(details)

	gathered = metricstest.MustGatherMetricsContext(ctx, t,
		"kgateway_xds_snapshot_syncs_total",
		"kgateway_xds_snapshot_sync_duration_seconds",
	)

	gathered.AssertMetricsInclude("kgateway_xds_snapshot_syncs_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "gateway", Value: details.Gateway},
				{Name: "namespace", Value: details.Namespace},
			},
			Value: 1,
		},
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

	for range 1024 {
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

func TestResourcesManagedSurvivesSweepOfRecreatedNamespace(t *testing.T) {
	setupTest()

	route := func(name string) *gwv1.HTTPRoute {
		return &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
			Spec: gwv1.HTTPRouteSpec{CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{Name: "gw"}},
			}},
		}
	}
	managed := func(value float64) *metricstest.ExpectedMetric {
		return &metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "namespace", Value: "ns"},
				{Name: "parent", Value: "gw"},
				{Name: "resource", Value: "HTTPRoute"},
			},
			Value: value,
		}
	}
	handler := GetResourceMetricEventHandler[*gwv1.HTTPRoute]()
	oldRoute, newRoute := route("old"), route("new")

	// The namespace is deleted and swept before the delete event of its route is handled.
	handler(krt.Event[*gwv1.HTTPRoute]{Event: controllers.EventAdd, New: &oldRoute})
	metrics.Sweeper{
		NamespaceExists: func(string) bool { return false },
		GatewayExists:   func(string, string) bool { return false },
	}.Sweep()
	metricstest.MustGatherMetrics(t).AssertMetricNotExists("kgateway_resources_managed")

	// The namespace is recreated with a new route, then the old route's delete event arrives.
	handler(krt.Event[*gwv1.HTTPRoute]{Event: controllers.EventAdd, New: &newRoute})
	handler(krt.Event[*gwv1.HTTPRoute]{Event: controllers.EventDelete, Old: &oldRoute})

	metricstest.MustGatherMetrics(t).AssertMetric("kgateway_resources_managed", managed(1))
}

func TestResourcesManagedDeletedAtZero(t *testing.T) {
	setupTest()

	gw := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "ns"}}
	moved := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "ns"},
		Spec: gwv1.HTTPRouteSpec{CommonRouteSpec: gwv1.CommonRouteSpec{
			ParentRefs: []gwv1.ParentReference{{Name: "gw"}},
		}},
	}
	movedTo := moved.DeepCopy()
	movedTo.Spec.ParentRefs[0].Name = "other"
	gwHandler := GetResourceMetricEventHandler[*gwv1.Gateway]()
	routeHandler := GetResourceMetricEventHandler[*gwv1.HTTPRoute]()
	managed := func(parent, resource string) []metrics.Label {
		return []metrics.Label{
			{Name: "namespace", Value: "ns"},
			{Name: "parent", Value: parent},
			{Name: "resource", Value: resource},
		}
	}

	gwHandler(krt.Event[*gwv1.Gateway]{Event: controllers.EventAdd, New: &gw})
	routeHandler(krt.Event[*gwv1.HTTPRoute]{Event: controllers.EventAdd, New: &moved})
	// An update that keeps its parent leaves the series in place.
	routeHandler(krt.Event[*gwv1.HTTPRoute]{Event: controllers.EventUpdate, Old: &moved, New: &moved})
	metricstest.MustGatherMetrics(t).AssertMetricsLabels("kgateway_resources_managed",
		[][]metrics.Label{managed("gw", "Gateway"), managed("gw", "HTTPRoute")})

	// A deleted Gateway and a route moved to another parent leave no zero series in a
	// namespace that stays.
	gwHandler(krt.Event[*gwv1.Gateway]{Event: controllers.EventDelete, Old: &gw})
	routeHandler(krt.Event[*gwv1.HTTPRoute]{Event: controllers.EventUpdate, Old: &moved, New: &movedTo})
	metricstest.MustGatherMetrics(t).AssertMetricsLabels("kgateway_resources_managed",
		[][]metrics.Label{managed("other", "HTTPRoute")})
}

func TestGatewayDeleteDropsPendingXDSSync(t *testing.T) {
	setupTest()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	StartResourceSyncMetricsProcessing(ctx)

	deleted := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "deleted"}}
	live := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "live"}}
	handler := GetResourceMetricEventHandler[*gwv1.Gateway]()
	handler(krt.Event[*gwv1.Gateway]{Event: controllers.EventAdd, New: &deleted})
	handler(krt.Event[*gwv1.Gateway]{Event: controllers.EventAdd, New: &live})
	handler(krt.Event[*gwv1.Gateway]{Event: controllers.EventDelete, Old: &deleted})

	// The next sync of the same-name Gateway ends only its own pending sync.
	EndResourceXDSSync(ResourceSyncDetails{Gateway: "gw", GatewayNamespace: "live", Namespace: "live", ResourceType: "Gateway", ResourceName: "gw"})
	// Only the live Gateway's sync is recorded, not the deleted Gateway's pending one.
	metricstest.MustGatherMetricsContext(ctx, t, "kgateway_xds_snapshot_sync_duration_seconds").
		AssertMetricsLabels("kgateway_xds_snapshot_sync_duration_seconds", [][]metrics.Label{
			{{Name: "gateway", Value: "gw"}, {Name: "namespace", Value: "live"}},
		})
}

func TestSweepRemovesStatusSyncsOfDeletedGateway(t *testing.T) {
	setupTest()

	gateway := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "ns"}}
	route := func(name string, ref gwv1.ParentReference) *gwv1.HTTPRoute {
		return &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "routes"},
			Spec:       gwv1.HTTPRouteSpec{CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{ref}}},
		}
	}
	toGateway := route("to-gateway", gwv1.ParentReference{Name: "gw", Namespace: new(gwv1.Namespace("ns"))})
	toListenerSet := route("to-listenerset", gwv1.ParentReference{Name: "ls", Kind: new(gwv1.Kind("ListenerSet"))})

	GetResourceMetricEventHandler[*gwv1.Gateway]()(krt.Event[*gwv1.Gateway]{Event: controllers.EventAdd, New: &gateway})
	routeHandler := GetResourceMetricEventHandler[*gwv1.HTTPRoute]()
	routeHandler(krt.Event[*gwv1.HTTPRoute]{Event: controllers.EventAdd, New: &toGateway})
	routeHandler(krt.Event[*gwv1.HTTPRoute]{Event: controllers.EventAdd, New: &toListenerSet})

	// The Gateway is gone but every namespace stays. Its own series and those of a route
	// in another namespace attached to it go with it. A route attached to a ListenerSet
	// names no Gateway namespace, so it stays until its own namespace is deleted.
	metrics.Sweeper{
		NamespaceExists: func(string) bool { return true },
		GatewayExists:   func(string, string) bool { return false },
	}.Sweep()

	// ResetMetrics leaves a series with empty labels.
	metricstest.MustGatherMetrics(t).AssertMetricsLabels("kgateway_resources_status_syncs_started_total", [][]metrics.Label{
		{{Name: "gateway", Value: ""}, {Name: "gateway_namespace", Value: ""}, {Name: "namespace", Value: ""}, {Name: "resource", Value: ""}},
		{{Name: "gateway", Value: "ls"}, {Name: "gateway_namespace", Value: ""}, {Name: "namespace", Value: "routes"}, {Name: "resource", Value: "HTTPRoute"}},
	})
}

func TestStatusSyncsOfSameNameGatewaysAreSeparate(t *testing.T) {
	setupTest()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	StartResourceSyncMetricsProcessing(ctx)

	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "routes"},
		Spec: gwv1.HTTPRouteSpec{CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
			{Name: "gw", Namespace: new(gwv1.Namespace("a"))},
			{Name: "gw", Namespace: new(gwv1.Namespace("b"))},
		}}},
	}
	GetResourceMetricEventHandler[*gwv1.HTTPRoute]()(krt.Event[*gwv1.HTTPRoute]{Event: controllers.EventAdd, New: &route})

	// The route has a sync per parent Gateway, and each one ends separately.
	for _, gatewayNamespace := range []string{"a", "b"} {
		EndResourceStatusSync(ResourceSyncDetails{
			Gateway:          "gw",
			GatewayNamespace: gatewayNamespace,
			Namespace:        "routes",
			ResourceType:     "HTTPRoute",
			ResourceName:     "r",
		})
	}
	series := func(gatewayNamespace string) []metrics.Label {
		return []metrics.Label{
			{Name: "gateway", Value: "gw"},
			{Name: "gateway_namespace", Value: gatewayNamespace},
			{Name: "namespace", Value: "routes"},
			{Name: "resource", Value: "HTTPRoute"},
		}
	}
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		metricstest.MustGatherMetrics(c).AssertMetricsLabels("kgateway_resources_status_sync_duration_seconds",
			[][]metrics.Label{series("a"), series("b")})
	}, time.Second, 10*time.Millisecond)

	// Deleting a/gw removes only its series.
	metrics.Sweeper{
		NamespaceExists: func(string) bool { return true },
		GatewayExists:   func(namespace, _ string) bool { return namespace == "b" },
	}.Sweep()
	metricstest.MustGatherMetrics(t).AssertMetricsLabels("kgateway_resources_status_sync_duration_seconds",
		[][]metrics.Label{series("b")})
}
