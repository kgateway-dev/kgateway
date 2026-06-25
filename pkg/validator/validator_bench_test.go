package validator

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
)

// Benchmarks for the validator modes. The underlying envoy invocation is
// stubbed with a fixed-latency fake so the benchmark measures mode overhead and
// cache behavior, not envoy itself; the real envoy fork costs much more per
// call, so measured speedups scale up accordingly. Set KGW_BENCH_REAL_ENVOY=true
// to run the binary-backed variant (requires an envoy binary in PATH; takes
// minutes).

type fakeLatencyValidator struct {
	latency time.Duration
}

func (f *fakeLatencyValidator) Validate(_ context.Context, _ *envoybootstrapv3.Bootstrap) error {
	time.Sleep(f.latency)
	return nil
}

const benchLatency = 2 * time.Millisecond

func benchBootstrap(i int) *envoybootstrapv3.Bootstrap {
	return &envoybootstrapv3.Bootstrap{
		StaticResources: &envoybootstrapv3.Bootstrap_StaticResources{
			Clusters: []*envoyclusterv3.Cluster{{
				Name:                 fmt.Sprintf("cluster-%d", i),
				ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS},
			}},
		},
	}
}

func BenchmarkValidator_DuplicateWorkload(b *testing.B) {
	bootstrap := benchBootstrap(0)
	for _, tc := range []struct {
		name string
		v    Validator
	}{
		{"binary", &fakeLatencyValidator{latency: benchLatency}},
		{"cache", NewCaching(&fakeLatencyValidator{latency: benchLatency}, 0)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := tc.v.Validate(context.Background(), bootstrap); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkValidator_UniqueWorkload(b *testing.B) {
	const distinct = 4096
	bootstraps := make([]*envoybootstrapv3.Bootstrap, distinct)
	for i := range distinct {
		bootstraps[i] = benchBootstrap(i)
	}
	for _, tc := range []struct {
		name string
		v    Validator
	}{
		{"binary", &fakeLatencyValidator{latency: benchLatency}},
		{"cache", NewCaching(&fakeLatencyValidator{latency: benchLatency}, distinct)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				if err := tc.v.Validate(context.Background(), bootstraps[i%distinct]); err != nil {
					b.Fatal(err)
				}
				i++
			}
		})
	}
}

func BenchmarkValidator_RealEnvoyDuplicate(b *testing.B) {
	if os.Getenv("KGW_BENCH_REAL_ENVOY") != "true" {
		b.Skip("set KGW_BENCH_REAL_ENVOY=true to benchmark against a real envoy binary")
	}
	bootstrap := benchBootstrap(0)
	for _, tc := range []struct {
		name string
		v    Validator
	}{
		{"binary", NewBinary()},
		{"cache", NewCaching(NewBinary(), 0)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				if err := tc.v.Validate(context.Background(), bootstrap); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
