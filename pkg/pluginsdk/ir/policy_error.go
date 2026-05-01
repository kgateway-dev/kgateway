package ir

import (
	"errors"
)

// PolicyError wraps a policy IR construction error to its source policy.
type PolicyError struct {
	// Ref is the source policy that produced Err. May be nil if the error
	// originated from a synthesized PolicyAtt with no associated CR.
	Ref *AttachedPolicyRef
	Err error
}

func (p *PolicyError) Error() string {
	if p == nil || p.Err == nil {
		return ""
	}
	if p.Ref == nil {
		return p.Err.Error()
	}
	return p.Ref.IDWithSectionName() + ": " + p.Err.Error()
}

func (p *PolicyError) Unwrap() error {
	if p == nil {
		return nil
	}
	return p.Err
}

// WrapPolicyErrors wraps each leaf of errs in *PolicyError keyed to ref.
// Multi-errors (errors.Join) are flattened first so each leaf is attributed.
// Already-wrapped entries pass through unchanged, making the call idempotent.
func WrapPolicyErrors(ref *AttachedPolicyRef, errs []error) []error {
	if len(errs) == 0 {
		return nil
	}
	out := make([]error, 0, len(errs))
	for _, e := range errs {
		for _, leaf := range FlattenJoinedErr(e) {
			var pe *PolicyError
			if errors.As(leaf, &pe) {
				out = append(out, leaf)
				continue
			}
			out = append(out, &PolicyError{Ref: ref, Err: leaf})
		}
	}
	return out
}

// FlattenJoinedErr walks nested errors.Join (and multi-%w fmt.Errorf) trees
// down to leaf errors. errors.Join returns a value that implements
// interface{ Unwrap() []error }; we recurse through any such node.
func FlattenJoinedErr(err error) []error {
	type unwrapMulti interface{ Unwrap() []error }
	var out []error
	var walk func(error)
	walk = func(e error) {
		if e == nil {
			return
		}
		if u, ok := e.(unwrapMulti); ok {
			for _, child := range u.Unwrap() {
				walk(child)
			}
			return
		}
		out = append(out, e)
	}
	walk(err)
	return out
}
