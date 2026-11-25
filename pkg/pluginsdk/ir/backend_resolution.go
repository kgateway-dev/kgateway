package ir

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DroppedReason says why a field a policy sets does not take effect on a backend.
type DroppedReason string

const (
	// DroppedByMerge means another policy of the same kind supplies the field, by higher
	// precedence weight or, at equal weight, by being older.
	DroppedByMerge DroppedReason = "Merge"
	// DroppedBySupersede means a policy of another kind owns the field on this backend, as
	// that kind declares through PolicyPlugin.Supersedes.
	DroppedBySupersede DroppedReason = "Supersede"
)

// PolicyWinner is the policy that supplies a field on a backend.
type PolicyWinner struct {
	// Kind is the winning policy's kind.
	Kind schema.GroupKind
	// By is the winning policy. Nil when the winner is a merged policy with no single source.
	By *AttachedPolicyRef
	// ByValid is false when By has errors. A backend whose winning policy is invalid gets a
	// blackhole cluster rather than any policy's field.
	ByValid bool
}

// DroppedField is one field a policy sets that does not take effect on a backend, and the
// policy that supplies it instead.
type DroppedField struct {
	// Field is the field's MergeOrigins key, e.g. "tls".
	Field string
	PolicyWinner
	Reason DroppedReason
}

// BackendPolicyResolution is the single decision of what each attached policy contributes
// to a backend. It is computed once, when policies attach to the backend, and everything
// downstream reads it: backend translation applies Applied, and status reports Dropped. No
// consumer merges attached policies again, so what is applied and what is reported cannot
// disagree.
//
// A backend usually has few policy kinds and few policies, so the decision is held in
// slices that are searched linearly and allocated only when there is something to record.
// A backend with no policies shares one empty resolution and allocates nothing.
type BackendPolicyResolution struct {
	// applied holds, per policy kind, what backend translation applies.
	applied []appliedKind
	// superseded holds the fields of a kind that another attached kind owns.
	superseded []supersededField
	// dropped holds, per attachment, the fields it sets that do not take effect.
	dropped []droppedFields
}

type appliedKind struct {
	gk schema.GroupKind
	// merged is the single merged policy of a kind that merges; all is nil then.
	merged [1]PolicyAtt
	// all holds every attachment of a kind that does not merge. It shares the attached
	// slice, which is never modified.
	all []PolicyAtt
}

type supersededField struct {
	gk    schema.GroupKind
	field string
	owner PolicyWinner
}

type droppedFields struct {
	// id is the attachment's AttachedPolicyRef.IDWithSectionName.
	id     string
	fields []DroppedField
}

// emptyResolution is the resolution of every backend with no policies attached. It is
// shared, so nothing may modify it; the setters refuse to.
var emptyResolution = &BackendPolicyResolution{}

// EmptyBackendPolicyResolution returns the shared resolution of a backend with no policies.
// It is not nil, so it tells a resolved backend from one that never went through attachment.
func EmptyBackendPolicyResolution() *BackendPolicyResolution {
	return emptyResolution
}

// NewBackendPolicyResolution returns an empty resolution for the resolver to fill, with room
// for kinds policy kinds.
func NewBackendPolicyResolution(kinds int) *BackendPolicyResolution {
	return &BackendPolicyResolution{applied: make([]appliedKind, 0, kinds)}
}

// Applied returns what backend translation applies for policy kind gk: a single merged
// policy for kinds that merge, every attachment otherwise, nil when none is attached. The
// returned slice must not be modified.
func (r *BackendPolicyResolution) Applied(gk schema.GroupKind) []PolicyAtt {
	if r == nil {
		return nil
	}
	for i := range r.applied {
		if r.applied[i].gk == gk {
			if r.applied[i].all != nil {
				return r.applied[i].all
			}
			return r.applied[i].merged[:]
		}
	}
	return nil
}

// Effective returns the single policy translation applies for a kind that merges, and
// false when none is attached or the kind does not merge.
func (r *BackendPolicyResolution) Effective(gk schema.GroupKind) (PolicyAtt, bool) {
	applied := r.Applied(gk)
	if len(applied) != 1 {
		return PolicyAtt{}, false
	}
	return applied[0], true
}

// IsSuperseded reports whether another attached policy kind owns field of kind gk.
func (r *BackendPolicyResolution) IsSuperseded(gk schema.GroupKind, field string) bool {
	_, ok := r.SupersededBy(gk, field)
	return ok
}

// HasSuperseded reports whether another attached policy kind owns any field of kind gk.
func (r *BackendPolicyResolution) HasSuperseded(gk schema.GroupKind) bool {
	if r == nil {
		return false
	}
	for _, s := range r.superseded {
		if s.gk == gk {
			return true
		}
	}
	return false
}

// SupersededBy returns the policy kind that owns field of kind gk, if any.
func (r *BackendPolicyResolution) SupersededBy(gk schema.GroupKind, field string) (PolicyWinner, bool) {
	if r == nil {
		return PolicyWinner{}, false
	}
	for _, s := range r.superseded {
		if s.gk == gk && s.field == field {
			return s.owner, true
		}
	}
	return PolicyWinner{}, false
}

// Dropped returns the fields the attachment ref sets that do not take effect, in the order
// the resolver found them. The returned slice must not be modified.
func (r *BackendPolicyResolution) Dropped(ref *AttachedPolicyRef) []DroppedField {
	if r == nil || ref == nil || len(r.dropped) == 0 {
		return nil
	}
	id := ref.IDWithSectionName()
	for _, d := range r.dropped {
		if d.id == id {
			return d.fields
		}
	}
	return nil
}

// SetMerged records the merged policy translation applies for a kind that merges. For the
// resolver only.
func (r *BackendPolicyResolution) SetMerged(gk schema.GroupKind, merged PolicyAtt) {
	r.mustBeMutable()
	r.applied = append(r.applied, appliedKind{gk: gk, merged: [1]PolicyAtt{merged}})
}

// SetAll records that translation applies every attachment of a kind that does not merge.
// all is shared, not copied. For the resolver only.
func (r *BackendPolicyResolution) SetAll(gk schema.GroupKind, all []PolicyAtt) {
	r.mustBeMutable()
	r.applied = append(r.applied, appliedKind{gk: gk, all: all})
}

// SetSuperseded records that owner owns field of kind gk, unless another kind already does.
// For the resolver only.
func (r *BackendPolicyResolution) SetSuperseded(gk schema.GroupKind, field string, owner PolicyWinner) {
	r.mustBeMutable()
	if r.IsSuperseded(gk, field) {
		return
	}
	r.superseded = append(r.superseded, supersededField{gk: gk, field: field, owner: owner})
}

// AddDropped records a field the attachment ref sets that does not take effect. For the
// resolver only; callers add fields in a deterministic order.
func (r *BackendPolicyResolution) AddDropped(ref *AttachedPolicyRef, d DroppedField) {
	r.mustBeMutable()
	if ref == nil {
		return
	}
	id := ref.IDWithSectionName()
	for i := range r.dropped {
		if r.dropped[i].id == id {
			r.dropped[i].fields = append(r.dropped[i].fields, d)
			return
		}
	}
	r.dropped = append(r.dropped, droppedFields{id: id, fields: []DroppedField{d}})
}

func (r *BackendPolicyResolution) mustBeMutable() {
	if r == emptyResolution {
		panic("ir: the shared empty BackendPolicyResolution must not be modified")
	}
}
