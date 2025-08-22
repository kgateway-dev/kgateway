package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt/krttest"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	tmetrics "github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/metrics"
	krtinternal "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

const (
	testSyncerName  = "test-syncer"
	testGatewayName = "test-gateway"
	testNamespace   = "test-namespace"
)

func setupTest() {
	ResetMetrics()
	tmetrics.ResetMetrics()
}

func TestCollectStatusSyncMetrics_Success(t *testing.T) {
	setupTest()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	finishFunc := collectStatusSyncMetrics(statusSyncMetricLabels{
		Name:      testGatewayName,
		Namespace: testNamespace,
		Syncer:    testSyncerName,
	})
	finishFunc(nil)

	currentMetrics := metricstest.MustGatherMetricsContext(ctx, t,
		"kgateway_status_syncer_status_syncs_total",
		"kgateway_status_syncer_status_sync_duration_seconds",
	)

	currentMetrics.AssertMetric("kgateway_status_syncer_status_syncs_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "name", Value: testGatewayName},
			{Name: "namespace", Value: testNamespace},
			{Name: "result", Value: "success"},
			{Name: "syncer", Value: "test-syncer"},
		},
		Value: 1,
	})

	currentMetrics.AssertMetricLabels("kgateway_status_syncer_status_sync_duration_seconds", []metrics.Label{
		{Name: "name", Value: testGatewayName},
		{Name: "namespace", Value: testNamespace},
		{Name: "syncer", Value: "test-syncer"},
	})
	currentMetrics.AssertHistogramPopulated("kgateway_status_syncer_status_sync_duration_seconds")
}

