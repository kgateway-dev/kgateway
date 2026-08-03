package ir

import (
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	corev1 "k8s.io/api/core/v1"
)

// This file pins the invariant that policyHash really stands in for
// attachedPolicies on EndpointsForBackend, which is what lets the field carry a
// +noKrtEquals marker. Every semantic difference AttachedPolicies.Equals can see
// must also move VersionHash, and the derived LbEpsEqualityHash must not depend
// on the order in which endpoints and policies are supplied.

type hashTestPolicyIR struct {
	val string
	// ct only backs CreationTime; like the real policy IRs it is not part of
	// equality.
	// +noKrtEquals
	ct time.Time
}

func (f *hashTestPolicyIR) CreationTime() time.Time { return f.ct }
func (f *hashTestPolicyIR) Equals(in any) bool {
	other, ok := in.(*hashTestPolicyIR)
	return ok && f.val == other.val
}

// hashTestHashablePolicyIR additionally implements PolicyHashIR, the path used
// by policies (such as BackendTLSPolicy) whose content can change without the
// CR generation moving.
type hashTestHashablePolicyIR struct {
	hashTestPolicyIR
	h uint64
}

func (f *hashTestHashablePolicyIR) PolicyHash() uint64 { return f.h }

var _ PolicyHashIR = &hashTestHashablePolicyIR{}

func policyHashTestGK() schema.GroupKind {
	return schema.GroupKind{Group: "example.com", Kind: "MyPolicy"}
}

func basePolicyAttForHash() PolicyAtt {
	return PolicyAtt{
		GroupKind:  policyHashTestGK(),
		Generation: 1,
		PolicyIr:   &hashTestPolicyIR{val: "base"},
		PolicyRef: &AttachedPolicyRef{
			Group:     "example.com",
			Kind:      "MyPolicy",
			Name:      "my-policy",
			Namespace: "default",
		},
		Errors: []error{errors.New("base error")},
	}
}

func baseAttachedPoliciesForHash() AttachedPolicies {
	return AttachedPolicies{
		Policies: map[schema.GroupKind][]PolicyAtt{
			policyHashTestGK(): {basePolicyAttForHash()},
		},
	}
}

// TestAttachedPoliciesVersionHashStandsInForEquals is the "B really stands in
// for A" check: for every mutation that makes Equals report a difference,
// VersionHash must differ too. A hash that missed one of these would let a
// policy change reach CDS without moving the EDS version.
func TestAttachedPoliciesVersionHashStandsInForEquals(t *testing.T) {
	cases := []struct {
		field  string
		mutate func(*AttachedPolicies)
	}{
		{
			field: "Policies/added GroupKind",
			mutate: func(a *AttachedPolicies) {
				a.Policies[schema.GroupKind{Group: "other.io", Kind: "OtherPolicy"}] = []PolicyAtt{basePolicyAttForHash()}
			},
		},
		{
			field: "Policies/removed GroupKind",
			mutate: func(a *AttachedPolicies) {
				delete(a.Policies, policyHashTestGK())
			},
		},
		{
			field: "Policies/added second policy under same GroupKind",
			mutate: func(a *AttachedPolicies) {
				a.Policies[policyHashTestGK()] = append(a.Policies[policyHashTestGK()], basePolicyAttForHash())
			},
		},
		{
			field: "PolicyAtt.Generation",
			mutate: func(a *AttachedPolicies) {
				p := basePolicyAttForHash()
				p.Generation = 99
				a.Policies[policyHashTestGK()] = []PolicyAtt{p}
			},
		},
		{
			field: "PolicyAtt.PolicyRef",
			mutate: func(a *AttachedPolicies) {
				p := basePolicyAttForHash()
				p.PolicyRef = &AttachedPolicyRef{
					Group: "example.com", Kind: "MyPolicy", Name: "other-policy", Namespace: "default",
				}
				a.Policies[policyHashTestGK()] = []PolicyAtt{p}
			},
		},
		{
			field: "PolicyAtt.Errors",
			mutate: func(a *AttachedPolicies) {
				p := basePolicyAttForHash()
				p.Errors = []error{errors.New("a different error")}
				a.Policies[policyHashTestGK()] = []PolicyAtt{p}
			},
		},
		{
			field: "PolicyAtt.PolicyIr via PolicyHashIR",
			mutate: func(a *AttachedPolicies) {
				p := basePolicyAttForHash()
				p.PolicyIr = &hashTestHashablePolicyIR{h: 12345}
				a.Policies[policyHashTestGK()] = []PolicyAtt{p}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			base := baseAttachedPoliciesForHash()
			mutated := baseAttachedPoliciesForHash()
			tc.mutate(&mutated)

			if base.Equals(mutated) {
				t.Fatalf("test bug: mutation of %q did not make AttachedPolicies.Equals report a difference", tc.field)
			}
			if base.VersionHash() == mutated.VersionHash() {
				t.Errorf("VersionHash collided after mutating %s; the hash must move whenever Equals reports a difference", tc.field)
			}
		})
	}
}

