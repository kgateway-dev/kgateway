package ir

import (
	"fmt"
	"testing"
)

func epsBackend(name string) BackendObjectIR {
	return NewBackendObjectIR(ObjectSource{
		Group:     "",
		Kind:      "Service",
		Namespace: "default",
		Name:      name,
	}, 80, "", "")
}

func TestReuseEndpointsFromMatchesReAdd(t *testing.T) {
	base := NewEndpointsForBackend(epsBackend("svc"))
	for i := 0; i < 50; i++ {
		loc := PodLocality{Region: "us-east-1", Zone: fmt.Sprintf("zone-%d", i%3)}
		base.Add(loc, EndpointWithMd{
			EndpointMd: EndpointMetadata{Labels: map[string]string{"i": fmt.Sprint(i)}},
		})
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

func TestReuseEndpointsFromNonEmptyZeroHash(t *testing.T) {
	// two identical (locality, endpoint) entries xor to a zero epsEqualityHash;
	// a re-Add still produces hash(0, upstreamHash), so reuse must too.
	base := NewEndpointsForBackend(epsBackend("svc"))
	loc := PodLocality{Region: "us-east-1", Zone: "zone-a"}
	emd := EndpointWithMd{EndpointMd: EndpointMetadata{Labels: map[string]string{"i": "1"}}}
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
