package setup

import (
	"context"
	"errors"
	"fmt"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/security"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	kmetrics "github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

const (
	owner    = wellknown.GatewayApiProxyValue
	ns       = "test-ns"
	name     = "gw"
	typeURL  = "envoy.config.cluster.v3.Cluster" // trimmed by code from type.googleapis.com/ prefix
	fullType = "type.googleapis.com/" + typeURL
)

// helper to build a DiscoveryRequest
func dr(typeURL string, err *status.Status) *discoveryv3.DiscoveryRequest {
	return &discoveryv3.DiscoveryRequest{
		Node: &envoycorev3.Node{
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
func expectedCounter(val float64, typeURL string) *metricstest.ExpectedMetric {
	return &metricstest.ExpectedMetric{Labels: labels(ns, name, typeURL), Value: val}
}

func expectedGauge(val float64, typeURL string) *metricstest.ExpectedMetric {
	return &metricstest.ExpectedMetric{Labels: labels(ns, name, typeURL), Value: val}
}

func TestSingleErrorLifecycle(t *testing.T) {
	resetMetrics()
	cb := newLogNackCallback(true)
	require.NoError(t, cb.OnStreamOpen(authenticatedContext(), 1, ""))

	// First request with an error -> increments total and gauge
	require.NoError(t, cb.OnStreamRequest(1, dr(fullType, &status.Status{Message: "boom"})))
	gathered := metricstest.MustGatherMetrics(t)
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_total", []metricstest.ExpectMetric{expectedCounter(1, typeURL)})
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_active", []metricstest.ExpectMetric{expectedGauge(1, typeURL)})

	// Second identical error for same stream/resource should not change metrics
	require.NoError(t, cb.OnStreamRequest(1, dr(fullType, &status.Status{Message: "boom"})))
	gathered = metricstest.MustGatherMetrics(t)
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_total", []metricstest.ExpectMetric{expectedCounter(1, typeURL)})
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_active", []metricstest.ExpectMetric{expectedGauge(1, typeURL)})

	// Successful request clears gauge but not counter
	require.NoError(t, cb.OnStreamRequest(1, dr(fullType, nil)))
	gathered = metricstest.MustGatherMetrics(t)
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_total", []metricstest.ExpectMetric{expectedCounter(1, typeURL)})
	// Gauge metric may disappear entirely after reset to 0; we assert either absence or value 0 for our labels
	// If present, it must have value 0 with our labels; if not present, that's acceptable
	if gathered.MetricLength("kgateway_envoy_xds_rejects_active") > 0 {
		gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_active", []metricstest.ExpectMetric{expectedGauge(0, typeURL)})
	}

	// A later rejection begins a new episode, even with no node metadata.
	require.NoError(t, cb.OnStreamRequest(1, &discoveryv3.DiscoveryRequest{
		TypeUrl: fullType, ErrorDetail: &status.Status{Message: "another rejection"},
	}))
	gathered = metricstest.MustGatherMetrics(t)
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_total", []metricstest.ExpectMetric{expectedCounter(2, typeURL)})
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_active", []metricstest.ExpectMetric{expectedGauge(1, typeURL)})
	cb.OnStreamClosed(1, nil)
}

func TestMultipleResourcesAndStreams(t *testing.T) {
	resetMetrics()
	cb := newLogNackCallback(true)
	require.NoError(t, cb.OnStreamOpen(authenticatedContext(), 1, ""))

	// Stream 1 errors on resource A and B
	require.NoError(t, cb.OnStreamRequest(1, dr(fullType, &status.Status{Message: "errA"})))
	typeURL2 := "envoy.config.listener.v3.Listener"
	fullType2 := "type.googleapis.com/" + typeURL2
	require.NoError(t, cb.OnStreamRequest(1, dr(fullType2, &status.Status{Message: "errB"})))

	require.NoError(t, cb.OnStreamOpen(authenticatedContext(), 2, ""))
	// Stream 2 error on resource A (same labels as first error)
	require.NoError(t, cb.OnStreamRequest(2, dr(fullType, &status.Status{Message: "errA"})))

	gathered := metricstest.MustGatherMetrics(t)
	// Counter: A twice (stream1+stream2) + B once = 3 total increments across label sets
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_total", []metricstest.ExpectMetric{
		expectedCounter(2, typeURL), // resource A counted twice
		expectedCounter(1, typeURL2),
	})
	// Gauge: currently outstanding errors: A (2 streams) + B (1) => A gauge=2, B gauge=1
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_active", []metricstest.ExpectMetric{
		expectedGauge(2, typeURL),
		expectedGauge(1, typeURL2),
	})

	// Clear resource A error on stream 1 only
	require.NoError(t, cb.OnStreamRequest(1, dr(fullType, nil)))
	gathered = metricstest.MustGatherMetrics(t)
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_total", []metricstest.ExpectMetric{
		expectedCounter(2, typeURL),
		expectedCounter(1, typeURL2),
	})
	// Gauge should show A=1 (stream2 still failing), B=1
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_active", []metricstest.ExpectMetric{
		expectedGauge(1, typeURL),
		expectedGauge(1, typeURL2),
	})

	// Close stream 2 (remaining A error) and stream 1 (B error)
	cb.OnStreamClosed(2, nil)
	cb.OnStreamClosed(1, nil)
	gathered = metricstest.MustGatherMetrics(t)
	// Counter values unchanged
	gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_total", []metricstest.ExpectMetric{
		expectedCounter(2, typeURL),
		expectedCounter(1, typeURL2),
	})
	// Gauges should now either be absent or zero; if present assert zero
	if gathered.MetricLength("kgateway_envoy_xds_rejects_active") > 0 {
		// We allow any remaining metrics to be zero; check inclusion semantics
		gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_active", []metricstest.ExpectMetric{
			expectedGauge(0, typeURL),
			expectedGauge(0, typeURL2),
		})
	}
}

