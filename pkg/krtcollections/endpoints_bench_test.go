package krtcollections

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func testBackend(name string) ir.BackendObjectIR {
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
	ret := ir.NewEndpointsForBackend(testBackend(name))
	labels := map[string]string{
		"app":                         "demo",
		"security.istio.io/tlsMode":   "istio",
		"topology.kubernetes.io/zone": "us-east-1a",
	}
	for i := 0; i < endpoints; i++ {
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

// BenchmarkCopyEndpointsForBackend compares copying an EndpointsForBackend to a
// variant/final copy by re-hashing every endpoint (old effective_endpoints /
// gateway_backend_variants behavior) vs reusing the precomputed hashes.
func BenchmarkCopyEndpointsForBackend(b *testing.B) {
	for _, endpoints := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("endpoints=%d", endpoints), func(b *testing.B) {
			base := buildEndpointsForBackend("svc", endpoints, 3, false)
			b.Run("rehash_add", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					clone := base.EmptyCopy()
					for locality, eps := range base.LbEps {
						for _, ep := range eps {
							clone.Add(locality, ep)
						}
					}
					if clone.LbEpsEqualityHash != base.LbEpsEqualityHash {
						b.Fatal("hash mismatch")
					}
				}
			})
			b.Run("reuse_hashes", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					clone := base.EmptyCopy()
					clone.ReuseEndpointsFrom(base)
					if clone.LbEpsEqualityHash != base.LbEpsEqualityHash {
						b.Fatal("hash mismatch")
					}
				}
			})
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
	for i := 0; i < backends; i++ {
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
