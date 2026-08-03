package ir

import (
	"fmt"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
)

func epsBackend(name string) BackendObjectIR {
	return NewBackendObjectIR(ObjectSource{
		Group:     "",
		Kind:      "Service",
		Namespace: "default",
		Name:      name,
	}, 80, "", "")
}

// testLbEndpoint builds a minimal non-nil LbEndpoint so hashEndpoints exercises
// the proto-marshaling path (HashProtoWithHasher) used in production.
func testLbEndpoint(addr string, port uint32) *envoyendpointv3.LbEndpoint {
	return &envoyendpointv3.LbEndpoint{
		HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
			Endpoint: &envoyendpointv3.Endpoint{
				Address: &envoycorev3.Address{
					Address: &envoycorev3.Address_SocketAddress{
						SocketAddress: &envoycorev3.SocketAddress{
							Address: addr,
							PortSpecifier: &envoycorev3.SocketAddress_PortValue{
								PortValue: port,
							},
						},
					},
				},
			},
		},
	}
}

func testEndpointWithMd(i int) EndpointWithMd {
	return EndpointWithMd{
		LbEndpoint: testLbEndpoint(fmt.Sprintf("10.0.%d.%d", i/256, i%256), 8080),
		EndpointMd: EndpointMetadata{Labels: map[string]string{"i": fmt.Sprint(i)}},
	}
}

func TestReuseEndpointsFromMatchesReAdd(t *testing.T) {
	base := NewEndpointsForBackend(epsBackend("svc"))
	for i := range 50 {
		loc := PodLocality{Region: "us-east-1", Zone: fmt.Sprintf("zone-%d", i%3)}
		base.Add(loc, testEndpointWithMd(i))
	}

	// reference: rebuild by re-Adding every endpoint
	readded := NewEndpointsForBackend(epsBackend("svc-variant"))
	for loc, eps := range base.LbEps {
		for _, ep := range eps {
			readded.Add(loc, ep)
		}
	}

	clone := NewEndpointsForBackend(epsBackend("svc-variant"))
	clone.ReuseEndpointsFrom(base)

	if clone.LbEpsEqualityHash != readded.LbEpsEqualityHash {
		t.Fatalf("LbEpsEqualityHash mismatch: reuse=%d readd=%d", clone.LbEpsEqualityHash, readded.LbEpsEqualityHash)
	}
	if clone.LbEpsEqualityHash == base.LbEpsEqualityHash {
		t.Fatal("variant hash should differ from base due to different upstream identity")
	}
	if len(clone.LbEps) != len(base.LbEps) {
		t.Fatalf("locality count mismatch: got %d want %d", len(clone.LbEps), len(base.LbEps))
	}
	for loc, eps := range base.LbEps {
		got := clone.LbEps[loc]
		if len(got) != len(eps) {
			t.Fatalf("endpoint count mismatch in %v: got %d want %d", loc, len(got), len(eps))
		}
		// verify slice was cloned (no aliasing of backing arrays)
		if len(got) > 0 && &got[0] == &eps[0] {
			t.Fatalf("endpoint slice for %v aliases base backing array", loc)
		}
	}
}

func TestReuseEndpointsFromDropsStaleLocalities(t *testing.T) {
	base := NewEndpointsForBackend(epsBackend("svc"))
	base.Add(PodLocality{Region: "r1", Zone: "z1"}, testEndpointWithMd(0))

	clone := NewEndpointsForBackend(epsBackend("svc-variant"))
	clone.Add(PodLocality{Region: "r9", Zone: "z9"}, testEndpointWithMd(1))
	clone.ReuseEndpointsFrom(base)

	if _, ok := clone.LbEps[PodLocality{Region: "r9", Zone: "z9"}]; ok {
		t.Fatal("stale locality r9/z9 should have been dropped")
	}
	if len(clone.LbEps) != 1 {
		t.Fatalf("expected 1 locality, got %d", len(clone.LbEps))
	}
}

func TestReuseEndpointsFromNonEmptyZeroHash(t *testing.T) {
	// two identical (locality, endpoint) entries xor to a zero epsEqualityHash;
	// a re-Add still produces hash(0, upstreamHash), so reuse must too.
	base := NewEndpointsForBackend(epsBackend("svc"))
	loc := PodLocality{Region: "us-east-1", Zone: "zone-a"}
	emd := testEndpointWithMd(1)
	base.Add(loc, emd)
	base.Add(loc, emd)
	if base.epsEqualityHash != 0 {
		t.Fatalf("expected xor of identical endpoints to be 0, got %d", base.epsEqualityHash)
	}

	readded := NewEndpointsForBackend(epsBackend("svc-variant"))
	readded.Add(loc, emd)
	readded.Add(loc, emd)

	clone := NewEndpointsForBackend(epsBackend("svc-variant"))
	clone.ReuseEndpointsFrom(base)
	if clone.LbEpsEqualityHash != readded.LbEpsEqualityHash {
		t.Fatalf("zero-hash non-empty reuse mismatch: reuse=%d readd=%d", clone.LbEpsEqualityHash, readded.LbEpsEqualityHash)
	}
}

func TestReuseEndpointsFromEmpty(t *testing.T) {
	base := NewEndpointsForBackend(epsBackend("svc"))
	clone := NewEndpointsForBackend(epsBackend("svc-variant"))
	clone.ReuseEndpointsFrom(base)
	readded := NewEndpointsForBackend(epsBackend("svc-variant"))
	if clone.LbEpsEqualityHash != readded.LbEpsEqualityHash {
		t.Fatalf("empty reuse hash mismatch: reuse=%d readd=%d", clone.LbEpsEqualityHash, readded.LbEpsEqualityHash)
	}
}
