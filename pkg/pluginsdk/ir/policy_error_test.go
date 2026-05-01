package ir

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestWrapPolicyErrorsExpandsJoinedInputs(t *testing.T) {
	// A single Errors entry can be a multi-error (errors.Join). Each err must
	// get its own *PolicyError so attribution renders on every line, not just
	// the first.
	ref := &AttachedPolicyRef{Group: "g", Kind: "K", Namespace: "ns", Name: "p"}
	a := errors.New("a")
	b := errors.New("b")

	out := WrapPolicyErrors(ref, []error{errors.Join(a, b)})
	require.Len(t, out, 2)

	for _, e := range out {
		var pe *PolicyError
		require.True(t, errors.As(e, &pe))
		require.Equal(t, ref, pe.Ref)
	}
	assert.Equal(t, "g/K/ns/p: a", out[0].Error())
	assert.Equal(t, "g/K/ns/p: b", out[1].Error())
}

func TestFlattenJoinedErr(t *testing.T) {
	// nil → nil
	assert.Nil(t, FlattenJoinedErr(nil))

	// single err → single-element slice with same pointer
	err := errors.New("err")
	got := FlattenJoinedErr(err)
	assert.Len(t, got, 1)
	assert.Same(t, err, got[0])

	// nested errors.Join trees flatten to errs only
	a := errors.New("a")
	b := errors.New("b")
	c := errors.New("c")
	nested := errors.Join(errors.Join(a, b), c)
	got = FlattenJoinedErr(nested)
	assert.Equal(t, []error{a, b, c}, got)
}
