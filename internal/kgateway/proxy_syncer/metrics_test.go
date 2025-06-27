package proxy_syncer

import (
	"testing"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

func setupTest() {
	ResetMetrics()
}

func TestNewStatusSyncRecorder(t *testing.T) {
	setupTest()

	syncerName := "test-syncer"
	m := NewStatusSyncMetricsRecorder(syncerName)

	finishFunc := m.StatusSyncStart()
	finishFunc(nil)
	m.SetResources(StatusSyncResourcesMetricLabels{Namespace: "default", Name: "test", Resource: "route"}, 5)

	expectedMetrics := []string{
		"kgateway_status_syncer_status_syncs_total",
		"kgateway_status_syncer_status_sync_duration_seconds",
		"kgateway_status_syncer_resources",
	}

	currentMetrics := metricstest.MustGatherMetrics(t)

	for _, expected := range expectedMetrics {
		currentMetrics.AssertMetricExists(expected)
	}
}

func TestStatusSyncStart_Success(t *testing.T) {
	setupTest()

	m := NewStatusSyncMetricsRecorder("test-syncer")

	finishFunc := m.StatusSyncStart()
	time.Sleep(10 * time.Millisecond)
	finishFunc(nil)

	currentMetrics := metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetric("kgateway_status_syncer_status_syncs_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "result", Value: "success"},
			{Name: "syncer", Value: "test-syncer"},
		},
		Value: 1,
	})

	currentMetrics.AssertMetricLabels("kgateway_status_syncer_status_sync_duration_seconds", []metrics.Label{
		{Name: "syncer", Value: "test-syncer"},
	})
	currentMetrics.AssertHistogramPopulated("kgateway_status_syncer_status_sync_duration_seconds")
}

func TesStatusSyncStart_Error(t *testing.T) {
	setupTest()

	m := NewStatusSyncMetricsRecorder("test-syncer")

	finishFunc := m.StatusSyncStart()
	finishFunc(assert.AnError)

	currentMetrics := metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetric("kgateway_status_syncer_status_syncs_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "result", Value: "error"},
			{Name: "syncer", Value: "test-syncer"},
		},
		Value: 1,
	})
	currentMetrics.AssertMetricNotExists("kgateway_status_syncer_status_sync_duration_seconds")
}

func TestStatusSyncResources(t *testing.T) {
	setupTest()

	m := NewStatusSyncMetricsRecorder("test-statusSync")

	// Test SetResources.
	m.SetResources(StatusSyncResourcesMetricLabels{Namespace: "default", Name: "test", Resource: "route"}, 5)
	m.SetResources(StatusSyncResourcesMetricLabels{Namespace: "kube-system", Name: "test", Resource: "gateway"}, 3)

	expectedRouteLabels := []metrics.Label{
		{Name: "name", Value: "test"},
		{Name: "namespace", Value: "default"},
		{Name: "resource", Value: "route"},
		{Name: "syncer", Value: "test-statusSync"},
	}
	expectedGatewayLabels := []metrics.Label{
		{Name: "name", Value: "test"},
		{Name: "namespace", Value: "kube-system"},
		{Name: "resource", Value: "gateway"},
		{Name: "syncer", Value: "test-statusSync"},
	}

	currentMetrics := metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetrics("kgateway_status_syncer_resources", []metricstest.ExpectMetric{
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
	m.IncResources(StatusSyncResourcesMetricLabels{Namespace: "default", Name: "test", Resource: "route"})

	currentMetrics = metricstest.MustGatherMetrics(t)
	currentMetrics.AssertMetrics("kgateway_status_syncer_resources", []metricstest.ExpectMetric{
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
	m.DecResources(StatusSyncResourcesMetricLabels{Namespace: "default", Name: "test", Resource: "route"})

	currentMetrics = metricstest.MustGatherMetrics(t)
	currentMetrics.AssertMetrics("kgateway_status_syncer_resources", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: expectedRouteLabels,
			Value:  5,
		},
		&metricstest.ExpectedMetric{
			Labels: expectedGatewayLabels,
			Value:  3,
		},
	})

	// Test ResetResources.
	m.ResetResources("route")

	currentMetrics = metricstest.MustGatherMetrics(t)
	currentMetrics.AssertMetrics("kgateway_status_syncer_resources", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: expectedRouteLabels,
			Value:  0,
		},
		&metricstest.ExpectedMetric{
			Labels: expectedGatewayLabels,
			Value:  3,
		},
	})
}

func TestStatusSyncMetricsNotActive(t *testing.T) {
	metrics.SetActive(false)
	defer metrics.SetActive(true)

	m := NewStatusSyncMetricsRecorder("test-syncer")

	finishFunc := m.StatusSyncStart()
	time.Sleep(10 * time.Millisecond)
	finishFunc(nil)

	currentMetrics := metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetricNotExists("kgateway_status_syncer_status_syncs_total")
	currentMetrics.AssertMetricNotExists("kgateway_status_syncer_status_sync_duration_seconds")
}

