package ir

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestMergeOrigins_Get(t *testing.T) {
	tests := []struct {
		name         string
		mergeOrigins MergeOrigins
		field        string
		expectedRefs []string
	}{
		{
			name:         "get from empty MergeOrigins",
			mergeOrigins: MergeOrigins{},
			field:        "field1",
			expectedRefs: nil,
		},
		{
			name: "get existing field with single ref",
			mergeOrigins: MergeOrigins{
				"field1": sets.New("group1/kind1/ns1/name1"),
			},
			field:        "field1",
			expectedRefs: []string{"group1/kind1/ns1/name1"},
		},
		{
			name: "get existing field with multiple refs",
			mergeOrigins: MergeOrigins{
				"field1": sets.New("group1/kind1/ns1/name1", "group2/kind2/ns2/name2"),
			},
			field:        "field1",
			expectedRefs: []string{"group1/kind1/ns1/name1", "group2/kind2/ns2/name2"},
		},
		{
			name: "get non-existing field",
			mergeOrigins: MergeOrigins{
				"field1": sets.New("group1/kind1/ns1/name1"),
			},
			field:        "field2",
			expectedRefs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.mergeOrigins.Get(tt.field)
			if tt.expectedRefs == nil {
				assert.Nil(t, result)
			} else {
				assert.ElementsMatch(t, tt.expectedRefs, result)
			}
		})
	}
}

func TestMergeOrigins_SetOne(t *testing.T) {
	tests := []struct {
		name                string
		initialMergeOrigins MergeOrigins
		field               string
		policyRef           *AttachedPolicyRef
		sourceMergeOrigins  MergeOrigins
		expectedResult      MergeOrigins
	}{
		{
			name:                "set with policy ref on empty MergeOrigins",
			initialMergeOrigins: MergeOrigins{},
			field:               "field1",
			policyRef: &AttachedPolicyRef{
				Group:     "group1",
				Kind:      "kind1",
				Namespace: "ns1",
				Name:      "name1",
			},
			sourceMergeOrigins: nil,
			expectedResult: MergeOrigins{
				"field1": sets.New("group1/kind1/ns1/name1"),
			},
		},
		{
			name: "set with policy ref replacing existing field",
			initialMergeOrigins: MergeOrigins{
				"field1": sets.New("old/ref/ns/name"),
			},
			field: "field1",
			policyRef: &AttachedPolicyRef{
				Group:     "group2",
				Kind:      "kind2",
				Namespace: "ns2",
				Name:      "name2",
			},
			sourceMergeOrigins: nil,
			expectedResult: MergeOrigins{
				"field1": sets.New("group2/kind2/ns2/name2"),
			},
		},
		{
			name:                "set with nil policy ref using merge origins",
			initialMergeOrigins: MergeOrigins{},
			field:               "field1",
			policyRef:           nil,
			sourceMergeOrigins: MergeOrigins{
				"field1": sets.New("group1/kind1/ns1/name1", "group2/kind2/ns2/name2"),
			},
			expectedResult: MergeOrigins{
				"field1": sets.New("group1/kind1/ns1/name1", "group2/kind2/ns2/name2"),
			},
		},
		{
			name: "set with nil policy ref and nil merge origins",
			initialMergeOrigins: MergeOrigins{
				"field1": sets.New("existing/ref/ns/name"),
			},
			field:              "field1",
			policyRef:          nil,
			sourceMergeOrigins: MergeOrigins{},
			expectedResult: MergeOrigins{
				"field1": sets.New[string](),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.initialMergeOrigins.SetOne(tt.field, tt.policyRef, tt.sourceMergeOrigins)
			assert.Equal(t, tt.expectedResult, tt.initialMergeOrigins)
		})
	}
}

