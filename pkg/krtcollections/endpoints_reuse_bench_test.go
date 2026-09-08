package krtcollections

import (
	"fmt"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func reuseBenchBackend(name string) ir.BackendObjectIR {
	return ir.NewBackendObjectIR(ir.ObjectSource{
		Group:     "",
		Kind:      "Service",
		Namespace: "default",
		Name:      name,
	}, 80, "", "")
}

// reuseBenchBuildEndpoints simulates transformK8sEndpoints for a backend with
// `endpoints` ready endpoints spread across `localities` zones.
func reuseBenchBuildEndpoints(name string, endpoints, localities int) *ir.EndpointsForBackend {
	ret := ir.NewEndpointsForBackend(reuseBenchBackend(name))
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
		ep := CreateLBEndpoint(fmt.Sprintf("10.0.%d.%d", i/256, i%256), 8080, labels, false)
		ret.Add(loc, ir.EndpointWithMd{
			LbEndpoint: ep,
			EndpointMd: ir.EndpointMetadata{Labels: labels},
		})
	}
	return ret
}

// BenchmarkCopyEndpointsForBackend compares copying an EndpointsForBackend to a
// variant/final copy by re-hashing every endpoint (old effective_endpoints /
// gateway_backend_variants behavior) vs reusing the precomputed hashes.
func BenchmarkCopyEndpointsForBackend(b *testing.B) {
	for _, endpoints := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("endpoints=%d", endpoints), func(b *testing.B) {
			base := reuseBenchBuildEndpoints("svc", endpoints, 3)
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
