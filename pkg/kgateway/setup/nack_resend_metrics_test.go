package setup

import (
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	streamv3 "github.com/envoyproxy/go-control-plane/pkg/server/stream/v3"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

func TestNackResendsSuppressedMetric(t *testing.T) {
	const metricName = "kgateway_envoy_xds_nack_resends_suppressed_total"
	for _, resourceType := range []string{envoyresource.ClusterType, envoyresource.EndpointType} {
		for _, scenario := range []string{"suppressed", "new snapshot", "missing snapshot", "stale nonce", "ACK", "untracked"} {
			t.Run(resourceType+"/"+scenario, func(t *testing.T) {
				xdsNackResendsSuppressed.Reset()
				t.Cleanup(xdsNackResendsSuppressed.Reset)
				metricLabels := []metrics.Label{{Name: typeURLLabel, Value: rejectionTypeURL(resourceType)}}
				xdsNackResendsSuppressed.Add(0, metricLabels...)
				policy := newWatchPolicy(envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil), envoycache.IDHash{}, true, false)
				node := &envoycorev3.Node{Id: "node"}
				version := "rejected"
				if scenario == "new snapshot" {
					version = "corrected"
				}
				if scenario != "missing snapshot" {
					require.NoError(t, policy.SetSnapshot(t.Context(), node.Id, nackSnapshot(version)))
				}
				initial := &discoveryv3.DiscoveryRequest{Node: node, TypeUrl: resourceType}
				require.NoError(t, policy.onStreamRequest(1, initial))
				if scenario != "untracked" {
					policy.onStreamResponse(t.Context(), 1, initial, &discoveryv3.DiscoveryResponse{TypeUrl: resourceType, Nonce: "1", VersionInfo: "rejected"})
				}
				req := nackRequest(node, "1")
				req.TypeUrl = resourceType
				if scenario == "stale nonce" {
					req.ResponseNonce = "old"
				}
				if scenario == "ACK" {
					req.ErrorDetail = nil
				}
				require.NoError(t, policy.onStreamRequest(1, req))
				cancel, err := policy.CreateWatch(req, streamv3.NewSotwSubscription(nil, true), make(chan envoycache.Response, 1))
				require.NoError(t, err)
				if cancel != nil {
					t.Cleanup(cancel)
				}
				gathered := metricstest.MustGatherMetrics(t)
				want := float64(0)
				if scenario == "suppressed" {
					want = 1
				}
				got := gathered.MustGetMetricValueByLabels(metricName, metricLabels)
				require.Equal(t, want, got, "only a suppressed resend should increment the counter")
			})
		}
	}
}

// Every reconnecting proxy's first endpoint request counts once, under the same
// type_url values as the rejects metrics; a cold request and an ACK do not.
func TestReconnectResponsesMetric(t *testing.T) {
	const metricName = "kgateway_envoy_xds_reconnect_responses_total"
	xdsReconnectResponses.Reset()
	t.Cleanup(xdsReconnectResponses.Reset)
	metricLabels := []metrics.Label{{Name: typeURLLabel, Value: rejectionTypeURL(envoyresource.EndpointType)}}
	policy := newWatchPolicy(envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil), envoycache.IDHash{}, false, true)
	node := &envoycorev3.Node{Id: "node"}
	watch := func(req *discoveryv3.DiscoveryRequest) {
		t.Helper()
		cancel, err := policy.CreateWatch(req, streamv3.NewSotwSubscription(nil, true), make(chan envoycache.Response, 1))
		require.NoError(t, err)
		if cancel != nil {
			t.Cleanup(cancel)
		}
	}

	watch(&discoveryv3.DiscoveryRequest{Node: node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{"c"}})
	watch(&discoveryv3.DiscoveryRequest{Node: node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{"c"}, VersionInfo: "1", ResponseNonce: "1"})
	xdsReconnectResponses.Add(0, metricLabels...)
	require.Equal(t, float64(0), metricstest.MustGatherMetrics(t).MustGetMetricValueByLabels(metricName, metricLabels),
		"a cold request and an ACK are not reconnects")

	watch(&discoveryv3.DiscoveryRequest{Node: node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{"c"}, VersionInfo: "1"})
	require.Equal(t, float64(1), metricstest.MustGatherMetrics(t).MustGetMetricValueByLabels(metricName, metricLabels))
}