// TestAttachedPoliciesVersionHashStable guards the two directions that must NOT
// change the hash: recomputation, and Go's randomized map iteration order.
func TestAttachedPoliciesVersionHashStable(t *testing.T) {
	t.Run("empty is zero", func(t *testing.T) {
		var empty AttachedPolicies
		if got := empty.VersionHash(); got != 0 {
			t.Errorf("VersionHash of empty AttachedPolicies = %d, want 0 so it contributes nothing downstream", got)
		}
	})

	t.Run("stable across map iteration order", func(t *testing.T) {
		build := func() AttachedPolicies {
			a := AttachedPolicies{Policies: map[schema.GroupKind][]PolicyAtt{}}
			for _, gk := range []schema.GroupKind{
				{Group: "a.io", Kind: "A"}, {Group: "b.io", Kind: "B"},
				{Group: "c.io", Kind: "C"}, {Group: "d.io", Kind: "D"},
			} {
				p := basePolicyAttForHash()
				p.GroupKind = gk
				a.Policies[gk] = []PolicyAtt{p}
			}
			return a
		}
		want := build().VersionHash()
		// Rebuild repeatedly: each map gets a fresh randomized iteration order.
		for i := range 50 {
			if got := build().VersionHash(); got != want {
				t.Fatalf("VersionHash is not stable across map iteration order (iteration %d): got %d, want %d", i, got, want)
			}
		}
	})
}

func backendForHashTest(t *testing.T) BackendObjectIR {
	t.Helper()
	b := NewBackendObjectIR(ObjectSource{
		Kind: "Service", Namespace: "default", Name: "my-service",
	}, 8080, "", "")
	b.Obj = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "my-service", Namespace: "default", ResourceVersion: "1"},
	}
	return b
}

func endpointForHashTest() EndpointWithMd {
	return EndpointWithMd{EndpointMd: EndpointMetadata{Labels: map[string]string{"zone": "a"}}}
}

// TestLbEpsEqualityHashIsOrderIndependent is the regression this refactor buys.
// LbEpsEqualityHash is derived on read, so a policy contribution can no longer
// be clobbered by a later Add, regardless of the order the two mutators run in.
func TestLbEpsEqualityHashIsOrderIndependent(t *testing.T) {
	loc := PodLocality{Region: "region", Zone: "zone"}
	policies := baseAttachedPoliciesForHash()

	// Policies first, then endpoints.
	a := NewEndpointsForBackend(backendForHashTest(t))
	a.SetAttachedPolicies(policies)
	a.Add(loc, endpointForHashTest())

	// Endpoints first, then policies.
	b := NewEndpointsForBackend(backendForHashTest(t))
	b.Add(loc, endpointForHashTest())
	b.SetAttachedPolicies(policies)

	if a.LbEpsEqualityHash() != b.LbEpsEqualityHash() {
		t.Errorf("LbEpsEqualityHash depends on mutator order: policies-then-endpoints = %d, endpoints-then-policies = %d",
			a.LbEpsEqualityHash(), b.LbEpsEqualityHash())
	}
	if !a.Equals(*b) {
		t.Error("Equals depends on mutator order; the two builds must be indistinguishable")
	}
}

