package krtcollections

import (
	"context"
	"fmt"
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/require"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/pkg/security"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
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

func TestShortTypeURL(t *testing.T) {
	cases := map[string]string{
		resourcev3.ClusterType:         "Cluster",
		resourcev3.EndpointType:        "ClusterLoadAssignment",
		resourcev3.RouteType:           "RouteConfiguration",
		resourcev3.ScopedRouteType:     "ScopedRouteConfiguration",
		resourcev3.VirtualHostType:     "VirtualHost",
		resourcev3.ListenerType:        "Listener",
		resourcev3.SecretType:          "Secret",
		resourcev3.ExtensionConfigType: "TypedExtensionConfig",
		resourcev3.RuntimeType:         "Runtime",
		"":                             "other", "Cluster": "other", "made.up.Cluster": "other",
		"type.googleapis.com/made.up.Resource": "other",
	}
	for input, want := range cases {
		require.Equal(t, want, shortTypeURL(input), "type URL %q", input)
	}
}

func TestNackRequestIdentity(t *testing.T) {
	t.Cleanup(SetXdsFirstConnectDelayForTest(0))
	for _, tc := range []struct {
		name                          string
		auth, initialized, peer, skip bool
		wantErr                       bool
		gateway, namespace            string
	}{
		{name: "authenticated spoof", auth: true, initialized: true, peer: true, gateway: "real-gateway", namespace: "real-ns"},
		{name: "unauthenticated arbitrary roles", initialized: true, gateway: "unknown", namespace: "unknown"},
		{name: "not initialized", wantErr: true},
		{name: "missing authenticated peer", initialized: true, auth: true, wantErr: true},
		{name: "non kgateway role", initialized: true, skip: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			xdsNacksTotal.Reset()
			// Reset seeds an empty-label zero series; remove it so assertions
			// account for exactly the series produced by these requests.
			xdsNacksTotal.DeletePartialMatch(metrics.Label{Name: "gateway", Value: ""})
			t.Cleanup(xdsNacksTotal.Reset)
			cb := &callbacks{xdsAuth: tc.auth}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.initialized {
				buildCollection(cb)(ctx, krtutil.KrtOptions{Stop: ctx.Done()}, nil)
			}
			if tc.peer {
				ctx = context.WithValue(ctx, xds.PeerCtxKey, &security.Caller{KubernetesInfo: security.KubernetesInfo{
					PodNamespace: "real-ns", PodServiceAccount: "real-gateway",
				}})
				require.NoError(t, cb.OnStreamOpen(ctx, 1, ""))
			}
			for i := range 20 {
				role := fmt.Sprintf("kgateway-kube-gateway-api~fake-ns-%d~fake-gateway-%d", i, i)
				if tc.skip {
					role = fmt.Sprintf("other~fake-ns-%d~fake-gateway-%d", i, i)
				}
				req := &envoy_service_discovery_v3.DiscoveryRequest{
					Node: &envoycorev3.Node{Id: "pod.ns", Metadata: &structpb.Struct{Fields: map[string]*structpb.Value{
						xds.RoleKey: structpb.NewStringValue(role),
					}}},
					TypeUrl:     fmt.Sprintf("made.up.Type%d", i),
					ErrorDetail: &statuspb.Status{Message: "rejected"},
				}
				err := cb.OnStreamRequest(1, req)
				if tc.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			}
			gathered := metricstest.MustGatherMetrics(t)
			if tc.gateway == "" {
				gathered.AssertMetricNotExists("kgateway_xds_nacks_total")
			} else {
				require.Equal(t, 1, gathered.MetricLength("kgateway_xds_nacks_total"))
				gathered.AssertMetrics("kgateway_xds_nacks_total", []metricstest.ExpectMetric{
					&metricstest.ExpectedMetric{Labels: []metrics.Label{
						{Name: "type_url", Value: "other"},
						{Name: "gateway", Value: tc.gateway},
						{Name: "namespace", Value: tc.namespace},
					}, Value: 20},
				})
			}
			if tc.initialized {
				cb.OnStreamClosed(1, nil)
			}
		})
	}
}