func authenticatedContext() context.Context {
	return context.WithValue(context.Background(), xds.PeerCtxKey, &security.Caller{
		KubernetesInfo: security.KubernetesInfo{PodNamespace: ns, PodServiceAccount: name},
	})
}

func TestRejectionMetricIdentityAndCardinality(t *testing.T) {
	for _, auth := range []bool{true, false} {
		t.Run(fmt.Sprintf("auth=%t", auth), func(t *testing.T) {
			resetMetrics()
			t.Cleanup(resetMetrics)
			// Reset seeds an empty-label series; exclude it from cardinality assertions.
			xdsRejectsTotal.DeletePartialMatch(kmetrics.Label{Name: gwNameLabel, Value: ""})
			xdsRejectsCurrent.DeletePartialMatch(kmetrics.Label{Name: gwNameLabel, Value: ""})
			cb := newLogNackCallback(auth)
			ctx, cancel := context.WithCancel(authenticatedContext())
			defer cancel()
			clients, build, _ := krtcollections.NewUniquelyConnectedClients(nil, auth)
			col := build(ctx, krtutil.KrtOptions{Stop: ctx.Done()}, nil)
			require.True(t, col.WaitUntilSynced(ctx.Done()))
			chain := chainCallbacks(clients, cb)
			require.NoError(t, chain.OnStreamOpen(ctx, 1, ""))
			for i := range 20 {
				req := dr(fmt.Sprintf("type.googleapis.com/arbitrary.Type%d", i), &status.Status{Message: "rejected"})
				req.Node.Metadata.Fields[xds.RoleKey] = structpb.NewStringValue(fmt.Sprintf(owner+"~fake-ns-%d~fake-gateway-%d", i, i))
				require.NoError(t, chain.OnStreamRequest(1, req))
			}
			wantNS, wantName := "unknown", "unknown"
			if auth {
				wantNS, wantName = ns, name
			}
			want := &metricstest.ExpectedMetric{Labels: labels(wantNS, wantName, "other"), Value: 1}
			gathered := metricstest.MustGatherMetrics(t)
			gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_total", []metricstest.ExpectMetric{want})
			gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_active", []metricstest.ExpectMetric{want})
			require.Equal(t, 1, gathered.MetricLength("kgateway_envoy_xds_rejects_total"))
			require.Equal(t, 1, gathered.MetricLength("kgateway_envoy_xds_rejects_active"))
			require.Len(t, cb.streamState[1].errors, 1)
			chain.OnStreamClosed(1, nil)
			require.Empty(t, cb.streamIdentity)
			require.Empty(t, cb.streamState)
			want.Value = 0
			metricstest.MustGatherMetrics(t).AssertMetricsInclude("kgateway_envoy_xds_rejects_active", []metricstest.ExpectMetric{want})
		})
	}
}

func TestRejectionRequiresAcceptedStream(t *testing.T) {
	resetMetrics()
	t.Cleanup(resetMetrics)
	cb := newLogNackCallback(true)
	require.ErrorContains(t, cb.OnStreamOpen(context.Background(), 1, ""), "missing authenticated xDS peer")
	require.NoError(t, cb.OnStreamRequest(1, dr(fullType, &status.Status{Message: "rejected"})))
	require.Empty(t, cb.streamState)

	rejected := errors.New("request rejected")
	chain := chainCallbacks(xdsserver.CallbackFuncs{
		StreamRequestFunc: func(_ int64, _ *discoveryv3.DiscoveryRequest) error { return rejected },
	}, cb)
	require.NoError(t, chain.OnStreamOpen(authenticatedContext(), 2, ""))
	require.ErrorIs(t, chain.OnStreamRequest(2, dr(fullType, &status.Status{Message: "rejected"})), rejected)
	require.Empty(t, cb.streamState)
	chain.OnStreamClosed(2, nil)
	require.Empty(t, cb.streamIdentity)
}

