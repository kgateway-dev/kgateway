package ir

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"
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

// reuseCmpOpts compares two EndpointsForBackend in full, including the unexported
// equality hashes.
//
// AttachedPolicies is ignored rather than compared: it is +noKrtEquals,
// ReuseEndpointsFrom does not touch it, and cmp cannot traverse it — PolicyAtt
// holds a PolicyIR interface whose concrete plugin types have unexported fields,
// so cmp panics ("cannot handle unexported field") as soon as a fixture attaches
// a real policy. There is no finite set of types to pass to AllowUnexported.
var reuseCmpOpts = []cmp.Option{
	cmp.AllowUnexported(EndpointsForBackend{}),
	cmpopts.IgnoreFields(EndpointsForBackend{}, "AttachedPolicies"),
	protocmp.Transform(),
}

// TestReuseEndpointsFromCopiesEveryDerivedField compares the whole struct against
// one built by re-Adding every endpoint. Unlike an assertion on the hashes alone,
// this fails when a future field is maintained incrementally by Add() and
// ReuseEndpointsFrom forgets to reproduce it, and when ReuseEndpointsFrom starts
// copying a backend-identity field it should have left to the caller.
func TestReuseEndpointsFromCopiesEveryDerivedField(t *testing.T) {
	base := NewEndpointsForBackend(epsBackend("svc"))
	for i := range 10 {
		base.Add(
			PodLocality{
				Region: "us-east-1",
				Zone:   fmt.Sprintf("zone-%d", i%3),
			},
			testEndpointWithMd(i),
		)
	}

	readded := NewEndpointsForBackend(epsBackend("svc-variant"))
	for locality, endpoints := range base.LbEps {
		for _, endpoint := range endpoints {
			readded.Add(locality, endpoint)
		}
	}

	clone := NewEndpointsForBackend(epsBackend("svc-variant"))
	clone.ReuseEndpointsFrom(base)

	if diff := cmp.Diff(readded, clone, reuseCmpOpts...); diff != "" {
		t.Fatalf("ReuseEndpointsFrom differs from re-Add (-want +got):\n%s", diff)
	}

	// Equal contents do not prove that each locality has its own backing slice,
	// so assert non-aliasing separately.
	for locality, baseEndpoints := range base.LbEps {
		clonedEndpoints := clone.LbEps[locality]
		if len(baseEndpoints) > 0 && len(clonedEndpoints) > 0 && &baseEndpoints[0] == &clonedEndpoints[0] {
			t.Errorf("endpoint slice for %v aliases the base backing array", locality)
		}
	}
}

// TestReuseEndpointsFromCopiesEmptyLocality covers the len(eps) > 0 == false
// branch, which needs a locality key with no endpoints.
//
// Such a locality is unreachable in production: Add is the only writer of LbEps
// anywhere in the tree, and it always appends an endpoint. It is also the one
// input where reuse and re-Add disagree on state — the re-Add loop never calls
// Add for a zero-length slice, so it drops the key, while reuse preserves it. The
// equality hash agrees either way, and the hash is what Equals compares, so this
// documents the divergence rather than asserting equivalence.
func TestReuseEndpointsFromCopiesEmptyLocality(t *testing.T) {
	base := NewEndpointsForBackend(epsBackend("svc"))
	emptyLocality := PodLocality{Region: "empty-region"}
	base.LbEps[emptyLocality] = nil

	clone := NewEndpointsForBackend(epsBackend("svc-variant"))
	initialHash := clone.LbEpsEqualityHash
	clone.ReuseEndpointsFrom(base)

	if _, ok := clone.LbEps[emptyLocality]; !ok {
		t.Error("empty locality was not copied")
	}
	if clone.LbEpsEqualityHash != initialHash {
		t.Errorf(
			"empty endpoint set changed hash: got %d want %d",
			clone.LbEpsEqualityHash,
			initialHash,
		)
	}
}

// TestEndpointHashInputsUnchanged is a canary on the structs hashEndpoints reads.
// EndpointsForBackend.Equals compares equality hashes instead of endpoints, so a
// new field on any of these is invisible to KRT until hashEndpoints folds it in.
// This cannot prove an existing field is actually hashed; it forces a conscious
// decision when one of these structs grows.
func TestEndpointHashInputsUnchanged(t *testing.T) {
	for ty, want := range map[reflect.Type][]string{
		reflect.TypeFor[PodLocality]():      {"Region", "Zone", "Subzone"},
		reflect.TypeFor[EndpointWithMd]():   {"LbEndpoint", "EndpointMd"},
		reflect.TypeFor[EndpointMetadata](): {"Labels"},
	} {
		var got []string
		for field := range ty.Fields() {
			got = append(got, field.Name)
		}
		if !slices.Equal(got, want) {
			t.Errorf(
				"%s fields changed: got %v want %v; update hashEndpoints or explicitly document the exclusion",
				ty.Name(), got, want,
			)
		}
	}
}
