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
				policy := newNackResendSuppressor(envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil), envoycache.IDHash{})
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
