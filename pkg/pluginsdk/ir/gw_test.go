package ir

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
)

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
