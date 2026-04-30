package ir

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
)

func TestAttachedPolicyRefIDs(t *testing.T) {
	ref := AttachedPolicyRef{
		Group:     "gateway.kgateway.dev",
		Kind:      "TrafficPolicy",
		Namespace: "ns",
		Name:      "p",
	}
	assert.Equal(t, "gateway.kgateway.dev/TrafficPolicy/ns/p", ref.ID())
	assert.Equal(t, "gateway.kgateway.dev/TrafficPolicy/ns/p", ref.IDWithSectionName(),
		"IDWithSectionName equals ID when SectionName is empty")

	ref.SectionName = "rule-0"
	assert.Equal(t, "gateway.kgateway.dev/TrafficPolicy/ns/p", ref.ID(),
		"ID must not include SectionName so merge-tracking keys remain stable")
	assert.Equal(t, "gateway.kgateway.dev/TrafficPolicy/ns/p/rule-0", ref.IDWithSectionName(),
		"IDWithSectionName must include SectionName so distinct attachments are not collapsed")
}

func TestPolicyErrorRendering(t *testing.T) {
	tests := []struct {
		name string
		pe   *PolicyError
		want string
	}{
		{
			name: "ref with no section",
			pe: &PolicyError{
				Ref: &AttachedPolicyRef{
					Group:     "gateway.kgateway.dev",
					Kind:      "TrafficPolicy",
					Namespace: "ns1",
					Name:      "pol-a",
				},
				Err: errors.New("bad config"),
			},
			want: "gateway.kgateway.dev/TrafficPolicy/ns1/pol-a: bad config",
		},
		{
			name: "ref with section",
			pe: &PolicyError{
				Ref: &AttachedPolicyRef{
					Group:       "gateway.kgateway.dev",
					Kind:        "TrafficPolicy",
					Namespace:   "ns1",
					Name:        "pol-a",
					SectionName: "rule-0",
				},
				Err: errors.New("bad config"),
			},
			want: "gateway.kgateway.dev/TrafficPolicy/ns1/pol-a/rule-0: bad config",
		},
		{
			name: "nil ref falls through to bare error",
			pe:   &PolicyError{Ref: nil, Err: errors.New("bad config")},
			want: "bad config",
		},
		{
			name: "nil receiver renders empty",
			pe:   nil,
			want: "",
		},
		{
			name: "nil inner err renders empty",
			pe:   &PolicyError{Ref: nil, Err: nil},
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.pe.Error())
		})
	}
}

func TestPolicyErrorUnwrapPreservesSentinel(t *testing.T) {
	errSentinel := errors.New("sentinel")
	pe := &PolicyError{
		Ref: &AttachedPolicyRef{Kind: "TrafficPolicy", Name: "p"},
		Err: fmt.Errorf("wrapping: %w", errSentinel),
	}

	// errors.Is must walk through both the PolicyError wrapper and the
	// fmt.Errorf wrap to find the sentinel.
	assert.True(t, errors.Is(pe, errSentinel))

	// errors.As must extract the *PolicyError from a deeper wrap.
	var target *PolicyError
	wrapped := fmt.Errorf("outer: %w", pe)
	assert.True(t, errors.As(wrapped, &target))
	assert.Equal(t, "p", target.Ref.Name)
}

func TestWrapPolicyErrorsIsIdempotent(t *testing.T) {
	ref1 := &AttachedPolicyRef{Group: "g", Kind: "K", Namespace: "ns", Name: "p1"}
	ref2 := &AttachedPolicyRef{Group: "g", Kind: "K", Namespace: "ns", Name: "p2"}

	bare := errors.New("bare-error")
	preWrapped := &PolicyError{Ref: ref1, Err: errors.New("pre-wrapped")}

	out := WrapPolicyErrors(ref2, []error{bare, preWrapped})
	assert.Len(t, out, 2)

	// first entry: newly wrapped with ref2
	var pe0 *PolicyError
	ok := errors.As(out[0], &pe0)
	assert.True(t, ok, "first entry should be wrapped")
	assert.Equal(t, ref2, pe0.Ref)

	// second entry: returned as-is, ref1 preserved (not re-wrapped with ref2)
	var pe1 *PolicyError
	ok = errors.As(out[1], &pe1)
	assert.True(t, ok, "second entry should still be the original wrapper")
	assert.Equal(t, ref1, pe1.Ref, "idempotent wrap must not overwrite the existing ref")
}

func TestWrapPolicyErrorsHandlesEmptyAndNil(t *testing.T) {
	assert.Nil(t, WrapPolicyErrors(nil, nil))
	assert.Nil(t, WrapPolicyErrors(nil, []error{}))

	// nil entries inside the slice are dropped
	out := WrapPolicyErrors(nil, []error{nil, errors.New("x"), nil})
	assert.Len(t, out, 1)
}

func TestWrapPolicyErrorsWithNilRef(t *testing.T) {
	out := WrapPolicyErrors(nil, []error{errors.New("x")})
	assert.Len(t, out, 1)
	var pe *PolicyError
	ok := errors.As(out[0], &pe)
	assert.True(t, ok)
	assert.Nil(t, pe.Ref)
	assert.Equal(t, "x", pe.Error())
}