func TestCollectStatusSyncMetrics_Error(t *testing.T) {
	setupTest()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	finishFunc := collectStatusSyncMetrics(statusSyncMetricLabels{
		Name:      testGatewayName,
		Namespace: testNamespace,
		Syncer:    testSyncerName,
	})
	finishFunc(assert.AnError)

	currentMetrics := metricstest.MustGatherMetricsContext(ctx, t,
		"kgateway_status_syncer_status_syncs_total",
		"kgateway_status_syncer_status_sync_duration_seconds",
	)

	currentMetrics.AssertMetric("kgateway_status_syncer_status_syncs_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "name", Value: testGatewayName},
			{Name: "namespace", Value: testNamespace},
			{Name: "result", Value: "error"},
			{Name: "syncer", Value: "test-syncer"},
		},
		Value: 1,
	})

	currentMetrics.AssertMetricLabels("kgateway_status_syncer_status_sync_duration_seconds", []metrics.Label{
		{Name: "name", Value: testGatewayName},
		{Name: "namespace", Value: testNamespace},
		{Name: "syncer", Value: "test-syncer"},
	})
	currentMetrics.AssertHistogramPopulated("kgateway_status_syncer_status_sync_duration_seconds")
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
						Resource: &envoyclusterv3.Cluster{
							Name: "test",
							TransportSocketMatches: []*envoyclusterv3.Cluster_TransportSocketMatch{{
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
					Endpoints: &envoyendpointv3.ClusterLoadAssignment{
						ClusterName: "test",
						Endpoints: []*envoyendpointv3.LocalityLbEndpoints{
							{
								Locality: &envoycorev3.Locality{
									Region:  "region1",
									Zone:    "zone1",
									SubZone: "subzone1",
								},
								LbEndpoints: []*envoyendpointv3.LbEndpoint{{
									HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
										Endpoint: &envoyendpointv3.Endpoint{
											Address: &envoycorev3.Address{
												Address: &envoycorev3.Address_SocketAddress{
													SocketAddress: &envoycorev3.SocketAddress{
														Address: "",
														PortSpecifier: &envoycorev3.SocketAddress_PortValue{
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
					Cluster: &envoyclusterv3.Cluster{
						TransportSocketMatches: []*envoyclusterv3.Cluster_TransportSocketMatch{{
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
			setupTest()

			mock := krttest.NewMock(t, tc.inputs)
			mockUcc := krttest.GetMockCollection[ir.UniqlyConnectedClient](mock)
			mockGatewayXDSResorces := krttest.GetMockCollection[GatewayXdsResources](mock)
			mockUccWithEndpoints := krttest.GetMockCollection[UccWithEndpoints](mock)
			mockUccWithCluster := krttest.GetMockCollection[uccWithCluster](mock)

			c := snapshotPerClient(krtinternal.KrtOptions{}, mockUcc, mockGatewayXDSResorces,
				PerClientEnvoyEndpoints{
					endpoints: mockUccWithEndpoints,
					index: krtpkg.UnnamedIndex(mockUccWithEndpoints, func(ucc UccWithEndpoints) []string {
						return []string{ucc.Client.ResourceName()}
					}),
				},
				PerClientEnvoyClusters{
					clusters: mockUccWithCluster,
					index: krtpkg.UnnamedIndex(mockUccWithCluster, func(ucc uccWithCluster) []string {
						return []string{ucc.Client.ResourceName()}
					}),
				})

			c.WaitUntilSynced(nil)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			gathered := metricstest.MustGatherMetricsContext(ctx, t,
				"kgateway_xds_snapshot_transforms_total",
				"kgateway_xds_snapshot_transform_duration_seconds",
				"kgateway_xds_snapshot_resources",
			)

			gathered.AssertMetric("kgateway_xds_snapshot_transforms_total", &metricstest.ExpectedMetric{
				Labels: []metrics.Label{
					{Name: "gateway", Value: "test"},
					{Name: "namespace", Value: "ns"},
					{Name: "result", Value: "success"},
				},
				Value: 1,
			})

			gathered.AssertMetricsLabels("kgateway_xds_snapshot_transform_duration_seconds", [][]metrics.Label{{
				{Name: "gateway", Value: "test"},
				{Name: "namespace", Value: "ns"},
			}})
			gathered.AssertHistogramPopulated("kgateway_xds_snapshot_transform_duration_seconds")

			gathered.AssertMetrics("kgateway_xds_snapshot_resources", []metricstest.ExpectMetric{
				&metricstest.ExpectedMetric{
					Labels: []metrics.Label{
						{Name: "gateway", Value: "test"},
						{Name: "namespace", Value: "ns"},
						{Name: "resource", Value: "Cluster"},
					},
					Value: 1,
				},
				&metricstest.ExpectedMetric{
					Labels: []metrics.Label{
						{Name: "gateway", Value: "test"},
						{Name: "namespace", Value: "ns"},
						{Name: "resource", Value: "Endpoint"},
					},
					Value: 1,
				},
				&metricstest.ExpectedMetric{
					Labels: []metrics.Label{
						{Name: "gateway", Value: "test"},
						{Name: "namespace", Value: "ns"},
						{Name: "resource", Value: "Listener"},
					},
					Value: 1,
				},
				&metricstest.ExpectedMetric{
					Labels: []metrics.Label{
						{Name: "gateway", Value: "test"},
						{Name: "namespace", Value: "ns"},
						{Name: "resource", Value: "Route"},
					},
					Value: 1,
				},
			})
		})
	}
}

func TestResourceSyncMetrics(t *testing.T) {
	setupTest()

	testNS := "test-namespace"
	testName := "test-name"
	testResource := "test-resource"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tmetrics.StartResourceSyncMetricsProcessing(ctx)

	tmetrics.StartResourceSync(testName, tmetrics.ResourceMetricLabels{
		Gateway:   testName,
		Namespace: testNS,
		Resource:  testResource,
	})

	tmetrics.EndResourceSync(tmetrics.ResourceSyncDetails{
		Gateway:      testName,
		Namespace:    testNS,
		ResourceType: testResource,
		ResourceName: testName,
	}, false, resourcesXDSSyncsTotal, resourcesXDSSyncDuration)

	gathered := metricstest.MustGatherMetricsContext(ctx, t,
		"kgateway_resources_syncs_started_total",
		"kgateway_resources_xds_snapshot_syncs_total",
		"kgateway_resources_xds_snapshot_sync_duration_seconds",
	)

	gathered.AssertMetric("kgateway_resources_syncs_started_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "gateway", Value: testName},
			{Name: "namespace", Value: testNS},
			{Name: "resource", Value: testResource},
		},
		Value: 1,
	})

	gathered.AssertMetric("kgateway_resources_xds_snapshot_syncs_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "gateway", Value: testName},
			{Name: "namespace", Value: testNS},
			{Name: "resource", Value: testResource},
		},
		Value: 1,
	})

	gathered.AssertMetricsLabels("kgateway_resources_xds_snapshot_sync_duration_seconds", [][]metrics.Label{{
		{Name: "gateway", Value: testName},
		{Name: "namespace", Value: testNS},
		{Name: "resource", Value: testResource},
	}})
	gathered.AssertHistogramPopulated("kgateway_resources_xds_snapshot_sync_duration_seconds")
}

func TestGetDetailsFromXDSClientResourceName(t *testing.T) {
	testCases := []struct {
		name     string
		resource string
		expected struct {
			role      string
			gateway   string
			namespace string
		}
	}{
		{
			name:     "Valid resource name",
			resource: "kgateway-kube-gateway-api~ns~test",
			expected: struct {
				role      string
				gateway   string
				namespace string
			}{
				role:      "kgateway-kube-gateway-api",
				gateway:   "test",
				namespace: "ns",
			},
		},
		{
			name:     "Invalid resource name",
			resource: "invalid-resource-name",
			expected: struct {
				role      string
				gateway   string
				namespace string
			}{
				role:      "unknown",
				gateway:   "unknown",
				namespace: "unknown",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cd := getDetailsFromXDSClientResourceName(tc.resource)
			assert.Equal(t, tc.expected.gateway, cd.Gateway)
			assert.Equal(t, tc.expected.namespace, cd.Namespace)
		})
	}
}
