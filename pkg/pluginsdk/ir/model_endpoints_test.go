package ir

import (
	"fmt"
	"hash/fnv"
	"reflect"
	"slices"
	"strconv"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
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
		EndpointMd: EndpointMetadata{Labels: map[string]string{"i": strconv.Itoa(i)}},
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

func TestReuseEndpointUsesPrecomputedContribution(t *testing.T) {
	locality := PodLocality{Region: "r1", Zone: "z1"}
	base := NewEndpointsForBackend(epsBackend("svc"))
	base.Add(locality, testEndpointWithMd(1))
	endpoint := base.LbEps[locality][0]

	// Replace the cached value with a sentinel so this test distinguishes reuse
	// from another call to hashEndpoints. The cache is unexported derived state;
	// production values are populated only by Add.
	const sentinel = uint64(42)
	endpoint.endpointEqualityHash = sentinel

	rebuilt := NewEndpointsForBackend(epsBackend("svc"))
	rebuilt.ReuseEndpoint(locality, endpoint)
	if rebuilt.epsEqualityHash != sentinel {
		t.Fatalf("ReuseEndpoint rehashed the endpoint: got %d want %d", rebuilt.epsEqualityHash, sentinel)
	}
	if got := rebuilt.LbEps[locality][0].endpointEqualityHash; got != sentinel {
		t.Fatalf("cached contribution was not preserved: got %d want %d", got, sentinel)
	}
}

func TestFoldVersionSurvivesEndpointAdds(t *testing.T) {
	locality := PodLocality{Region: "r1", Zone: "z1"}
	endpoint := testEndpointWithMd(1)

	foldedBeforeAdd := NewEndpointsForBackend(epsBackend("svc"))
	foldedBeforeAdd.FoldVersion(11)
	foldedBeforeAdd.FoldVersion(22)
	foldedBeforeAdd.Add(locality, endpoint)

	foldedAfterAdd := NewEndpointsForBackend(epsBackend("svc"))
	foldedAfterAdd.Add(locality, endpoint)
	foldedAfterAdd.FoldVersion(11)
	foldedAfterAdd.FoldVersion(22)

	if foldedBeforeAdd.LbEpsEqualityHash != foldedAfterAdd.LbEpsEqualityHash {
		t.Fatalf(
			"Add erased or reordered folded versions: before-add=%d after-add=%d",
			foldedBeforeAdd.LbEpsEqualityHash,
			foldedAfterAdd.LbEpsEqualityHash,
		)
	}
	if !foldedBeforeAdd.hasFoldedVersionHash || foldedBeforeAdd.foldedVersionHash != hash(11, 22) {
		t.Fatalf("folded version state was not retained: has=%v hash=%d", foldedBeforeAdd.hasFoldedVersionHash, foldedBeforeAdd.foldedVersionHash)
	}

	emptyCopy := foldedBeforeAdd.EmptyCopy()
	emptyCopy.Add(locality, endpoint)
	if emptyCopy.LbEpsEqualityHash != foldedBeforeAdd.LbEpsEqualityHash {
		t.Fatalf(
			"EmptyCopy dropped folded versions: rebuilt=%d original=%d",
			emptyCopy.LbEpsEqualityHash,
			foldedBeforeAdd.LbEpsEqualityHash,
		)
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

// FoldVersion replaced a hand-rolled combiner in newFinalBackendEndpoints:
//
//	hasher := fnv.New64a()
//	utils.HashUint64(hasher, endpointHash)
//	utils.HashUint64(hasher, policyHash)
//
// LbEpsEqualityHash is the EDS resource version every connected Envoy already
// holds, so the conversion had to be bit-identical or the first control-plane
// restart after upgrade would re-push every CLA in the fleet at once. This pins
// that: FNV-1a is a streaming hash, so two 8-byte little-endian writes and one
// 16-byte write of the same bytes agree.
func TestFoldVersionMatchesTheReplacedCombiner(t *testing.T) {
	locality := PodLocality{Region: "r1", Zone: "z1"}
	const policyHash = uint64(0xfeedface)

	folded := NewEndpointsForBackend(epsBackend("svc"))
	folded.Add(locality, testEndpointWithMd(1))
	unfoldedHash := folded.LbEpsEqualityHash
	folded.FoldVersion(policyHash)

	hasher := fnv.New64a()
	utils.HashUint64(hasher, unfoldedHash)
	utils.HashUint64(hasher, policyHash)
	if want := hasher.Sum64(); folded.LbEpsEqualityHash != want {
		t.Fatalf("FoldVersion changed the published EDS version: got %d want %d", folded.LbEpsEqualityHash, want)
	}
}

// A clone is taken in order to be modified, so it must not carry the original's
// contribution hash: ReuseEndpoint would then publish changed endpoint content
// under the version Envoy already holds.
func TestCloneDropsCachedContributionHash(t *testing.T) {
	locality := PodLocality{Region: "r1", Zone: "z1"}
	base := NewEndpointsForBackend(epsBackend("svc"))
	base.Add(locality, testEndpointWithMd(1))
	original := base.LbEps[locality][0]
	if original.endpointEqualityHash == 0 {
		t.Fatal("Add did not cache a contribution hash")
	}

	modified := original.Clone()
	if modified.endpointEqualityHash != 0 {
		t.Errorf("Clone kept the cached contribution: got %d", modified.endpointEqualityHash)
	}
	modified.EndpointMd.Labels["i"] = "rewritten"
	modified.GetEndpoint().GetAddress().GetSocketAddress().Address = "10.9.9.9"

	if original.EndpointMd.Labels["i"] == "rewritten" {
		t.Error("Clone shares the label map with its source")
	}
	if original.GetEndpoint().GetAddress().GetSocketAddress().GetAddress() == "10.9.9.9" {
		t.Error("Clone shares the LbEndpoint proto with its source")
	}

	reusedOriginal := NewEndpointsForBackend(epsBackend("svc"))
	reusedOriginal.ReuseEndpoint(locality, original)
	reusedModified := NewEndpointsForBackend(epsBackend("svc"))
	reusedModified.ReuseEndpoint(locality, modified)
	if reusedOriginal.LbEpsEqualityHash == reusedModified.LbEpsEqualityHash {
		t.Errorf("different content versioned identically: %d", reusedOriginal.LbEpsEqualityHash)
	}

	// The recomputed value must match what Add would have produced.
	readded := NewEndpointsForBackend(epsBackend("svc"))
	readded.Add(locality, modified)
	if readded.LbEpsEqualityHash != reusedModified.LbEpsEqualityHash {
		t.Errorf("recomputed reuse hash differs from Add: reuse=%d add=%d", reusedModified.LbEpsEqualityHash, readded.LbEpsEqualityHash)
	}
}

// AdoptEndpointsFrom installs a set built elsewhere while leaving the
// receiver's own identity and folded version alone.
func TestAdoptEndpointsFromKeepsReceiverIdentity(t *testing.T) {
	locality := PodLocality{Region: "r1", Zone: "z1"}
	src := NewEndpointsForBackend(epsBackend("svc"))
	src.Add(locality, testEndpointWithMd(1))
	src.Add(locality, testEndpointWithMd(2))

	target := NewEndpointsForBackend(epsBackend("svc-variant"))
	target.FoldVersion(99)
	target.AdoptEndpointsFrom(src)

	readded := NewEndpointsForBackend(epsBackend("svc-variant"))
	readded.FoldVersion(99)
	for _, ep := range src.LbEps[locality] {
		readded.Add(locality, ep)
	}
	if target.LbEpsEqualityHash != readded.LbEpsEqualityHash {
		t.Errorf("adopt hash differs from re-Add: adopt=%d readd=%d", target.LbEpsEqualityHash, readded.LbEpsEqualityHash)
	}
	if target.UpstreamResourceName != readded.UpstreamResourceName {
		t.Errorf("adopt overwrote backend identity: got %q", target.UpstreamResourceName)
	}
	if target.endpointCount != 2 {
		t.Errorf("endpointCount not carried over: got %d", target.endpointCount)
	}

	// An empty source must reduce the receiver to its identity hash, not to
	// hash(0, upstreamHash).
	empty := NewEndpointsForBackend(epsBackend("svc"))
	target.AdoptEndpointsFrom(empty)
	wantEmpty := NewEndpointsForBackend(epsBackend("svc-variant"))
	wantEmpty.FoldVersion(99)
	if target.LbEpsEqualityHash != wantEmpty.LbEpsEqualityHash {
		t.Errorf("adopting an empty set: got %d want %d", target.LbEpsEqualityHash, wantEmpty.LbEpsEqualityHash)
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
		reflect.TypeFor[EndpointWithMd]():   {"LbEndpoint", "EndpointMd", "endpointEqualityHash"},
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