func TestRejectionTypeURL(t *testing.T) {
	for _, known := range []string{
		resourcev3.ClusterType, resourcev3.EndpointType, resourcev3.RouteType,
		resourcev3.ScopedRouteType, resourcev3.VirtualHostType, resourcev3.ListenerType,
		resourcev3.SecretType, resourcev3.ExtensionConfigType, resourcev3.RuntimeType,
	} {
		require.Equal(t, known[len("type.googleapis.com/"):], rejectionTypeURL(known))
	}
	for _, unknown := range []string{"", "Cluster", "made.up.Cluster", "type.googleapis.com/made.up.Resource"} {
		require.Equal(t, "other", rejectionTypeURL(unknown))
	}
}

// Metrics must use the same pod-normalized gateway as xDS publication, including
// after identity drift closes a stream and the client reconnects (#14582).
func TestRejectionMetricsFollowResolvedIdentity(t *testing.T) {
	resetMetrics()
	t.Cleanup(resetMetrics)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), xds.PeerCtxKey, &security.Caller{
		KubernetesInfo: security.KubernetesInfo{
			PodNamespace: ns, PodName: "proxy-pod", PodServiceAccount: "custom-service-account",
		},
	}))
	defer cancel()
	pod := krtcollections.LocalityPod{
		Named:           krt.Named{Name: "proxy-pod", Namespace: ns},
		AugmentedLabels: map[string]string{"gateway.networking.k8s.io/gateway-name": "first-gateway"},
	}
	pods := krt.NewStaticCollection[krtcollections.LocalityPod](nil, []krtcollections.LocalityPod{pod})
	clients, build, _ := krtcollections.NewUniquelyConnectedClients(nil, true)
	col := build(ctx, krtutil.KrtOptions{Stop: ctx.Done()}, pods)
	require.True(t, col.WaitUntilSynced(ctx.Done()))
	metricsCallback := newLogNackCallback(true)
	chain := chainCallbacks(clients, metricsCallback)
	require.NoError(t, chain.OnStreamOpen(ctx, 1, ""))
	req := dr(fullType, &status.Status{Message: "rejected"})
	// The request's role deliberately disagrees with both the pod label and SA.
	req.Node.Metadata.Fields[xds.RoleKey] = structpb.NewStringValue(owner + "~spoofed-ns~spoofed-gateway")
	require.NoError(t, chain.OnStreamRequest(1, req))
	assertMetrics := func(gateway string, total, active float64) {
		t.Helper()
		gathered := metricstest.MustGatherMetrics(t)
		gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_total", []metricstest.ExpectMetric{
			&metricstest.ExpectedMetric{Labels: labels(ns, gateway, typeURL), Value: total},
		})
		gathered.AssertMetricsInclude("kgateway_envoy_xds_rejects_active", []metricstest.ExpectMetric{
			&metricstest.ExpectedMetric{Labels: labels(ns, gateway, typeURL), Value: active},
		})
	}
	assertMetrics("first-gateway", 1, 1)
	// Follow-ups reuse the rewritten node, just like go-control-plane.
	require.NoError(t, chain.OnStreamRequest(1, req))
	assertMetrics("first-gateway", 1, 1)

	pod.AugmentedLabels = map[string]string{"gateway.networking.k8s.io/gateway-name": "second-gateway"}
	pods.UpdateObject(pod)
	require.ErrorContains(t, chain.OnStreamRequest(1, req), "xds client identity changed")
	// The rejected request must not reach the metrics callback or move its gauge.
	assertMetrics("first-gateway", 1, 1)
	require.Equal(t, "first-gateway", metricsCallback.streamIdentity[1].Name)
	chain.OnStreamClosed(1, req.Node)
	assertMetrics("first-gateway", 1, 0)

	require.NoError(t, chain.OnStreamOpen(ctx, 2, ""))
	require.NoError(t, chain.OnStreamRequest(2, req))
	assertMetrics("second-gateway", 1, 1)
	req.ErrorDetail = nil
	require.NoError(t, chain.OnStreamRequest(2, req))
	assertMetrics("second-gateway", 1, 0)
	chain.OnStreamClosed(2, req.Node)
	require.Empty(t, metricsCallback.streamIdentity)
}