func TestMergeOrigins_Append(t *testing.T) {
	tests := []struct {
		name                string
		initialMergeOrigins MergeOrigins
		field               string
		policyRef           *AttachedPolicyRef
		sourceMergeOrigins  MergeOrigins
		expectedResult      MergeOrigins
	}{
		{
			name:                "append with policy ref on empty MergeOrigins",
			initialMergeOrigins: MergeOrigins{},
			field:               "field1",
			policyRef: &AttachedPolicyRef{
				Group:     "group1",
				Kind:      "kind1",
				Namespace: "ns1",
				Name:      "name1",
			},
			sourceMergeOrigins: nil,
			expectedResult: MergeOrigins{
				"field1": sets.New("group1/kind1/ns1/name1"),
			},
		},
		{
			name: "append with policy ref to existing field",
			initialMergeOrigins: MergeOrigins{
				"field1": sets.New("existing/ref/ns/name"),
			},
			field: "field1",
			policyRef: &AttachedPolicyRef{
				Group:     "group2",
				Kind:      "kind2",
				Namespace: "ns2",
				Name:      "name2",
			},
			sourceMergeOrigins: nil,
			expectedResult: MergeOrigins{
				"field1": sets.New("existing/ref/ns/name", "group2/kind2/ns2/name2"),
			},
		},
		{
			name: "append with nil policy ref using merge origins",
			initialMergeOrigins: MergeOrigins{
				"field1": sets.New("existing/ref/ns/name"),
			},
			field:     "field1",
			policyRef: nil,
			sourceMergeOrigins: MergeOrigins{
				"field1": sets.New("source1/ref/ns/name", "source2/ref/ns/name"),
			},
			expectedResult: MergeOrigins{
				"field1": sets.New("existing/ref/ns/name", "source1/ref/ns/name", "source2/ref/ns/name"),
			},
		},
		{
			name:                "append to non-existing field",
			initialMergeOrigins: MergeOrigins{},
			field:               "field2",
			policyRef: &AttachedPolicyRef{
				Group:     "group1",
				Kind:      "kind1",
				Namespace: "ns1",
				Name:      "name1",
			},
			sourceMergeOrigins: nil,
			expectedResult: MergeOrigins{
				"field2": sets.New("group1/kind1/ns1/name1"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.initialMergeOrigins.Append(tt.field, tt.policyRef, tt.sourceMergeOrigins)
			assert.Equal(t, tt.expectedResult, tt.initialMergeOrigins)
		})
	}
}

func TestMergeOrigins_IsSet(t *testing.T) {
	tests := []struct {
		name         string
		mergeOrigins MergeOrigins
		expected     bool
	}{
		{
			name:         "empty MergeOrigins",
			mergeOrigins: MergeOrigins{},
			expected:     false,
		},
		{
			name:         "nil MergeOrigins",
			mergeOrigins: nil,
			expected:     false,
		},
		{
			name: "MergeOrigins with one field",
			mergeOrigins: MergeOrigins{
				"field1": sets.New("group1/kind1/ns1/name1"),
			},
			expected: true,
		},
		{
			name: "MergeOrigins with multiple fields",
			mergeOrigins: MergeOrigins{
				"field1": sets.New("group1/kind1/ns1/name1"),
				"field2": sets.New("group2/kind2/ns2/name2"),
			},
			expected: true,
		},
		{
			name: "MergeOrigins with empty set",
			mergeOrigins: MergeOrigins{
				"field1": sets.New[string](),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.mergeOrigins.IsSet()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMergeOrigins_GetRefCount(t *testing.T) {
	policyRef1 := &AttachedPolicyRef{
		Group:     "group1",
		Kind:      "kind1",
		Namespace: "ns1",
		Name:      "name1",
	}

	tests := []struct {
		name         string
		mergeOrigins MergeOrigins
		policyRef    *AttachedPolicyRef
		expected     MergeOriginsRefCount
	}{
		{
			name:         "nil policy ref",
			mergeOrigins: MergeOrigins{"field1": sets.New("group1/kind1/ns1/name1")},
			policyRef:    nil,
			expected:     MergeOriginsRefCountNone,
		},
		{
			name:         "empty MergeOrigins",
			mergeOrigins: MergeOrigins{},
			policyRef:    policyRef1,
			expected:     MergeOriginsRefCountNone,
		},
		{
			name: "ref not found in any field",
			mergeOrigins: MergeOrigins{
				"field1": sets.New("group2/kind2/ns2/name2"),
				"field2": sets.New("group3/kind3/ns3/name3"),
			},
			policyRef: policyRef1,
			expected:  MergeOriginsRefCountNone,
		},
		{
			name: "ref found in some fields (partial)",
			mergeOrigins: MergeOrigins{
				"field1": sets.New("group1/kind1/ns1/name1"),
				"field2": sets.New("group2/kind2/ns2/name2"),
				"field3": sets.New("group1/kind1/ns1/name1"),
			},
			policyRef: policyRef1,
			expected:  MergeOriginsRefCountPartial,
		},
		{
			name: "ref found in all fields",
			mergeOrigins: MergeOrigins{
				"field1": sets.New("group1/kind1/ns1/name1"),
				"field2": sets.New("group1/kind1/ns1/name1", "group2/kind2/ns2/name2"),
			},
			policyRef: policyRef1,
			expected:  MergeOriginsRefCountAll,
		},
		{
			name: "single field with matching ref",
			mergeOrigins: MergeOrigins{
				"field1": sets.New("group1/kind1/ns1/name1"),
			},
			policyRef: policyRef1,
			expected:  MergeOriginsRefCountAll,
		},
		{
			name: "field with nil set",
			mergeOrigins: MergeOrigins{
				"field1": nil,
				"field2": sets.New("group1/kind1/ns1/name1"),
			},
			policyRef: policyRef1,
			expected:  MergeOriginsRefCountPartial,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.mergeOrigins.GetRefCount(tt.policyRef)
			assert.Equal(t, tt.expected, result)
		})
	}
}