// TestLbEpsEqualityHashTracksPolicies checks that policy content reaches the EDS
// version at all, and that EmptyCopy carries the pair forward.
func TestLbEpsEqualityHashTracksPolicies(t *testing.T) {
	loc := PodLocality{Region: "region", Zone: "zone"}

	withoutPolicies := NewEndpointsForBackend(backendForHashTest(t))
	withoutPolicies.Add(loc, endpointForHashTest())

	withPolicies := NewEndpointsForBackend(backendForHashTest(t))
	withPolicies.Add(loc, endpointForHashTest())
	withPolicies.SetAttachedPolicies(baseAttachedPoliciesForHash())

	if withoutPolicies.LbEpsEqualityHash() == withPolicies.LbEpsEqualityHash() {
		t.Error("attaching a policy did not move LbEpsEqualityHash; Envoy would not receive a fresh CLA")
	}
	if withoutPolicies.Equals(*withPolicies) {
		t.Error("Equals did not detect the attached policy; the KRT event would be suppressed")
	}

	t.Run("changing policy content moves the hash", func(t *testing.T) {
		changed := NewEndpointsForBackend(backendForHashTest(t))
		changed.Add(loc, endpointForHashTest())
		bumped := baseAttachedPoliciesForHash()
		bumped.Policies[policyHashTestGK()][0].Generation = 2
		changed.SetAttachedPolicies(bumped)

		if changed.LbEpsEqualityHash() == withPolicies.LbEpsEqualityHash() {
			t.Error("bumping the policy generation did not move LbEpsEqualityHash")
		}
	})

	t.Run("EmptyCopy carries the policy pair", func(t *testing.T) {
		cp := withPolicies.EmptyCopy()
		if !cp.AttachedPolicies().Equals(withPolicies.AttachedPolicies()) {
			t.Error("EmptyCopy dropped attachedPolicies")
		}
		// No endpoints on the copy, so the endpoint contribution is absent, but
		// the policy contribution must survive.
		bare := NewEndpointsForBackend(backendForHashTest(t))
		if cp.LbEpsEqualityHash() == bare.LbEpsEqualityHash() {
			t.Error("EmptyCopy dropped the policy contribution to LbEpsEqualityHash")
		}
	})
}

// TestLbEpsEqualityHashMatchesLegacyComposition pins the derived value to the
// composition the stored field used to produce, so this refactor does not churn
// every EDS version on rollout.
func TestLbEpsEqualityHashMatchesLegacyComposition(t *testing.T) {
	loc := PodLocality{Region: "region", Zone: "zone"}

	t.Run("no endpoints, no policies", func(t *testing.T) {
		e := NewEndpointsForBackend(backendForHashTest(t))
		if got, want := e.LbEpsEqualityHash(), e.upstreamHash; got != want {
			t.Errorf("bare backend hash = %d, want upstreamHash %d", got, want)
		}
	})

	t.Run("endpoints, no policies", func(t *testing.T) {
		e := NewEndpointsForBackend(backendForHashTest(t))
		e.Add(loc, endpointForHashTest())
		if got, want := e.LbEpsEqualityHash(), hash(e.epsEqualityHash, e.upstreamHash); got != want {
			t.Errorf("hash = %d, want hash(epsEqualityHash, upstreamHash) = %d", got, want)
		}
	})

	t.Run("endpoints and policies", func(t *testing.T) {
		e := NewEndpointsForBackend(backendForHashTest(t))
		e.Add(loc, endpointForHashTest())
		e.SetAttachedPolicies(baseAttachedPoliciesForHash())
		want := hash(hash(e.epsEqualityHash, e.upstreamHash), baseAttachedPoliciesForHash().VersionHash())
		if got := e.LbEpsEqualityHash(); got != want {
			t.Errorf("hash = %d, want combine(hash(eps, upstream), policyHash) = %d", got, want)
		}
	})
}
