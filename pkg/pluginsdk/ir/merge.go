package ir

import (
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/util/sets"
)

// MergeOriginsRefCount is used to track the state of a policy ref in the MergeOrigins
type MergeOriginsRefCount int

const (
	// MergeOriginsRefCountNone means the ref is not set for any fields in MergeOrigins
	MergeOriginsRefCountNone MergeOriginsRefCount = iota

	// MergeOriginsRefCountPartial means the ref is partially set, i.e., set for one or more fields but not all
	MergeOriginsRefCountPartial

	// MergeOriginsRefCountAll means the ref is fully set, i.e., set for all fields
	MergeOriginsRefCountAll
)

// MergeOrigins maps policy field names to policy refs that contribute to it
// during policy merging
type MergeOrigins map[string]sets.Set[string]

// Get returns the policy refs for the given field name
func (m MergeOrigins) Get(
	field string,
) []string {
	if _, ok := m[field]; !ok {
		return nil
	}
	return m[field].UnsortedList()
}

// SetOne updates the policy refs for the field with the given ref or MergeOrigins
// if the ref is nil.
// This should be used with shallow merging.
func (m MergeOrigins) SetOne(
	field string,
	policyRef *AttachedPolicyRef,
	mergeOrigins MergeOrigins,
) {
	if policyRef != nil {
		m[field] = sets.New(policyRef.ID())
		return
	}
	m[field] = mergeOrigins[field].Clone()
}

// Append updates the policy refs for the field by appending the given ref or
// MergeOrigins if the ref is nil.
// This should be used with deep merging.
func (m MergeOrigins) Append(
	field string,
	policyRef *AttachedPolicyRef,
	mergeOrigins MergeOrigins,
) {
	if _, ok := m[field]; !ok {
		m[field] = sets.New[string]()
	}
	if policyRef != nil {
		m[field].Insert(policyRef.ID())
		return
	}
	m[field] = m[field].Union(mergeOrigins[field])
}

// GetRefCount returns an enum indicating the count of the given policy ref in the MergeOrigins
func (m MergeOrigins) GetRefCount(
	policyRef *AttachedPolicyRef,
) MergeOriginsRefCount {
	if policyRef == nil || !m.IsSet() {
		return MergeOriginsRefCountNone
	}

	forRef := 0
	for _, field := range m {
		if field != nil && field.Has(policyRef.ID()) {
			forRef++
		}
	}

	switch forRef {
	case 0:
		return MergeOriginsRefCountNone
	case len(m):
		return MergeOriginsRefCountAll
	default:
		return MergeOriginsRefCountPartial
	}
}

// IsSet return a boolean indicating whether MergeOrigins is set
func (m MergeOrigins) IsSet() bool {
	return len(m) > 0
}

func (m MergeOrigins) ToProtoStruct() *structpb.Struct {
	if !m.IsSet() {
		return nil
	}

	s := &structpb.Struct{
		Fields: make(map[string]*structpb.Value),
	}
	for field, refs := range m {
		// Create a ListValue for the slice of strings
		listValues := make([]*structpb.Value, len(refs))
		// Iterate the list in sorted order to ensure deterministic output (List vs UnsortedList)
		for i, str := range sets.List(refs) {
			listValues[i] = structpb.NewStringValue(str)
		}
		s.Fields[field] = structpb.NewListValue(&structpb.ListValue{
			Values: listValues,
		})
	}
	return s
}
