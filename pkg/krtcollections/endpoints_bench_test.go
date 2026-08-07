package krtcollections

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func benchBackend(name string) ir.BackendObjectIR {
	return ir.NewBackendObjectIR(ir.ObjectSource{
		Group:     "",
		Kind:      "Service",
		Namespace: "default",
		Name:      name,
	}, 80, "", "")
}

// buildEndpointsForBackend simulates transformK8sEndpoints for a backend with
// `endpoints` ready endpoints spread across `localities` zones.
func buildEndpointsForBackend(name string, endpoints, localities int, autoMtls bool) *ir.EndpointsForBackend {
	ret := ir.NewEndpointsForBackend(benchBackend(name))
	labels := map[string]string{
		"app":                         "demo",
		"security.istio.io/tlsMode":   "istio",
		"topology.kubernetes.io/zone": "us-east-1a",
	}
	for i := range endpoints {
		loc := ir.PodLocality{
			Region: "us-east-1",
			Zone:   fmt.Sprintf("us-east-1%c", 'a'+i%localities),
		}
		ep := CreateLBEndpoint(fmt.Sprintf("10.0.%d.%d", i/256, i%256), 8080, labels, autoMtls)
		ret.Add(loc, ir.EndpointWithMd{
			LbEndpoint: ep,
			EndpointMd: ir.EndpointMetadata{Labels: labels},
		})
	}
	return ret
}

// BenchmarkBuildEndpointsForBackend measures the per-backend endpoint IR build
// (the CreateLBEndpoint + hashing hot path dominating the heap profiles).
func BenchmarkBuildEndpointsForBackend(b *testing.B) {
	for _, endpoints := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("endpoints=%d", endpoints), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				out := buildEndpointsForBackend(fmt.Sprintf("svc-%d", i), endpoints, 3, false)
				if len(out.LbEps) == 0 {
					b.Fatal("empty")
				}
			}
		})
	}
}

// TestBenchHeapDelta reports heap allocated by building N backends' endpoints,
// for manual before/after comparison with runtime.ReadMemStats.
func TestBenchHeapDelta(t *testing.T) {
	const backends = 200
	const endpoints = 1000
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	refs := make([]*ir.EndpointsForBackend, 0, backends)
	for i := range backends {
		refs = append(refs, buildEndpointsForBackend(fmt.Sprintf("svc-%d", i), endpoints, 3, false))
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	t.Logf("backends=%d endpoints/backend=%d total_eps=%d heap_alloc_delta=%d bytes (%.1f bytes/endpoint)",
		backends, endpoints, backends*endpoints,
		after.HeapAlloc-before.HeapAlloc,
		float64(after.HeapAlloc-before.HeapAlloc)/float64(backends*endpoints))
	runtime.KeepAlive(refs)
}
