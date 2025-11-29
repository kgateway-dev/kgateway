package setup

import (
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/xds"
	kmetrics "github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

// helper to build a DiscoveryRequest
func dr(owner, ns, name, typeURL string, err *status.Status) *discoveryv3.DiscoveryRequest {
	return &discoveryv3.DiscoveryRequest{
		Node: &corev3.Node{
			Metadata: &structpb.Struct{Fields: map[string]*structpb.Value{
				xds.RoleKey: structpb.NewStringValue(owner + xds.KeyDelimiter + ns + xds.KeyDelimiter + name),
			}},
		},
		TypeUrl:     typeURL,
		ErrorDetail: err,
	}
}

func labels(ns, name, typeURL string) []kmetrics.Label {
	return []kmetrics.Label{
		{Name: gwNamespaceLabel, Value: ns},
		{Name: gwNameLabel, Value: name},
		{Name: typeURLLabel, Value: typeURL},
	}
}

// reset metrics between tests to avoid cross-test contamination
func resetMetrics() {
	xdsRejectsTotal.Reset()
	xdsRejectsCurrent.Reset()
}

// gather helper returning counter and gauge expected metric objects for inclusion assertions
func expectedCounter(val float64, ns, name, typeURL string) *metricstest.ExpectedMetric {
	return &metricstest.ExpectedMetric{Labels: labels(ns, name, typeURL), Value: val}
}

func expectedGauge(val float64, ns, name, typeURL string) *metricstest.ExpectedMetric {
	return &metricstest.ExpectedMetric{Labels: labels(ns, name, typeURL), Value: val}
}

const (
	owner    = "owner"
	ns       = "test-ns"
	name     = "gw"
	typeURL  = "envoy.config.cluster.v3.Cluster" // trimmed by code from type.googleapis.com/ prefix
	fullType = "type.googleapis.com/" + typeURL
)

func TestSingleErrorLifecycle(t *testing.T) {
	resetMetrics()
	cb := newLogNackCallback()

	// First request with an error -> increments total and gauge
	require.NoError(t, cb.OnStreamRequest(1, dr(owner, ns, name, fullType, &status.Status{Message: "boom"})))
	gathered := metricstest.MustGatherMetrics(t)
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_total", []metricstest.ExpectMetric{expectedCounter(1, ns, name, typeURL)})
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_gauge", []metricstest.ExpectMetric{expectedGauge(1, ns, name, typeURL)})

	// Second identical error for same stream/resource should not change metrics
	require.NoError(t, cb.OnStreamRequest(1, dr(owner, ns, name, fullType, &status.Status{Message: "boom"})))
	gathered = metricstest.MustGatherMetrics(t)
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_total", []metricstest.ExpectMetric{expectedCounter(1, ns, name, typeURL)})
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_gauge", []metricstest.ExpectMetric{expectedGauge(1, ns, name, typeURL)})

	// Successful request clears gauge but not counter
	require.NoError(t, cb.OnStreamRequest(1, dr(owner, ns, name, fullType, nil)))
	gathered = metricstest.MustGatherMetrics(t)
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_total", []metricstest.ExpectMetric{expectedCounter(1, ns, name, typeURL)})
	// Gauge metric may disappear entirely after reset to 0; we assert either absence or value 0 for our labels
	// If present, it must have value 0 with our labels; if not present, that's acceptable
	if gathered.MetricLength("kgateway_envoy_xds_rejects_gauge") > 0 {
		gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_gauge", []metricstest.ExpectMetric{expectedGauge(0, ns, name, typeURL)})
	}
}

func TestMultipleResourcesAndStreams(t *testing.T) {
	resetMetrics()
	cb := newLogNackCallback()

	// Stream 1 errors on resource A and B
	require.NoError(t, cb.OnStreamRequest(1, dr(owner, ns, name, fullType, &status.Status{Message: "errA"})))
	typeURL2 := "envoy.config.listener.v3.Listener"
	fullType2 := "type.googleapis.com/" + typeURL2
	require.NoError(t, cb.OnStreamRequest(1, dr(owner, ns, name, fullType2, &status.Status{Message: "errB"})))

	// Stream 2 error on resource A (same labels as first error)
	require.NoError(t, cb.OnStreamRequest(2, dr(owner, ns, name, fullType, &status.Status{Message: "errA"})))

	gathered := metricstest.MustGatherMetrics(t)
	// Counter: A twice (stream1+stream2) + B once = 3 total increments across label sets
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_total", []metricstest.ExpectMetric{
		expectedCounter(2, ns, name, typeURL), // resource A counted twice
		expectedCounter(1, ns, name, typeURL2),
	})
	// Gauge: currently outstanding errors: A (2 streams) + B (1) => A gauge=2, B gauge=1
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_gauge", []metricstest.ExpectMetric{
		expectedGauge(2, ns, name, typeURL),
		expectedGauge(1, ns, name, typeURL2),
	})

	// Clear resource A error on stream 1 only
	require.NoError(t, cb.OnStreamRequest(1, dr(owner, ns, name, fullType, nil)))
	gathered = metricstest.MustGatherMetrics(t)
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_total", []metricstest.ExpectMetric{
		expectedCounter(2, ns, name, typeURL),
		expectedCounter(1, ns, name, typeURL2),
	})
	// Gauge should show A=1 (stream2 still failing), B=1
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_gauge", []metricstest.ExpectMetric{
		expectedGauge(1, ns, name, typeURL),
		expectedGauge(1, ns, name, typeURL2),
	})

	// Close stream 2 (remaining A error) and stream 1 (B error)
	cb.OnStreamClosed(2, nil)
	cb.OnStreamClosed(1, nil)
	gathered = metricstest.MustGatherMetrics(t)
	// Counter values unchanged
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_total", []metricstest.ExpectMetric{
		expectedCounter(2, ns, name, typeURL),
		expectedCounter(1, ns, name, typeURL2),
	})
	// Gauges should now either be absent or zero; if present assert zero
	if gathered.MetricLength("kgateway_envoy_xds_rejects_gauge") > 0 {
		// We allow any remaining metrics to be zero; check inclusion semantics
		gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_gauge", []metricstest.ExpectMetric{
			expectedGauge(0, ns, name, typeURL),
			expectedGauge(0, ns, name, typeURL2),
		})
	}
}