func TestPolicyApplyOrderedGroupKinds(t *testing.T) {
	fooGK := schema.GroupKind{Group: "foo", Kind: "bar"}
	barGK := schema.GroupKind{Group: "bar", Kind: "baz"}

	tests := []struct {
		name     string
		policies map[schema.GroupKind][]PolicyAtt
		assertFn func(*assert.Assertions, []schema.GroupKind)
	}{
		{
			name:     "1",
			policies: map[schema.GroupKind][]PolicyAtt{fooGK: {}, barGK: {}, VirtualBuiltInGK: {}},
			assertFn: func(a *assert.Assertions, got []schema.GroupKind) {
				a.Len(got, 3)
				a.Equal(got[0], VirtualBuiltInGK, "VirtualBuiltInGK should be first in the list")
			},
		},
		{
			name:     "2",
			policies: map[schema.GroupKind][]PolicyAtt{fooGK: {}, barGK: {}},
			assertFn: func(a *assert.Assertions, got []schema.GroupKind) {
				a.Len(got, 2)
				// either fooGK or barGK can be last as map's key iteration order is not deterministic
			},
		},
		{
			name:     "3",
			policies: map[schema.GroupKind][]PolicyAtt{barGK: {}, VirtualBuiltInGK: {}, fooGK: {}},
			assertFn: func(a *assert.Assertions, got []schema.GroupKind) {
				a.Len(got, 3)
				a.Equal(got[0], VirtualBuiltInGK, "VirtualBuiltInGK should be first in the list")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)
			ap := AttachedPolicies{Policies: tt.policies}
			got := ap.ApplyOrderedGroupKinds()
			tt.assertFn(a, got)
		})
	}
}

// Mock PolicyIR implementation for testing
type mockPolicyIR struct {
	// +noKrtEquals
	time   time.Time
	equals bool
}

func (m mockPolicyIR) CreationTime() time.Time {
	return m.time
}

func (m mockPolicyIR) Equals(other any) bool {
	return m.equals
}

func TestPolicyAttEquals(t *testing.T) {
	equalIR := mockPolicyIR{
		equals: true,
	}
	unequalIR := mockPolicyIR{
		equals: false,
	}

	testCases := []struct {
		name string
		a, b PolicyAtt
		want bool
	}{
		{
			name: "identical",
			a: PolicyAtt{
				GroupKind:               schema.GroupKind{Group: "test", Kind: "Policy"},
				Generation:              1,
				PolicyIr:                equalIR,
				PolicyRef:               nil,
				InheritedPolicyPriority: "",
				Errors:                  nil,
			},
			b: PolicyAtt{
				GroupKind:               schema.GroupKind{Group: "test", Kind: "Policy"},
				Generation:              1,
				PolicyIr:                equalIR,
				PolicyRef:               nil,
				InheritedPolicyPriority: "",
				Errors:                  nil,
			},
			want: true,
		},
		{
			name: "different GroupKind",
			a: PolicyAtt{
				GroupKind:               schema.GroupKind{Group: "test1", Kind: "Policy"},
				Generation:              1,
				PolicyIr:                equalIR,
				PolicyRef:               nil,
				InheritedPolicyPriority: "",
				Errors:                  nil,
			},
			b: PolicyAtt{
				GroupKind:               schema.GroupKind{Group: "test2", Kind: "Policy"},
				Generation:              1,
				PolicyIr:                equalIR,
				PolicyRef:               nil,
				InheritedPolicyPriority: "",
				Errors:                  nil,
			},
			want: false,
		},
		{
			name: "different Generation",
			a: PolicyAtt{
				GroupKind:               schema.GroupKind{Group: "test", Kind: "Policy"},
				Generation:              1,
				PolicyIr:                equalIR,
				PolicyRef:               nil,
				InheritedPolicyPriority: "",
				Errors:                  nil,
			},
			b: PolicyAtt{
				GroupKind:               schema.GroupKind{Group: "test", Kind: "Policy"},
				Generation:              2,
				PolicyIr:                equalIR,
				PolicyRef:               nil,
				InheritedPolicyPriority: "",
				Errors:                  nil,
			},
			want: false,
		},
		{
			name: "different PolicyIr",
			a: PolicyAtt{
				GroupKind:               schema.GroupKind{Group: "test", Kind: "Policy"},
				Generation:              1,
				PolicyIr:                unequalIR,
				PolicyRef:               nil,
				InheritedPolicyPriority: "",
				Errors:                  nil,
			},
			b: PolicyAtt{
				GroupKind:               schema.GroupKind{Group: "test", Kind: "Policy"},
				Generation:              1,
				PolicyIr:                equalIR,
				PolicyRef:               nil,
				InheritedPolicyPriority: "",
				Errors:                  nil,
			},
			want: false,
		},
		{
			name: "different PolicyRef",
			a: PolicyAtt{
				GroupKind:               schema.GroupKind{Group: "test", Kind: "Policy"},
				Generation:              1,
				PolicyIr:                equalIR,
				PolicyRef:               &AttachedPolicyRef{Group: "test", Kind: "Policy", Name: "policy1"},
				InheritedPolicyPriority: "",
				Errors:                  nil,
			},
			b: PolicyAtt{
				GroupKind:               schema.GroupKind{Group: "test", Kind: "Policy"},
				Generation:              1,
				PolicyIr:                equalIR,
				PolicyRef:               &AttachedPolicyRef{Group: "test", Kind: "Policy", Name: "policy2"},
				InheritedPolicyPriority: "",
				Errors:                  nil,
			},
			want: false,
		},
		{
			name: "different InheritedPolicyPriority",
			a: PolicyAtt{
				GroupKind:               schema.GroupKind{Group: "test", Kind: "Policy"},
				Generation:              1,
				PolicyIr:                equalIR,
				InheritedPolicyPriority: "",
				Errors:                  nil,
			},
			b: PolicyAtt{
				GroupKind:               schema.GroupKind{Group: "test", Kind: "Policy"},
				Generation:              1,
				PolicyIr:                equalIR,
				InheritedPolicyPriority: apiannotations.ShallowMergePreferParent,
				Errors:                  nil,
			},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := assert.New(t)

			got := tc.a.Equals(tc.b)
			a.Equal(tc.want, got)
		})
	}
}
