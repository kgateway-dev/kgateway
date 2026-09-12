package krtcollections

import (
	"context"
	"testing"
	"time"

	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

func TestRecordNackIfAny(t *testing.T) {
	xdsNacksTotal.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	role := "kgateway-kube-gateway-api~gw-ns~gw-name"

	// An ACK (no ErrorDetail) must not count.
	recordNackIfAny(role, &envoy_service_discovery_v3.DiscoveryRequest{
		TypeUrl:     "type.googleapis.com/envoy.config.route.v3.RouteConfiguration",
		VersionInfo: "5",
	})

	// A NACK counts, labeled by short type URL and the role's gateway identity.
	recordNackIfAny(role, &envoy_service_discovery_v3.DiscoveryRequest{
		TypeUrl:       "type.googleapis.com/envoy.config.route.v3.RouteConfiguration",
		VersionInfo:   "4", // last-accepted
		ResponseNonce: "nonce-5",
		ErrorDetail:   &statuspb.Status{Message: "invalid route configuration"},
	})

	// The unique cache key derived from the role keeps the same first three
	// segments, so follow-up requests on rewritten nodes label identically.
	recordNackIfAny(role+"~12345~pod-ns", &envoy_service_discovery_v3.DiscoveryRequest{
		TypeUrl:     "type.googleapis.com/envoy.config.cluster.v3.Cluster",
		ErrorDetail: &statuspb.Status{Message: "invalid cluster"},
	})

	gathered := metricstest.MustGatherMetricsContext(ctx, t, "kgateway_xds_nacks_total")
	gathered.AssertMetricsInclude("kgateway_xds_nacks_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetricValueTest{
			Labels: []metrics.Label{
				{Name: "type_url", Value: "RouteConfiguration"},
				{Name: "gateway", Value: "gw-name"},
				{Name: "namespace", Value: "gw-ns"},
			},
			Test: metricstest.Equal(1),
		},
		&metricstest.ExpectedMetricValueTest{
			Labels: []metrics.Label{
				{Name: "type_url", Value: "Cluster"},
				{Name: "gateway", Value: "gw-name"},
				{Name: "namespace", Value: "gw-ns"},
			},
			Test: metricstest.Equal(1),
		},
	})
}

func TestGatewayFromRole(t *testing.T) {
	cases := []struct {
		role      string
		gateway   string
		namespace string
	}{
		{"kgateway-kube-gateway-api~ns~gw", "gw", "ns"},
		{"kgateway-kube-gateway-api~ns~gw~123~pod-ns", "gw", "ns"},
		{"something-else", "unknown", "unknown"},
		{"", "unknown", "unknown"},
	}
	for _, tc := range cases {
		gw, ns := gatewayFromRole(tc.role)
		if gw != tc.gateway || ns != tc.namespace {
			t.Errorf("gatewayFromRole(%q) = (%q, %q), want (%q, %q)", tc.role, gw, ns, tc.gateway, tc.namespace)
		}
	}
}
