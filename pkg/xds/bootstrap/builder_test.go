// bootstrap_builder_test.go
package bootstrap

import (
	"strings"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/google/go-cmp/cmp"
)

func TestConfigBuilder_Build(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(*ConfigBuilder)
		validate      func(*testing.T, *envoybootstrapv3.Bootstrap)
		wantErrSubstr string
	}{
		{
			name:  "empty builder",
			setup: func(b *ConfigBuilder) {},
			validate: func(t *testing.T, got *envoybootstrapv3.Bootstrap) {
				if got == nil {
					t.Fatal("Build() returned nil bootstrap")
				}
				if n := got.GetNode().GetId(); n != "validation-node-id" {
					t.Fatalf("unexpected node ID: %q", n)
				}
				if len(got.GetStaticResources().GetClusters()) != 0 {
					t.Fatalf("expected no clusters, got %d", len(got.GetStaticResources().GetClusters()))
				}
			},
		},
		{
			name: "with filter config",
			setup: func(b *ConfigBuilder) {
				// Add a dummy per-filter config
				b.AddFilterConfig("test-filter", &envoy_hcm.HttpConnectionManager{StatPrefix: "dummy"})
			},
			validate: func(t *testing.T, got *envoybootstrapv3.Bootstrap) {
				hcm := unmarshalHCM(t, got)
				vhosts := hcm.GetRouteConfig().GetVirtualHosts()
				if len(vhosts) != 1 {
					t.Fatalf("expected 1 vhost, got %d", len(vhosts))
				}
				if _, ok := vhosts[0].GetTypedPerFilterConfig()["test-filter"]; !ok {
					t.Fatalf("typed per-filter config 'test-filter' missing on vhost")
				}
			},
		},
		{
			name: "with clusters",
			setup: func(b *ConfigBuilder) {
				b.AddCluster(&envoyclusterv3.Cluster{Name: "test_cluster"})
			},
			validate: func(t *testing.T, got *envoybootstrapv3.Bootstrap) {
				want := 1
				if diff := cmp.Diff(want, len(got.GetStaticResources().GetClusters())); diff != "" {
					t.Fatalf("cluster count mismatch (-want +got):\n%s", diff)
				}
			},
		},
		{
			name: "with route",
			setup: func(b *ConfigBuilder) {
				b.AddRoute(&envoyroutev3.Route{
					Name: "test_route",
					Match: &envoyroutev3.RouteMatch{
						PathSpecifier: &envoyroutev3.RouteMatch_Prefix{Prefix: "/test"},
					},
				})
			},
			validate: func(t *testing.T, got *envoybootstrapv3.Bootstrap) {
				hcm := unmarshalHCM(t, got)
				vhosts := hcm.GetRouteConfig().GetVirtualHosts()
				if len(vhosts) != 1 {
					t.Fatalf("expected 1 vhost, got %d", len(vhosts))
				}
				routes := vhosts[0].GetRoutes()
				if len(routes) != 1 {
					t.Fatalf("expected 1 route, got %d", len(routes))
				}
				if routes[0].GetName() != "test_route" {
					t.Fatalf("expected route name 'test_route', got %q", routes[0].GetName())
				}
			},
		},
		{
			name: "with AddCluster method",
			setup: func(b *ConfigBuilder) {
				b.AddCluster(&envoyclusterv3.Cluster{Name: "test_cluster_2"})
			},
			validate: func(t *testing.T, got *envoybootstrapv3.Bootstrap) {
				clusters := got.GetStaticResources().GetClusters()
				if len(clusters) != 1 {
					t.Fatalf("expected 1 cluster, got %d", len(clusters))
				}
				if clusters[0].GetName() != "test_cluster_2" {
					t.Fatalf("expected cluster name 'test_cluster_2', got %q", clusters[0].GetName())
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builder := New()
			tc.setup(builder)

			got, err := builder.Build()
			if tc.wantErrSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrSubstr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErrSubstr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Build() returned unexpected error: %v", err)
			}
			tc.validate(t, got)
		})
	}
}

// unmarshalHCM pulls the first HCM filter out of the generated bootstrap for inspection.
func unmarshalHCM(t *testing.T, bs *envoybootstrapv3.Bootstrap) *envoy_hcm.HttpConnectionManager {
	t.Helper()

	l := bs.GetStaticResources().GetListeners()[0]
	hcmAny := l.GetFilterChains()[0].GetFilters()[0].GetTypedConfig()
	hcm := &envoy_hcm.HttpConnectionManager{}
	if err := hcmAny.UnmarshalTo(hcm); err != nil {
		t.Fatalf("failed to unmarshal HCM: %v", err)
	}
	return hcm
}