func TestXDSSnapshotsCollectionMetrics(t *testing.T) {
	testCases := []struct {
		name   string
		inputs []any
	}{
		{
			name: "NewProxySyncer",
			inputs: []any{
				ir.NewUniqlyConnectedClient(
					"kgateway-kube-gateway-api~ns~test",
					"ns",
					map[string]string{"a": "b"},
					ir.PodLocality{
						Zone:    "zone1",
						Region:  "region1",
						Subzone: "subzone1",
					}),
				GatewayXdsResources{
					NamespacedName: types.NamespacedName{
						Name:      "test",
						Namespace: "ns",
					},
					Routes: envoycache.Resources{
						Version: "v1",
						Items: map[string]envoycachetypes.ResourceWithTTL{
							"test-route": {TTL: ptr.To(time.Minute)},
						},
					},
					Listeners: envoycache.Resources{
						Version: "v1",
						Items: map[string]envoycachetypes.ResourceWithTTL{
							"test-listener": {TTL: ptr.To(time.Minute)},
						},
					},
					Clusters: []envoycachetypes.ResourceWithTTL{{
						Resource: &clusterv3.Cluster{
							Name: "test",
							TransportSocketMatches: []*clusterv3.Cluster_TransportSocketMatch{{
								Name: "test",
							}},
						},
						TTL: ptr.To(time.Minute),
					}},
				},
				UccWithEndpoints{
					Client: ir.NewUniqlyConnectedClient(
						"kgateway-kube-gateway-api~ns~test",
						"ns",
						map[string]string{"a": "b"},
						ir.PodLocality{
							Zone:    "zone1",
							Region:  "region1",
							Subzone: "subzone1",
						}),
					Endpoints: &endpointv3.ClusterLoadAssignment{
						ClusterName: "test",
						Endpoints: []*endpointv3.LocalityLbEndpoints{
							{
								Locality: &corev3.Locality{
									Region:  "region1",
									Zone:    "zone1",
									SubZone: "subzone1",
								},
								LbEndpoints: []*endpointv3.LbEndpoint{{
									HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
										Endpoint: &endpointv3.Endpoint{
											Address: &corev3.Address{
												Address: &corev3.Address_SocketAddress{
													SocketAddress: &corev3.SocketAddress{
														Address: "",
														PortSpecifier: &corev3.SocketAddress_PortValue{
															PortValue: 8080,
														},
													},
												},
											},
										},
									},
								}},
							},
						},
					},
				},
				uccWithCluster{
					Client: ir.NewUniqlyConnectedClient(
						"kgateway-kube-gateway-api~ns~test",
						"ns",
						map[string]string{"a": "b"},
						ir.PodLocality{
							Zone:    "zone1",
							Region:  "region1",
							Subzone: "subzone1",
						}),
					Cluster: &clusterv3.Cluster{
						TransportSocketMatches: []*clusterv3.Cluster_TransportSocketMatch{{
							Name: "test",
						}},
						Name: "test",
					},
					Name: "test",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ResetMetrics()

			mock := krttest.NewMock(t, tc.inputs)
			mockUcc := krttest.GetMockCollection[ir.UniqlyConnectedClient](mock)
			mockGatewayXDSResorces := krttest.GetMockCollection[GatewayXdsResources](mock)
			mockUccWithEndpoints := krttest.GetMockCollection[UccWithEndpoints](mock)
			mockUccWithCluster := krttest.GetMockCollection[uccWithCluster](mock)

			c := snapshotPerClient(krtutil.KrtOptions{}, mockUcc, mockGatewayXDSResorces,
				PerClientEnvoyEndpoints{
					endpoints: mockUccWithEndpoints,
					index: krt.NewIndex(mockUccWithEndpoints, func(ucc UccWithEndpoints) []string {
						return []string{ucc.Client.ResourceName()}
					}),
				},
				PerClientEnvoyClusters{
					clusters: mockUccWithCluster,
					index: krt.NewIndex(mockUccWithCluster, func(ucc uccWithCluster) []string {
						return []string{ucc.Client.ResourceName()}
					}),
				},
				krtcollections.NewCollectionMetricsRecorder("ClientXDSSnapshots"))

			c.WaitUntilSynced(nil)
			time.Sleep(5 * time.Millisecond) // Allow some time for events to process.

			gathered := metricstest.MustGatherMetrics(t)

			gathered.AssertMetric("kgateway_collection_transforms_total", &metricstest.ExpectedMetric{
				Labels: []metrics.Label{
					{Name: "collection", Value: "ClientXDSSnapshots"},
					{Name: "result", Value: "success"},
				},
				Value: 1,
			})

			gathered.AssertMetricsLabels("kgateway_collection_transform_duration_seconds", [][]metrics.Label{{
				{Name: "collection", Value: "ClientXDSSnapshots"},
			}})

			gathered.AssertMetrics("kgateway_collection_resources", []metricstest.ExpectMetric{
				&metricstest.ExpectedMetric{
					Labels: []metrics.Label{
						{Name: "collection", Value: "ClientXDSSnapshots"},
						{Name: "name", Value: "test"},
						{Name: "namespace", Value: "ns"},
						{Name: "resource", Value: "Cluster"},
					},
					Value: 1,
				},
				&metricstest.ExpectedMetric{
					Labels: []metrics.Label{
						{Name: "collection", Value: "ClientXDSSnapshots"},
						{Name: "name", Value: "test"},
						{Name: "namespace", Value: "ns"},
						{Name: "resource", Value: "Endpoint"},
					},
					Value: 1,
				},
				&metricstest.ExpectedMetric{
					Labels: []metrics.Label{
						{Name: "collection", Value: "ClientXDSSnapshots"},
						{Name: "name", Value: "test"},
						{Name: "namespace", Value: "ns"},
						{Name: "resource", Value: "Listener"},
					},
					Value: 1,
				},
				&metricstest.ExpectedMetric{
					Labels: []metrics.Label{
						{Name: "collection", Value: "ClientXDSSnapshots"},
						{Name: "name", Value: "test"},
						{Name: "namespace", Value: "ns"},
						{Name: "resource", Value: "Route"},
					},
					Value: 1,
				},
			})
		})
	}
}
