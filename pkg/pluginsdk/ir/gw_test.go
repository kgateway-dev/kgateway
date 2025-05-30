package ir

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
			policies: map[schema.GroupKind][]PolicyAtt{VirtualBuiltInGK: {}, fooGK: {}, barGK: {}},
			assertFn: func(a *assert.Assertions, got []schema.GroupKind) {
				a.Len(got, 3)
				a.Equal(got[2], VirtualBuiltInGK, "VirtualBuiltInGK should be last in the list")
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
				a.Equal(got[2], VirtualBuiltInGK, "VirtualBuiltInGK should be last in the list")
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
