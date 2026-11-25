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

// DroppedField is one field a policy sets that does not take effect on a backend.
type DroppedField struct {
	// Field is the field's MergeOrigins key, e.g. "tls".
	Field string
	// Kind is the policy kind that supplies the field instead.
	Kind schema.GroupKind
	// By is the policy that supplies the field instead. Nil when the winner is a merged
	// policy with no single source.
	By *AttachedPolicyRef
	// ByValid is false when By has errors. A backend whose winning policy is invalid
	// gets a blackhole cluster rather than the dropped field.
	ByValid bool
	Reason  DroppedReason
}

// SupersedingPolicy is the policy kind that owns another kind's field on a backend.
type SupersedingPolicy struct {
	Kind schema.GroupKind
	// By is the policy of Kind that applies; nil when it is a merged policy.
	By *AttachedPolicyRef
	// ByValid is false when By has errors, so the cluster is blackholed.
	ByValid bool
}

// BackendPolicyResolution is the single decision of what each attached policy contributes
// to a backend. It is computed once, when policies attach to the backend, and everything
// downstream reads it: backend translation applies Applied, and status reports Dropped. No
// consumer merges attached policies again, so what is applied and what is reported cannot
// disagree.
type BackendPolicyResolution struct {
	// applied holds, per policy kind, what backend translation applies: the merged policy
	// for kinds that merge, with fields another kind supersedes cleared, otherwise every
	// attachment.
	applied map[schema.GroupKind][]PolicyAtt
	// superseded holds, per policy kind, the fields another attached kind owns.
	superseded map[schema.GroupKind]map[string]SupersedingPolicy
	// dropped holds, per attachment (AttachedPolicyRef.IDWithSectionName), the fields it
	// sets that do not take effect.
	dropped map[string][]DroppedField
}

// NewBackendPolicyResolution returns an empty resolution for the resolver to fill.
func NewBackendPolicyResolution() *BackendPolicyResolution {
	return &BackendPolicyResolution{
		applied:    map[schema.GroupKind][]PolicyAtt{},
		superseded: map[schema.GroupKind]map[string]SupersedingPolicy{},
		dropped:    map[string][]DroppedField{},
	}
}

// Applied returns what backend translation applies for policy kind gk: a single merged
// policy for kinds that merge, every attachment otherwise, nil when none is attached.
func (r *BackendPolicyResolution) Applied(gk schema.GroupKind) []PolicyAtt {
	if r == nil {
		return nil
	}
	return r.applied[gk]
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

// SupersededBy returns the policy kind that owns field of kind gk, if any.
func (r *BackendPolicyResolution) SupersededBy(gk schema.GroupKind, field string) (SupersedingPolicy, bool) {
	if r == nil {
		return SupersedingPolicy{}, false
	}
	s, ok := r.superseded[gk][field]
	return s, ok
}

// Dropped returns the fields the attachment ref sets that do not take effect, in field order.
func (r *BackendPolicyResolution) Dropped(ref *AttachedPolicyRef) []DroppedField {
	if r == nil || ref == nil {
		return nil
	}
	return r.dropped[ref.IDWithSectionName()]
}

// SetApplied records what translation applies for kind gk. For the resolver only.
func (r *BackendPolicyResolution) SetApplied(gk schema.GroupKind, pols []PolicyAtt) {
	r.applied[gk] = pols
}

// SetSuperseded records that owner owns field of kind gk. For the resolver only.
func (r *BackendPolicyResolution) SetSuperseded(gk schema.GroupKind, field string, owner SupersedingPolicy) {
	if r.superseded[gk] == nil {
		r.superseded[gk] = map[string]SupersedingPolicy{}
	}
	r.superseded[gk][field] = owner
}

// AddDropped records a field the attachment ref sets that does not take effect. For the
// resolver only; callers add fields in a deterministic order.
func (r *BackendPolicyResolution) AddDropped(ref *AttachedPolicyRef, d DroppedField) {
	if ref == nil {
		return
	}
	id := ref.IDWithSectionName()
	r.dropped[id] = append(r.dropped[id], d)
}
