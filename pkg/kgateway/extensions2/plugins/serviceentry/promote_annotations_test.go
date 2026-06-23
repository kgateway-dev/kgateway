package serviceentry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	networking "istio.io/api/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/util/sets"
)

// TestSelectedWorkloadFromEntry_PromoteAnnotations verifies the generic WorkloadEntry
// annotation-promotion behavior: configured annotation keys are copied verbatim into
// AugmentedLabels, unconfigured keys are left alone, and absent annotations are a no-op.
// The keys here are arbitrary; kgateway is agnostic to what they mean.
func TestSelectedWorkloadFromEntry_PromoteAnnotations(t *testing.T) {
	tests := []struct {
		name                string
		metadataLabels      map[string]string
		metadataAnnotations map[string]string
		promoteKeys         sets.Set[string]
		wantLabel           map[string]string // key -> expected value in AugmentedLabels
		wantAbsentKeys      []string
	}{
		{
			name: "nil promote set promotes nothing",
			metadataAnnotations: map[string]string{
				"example.io/promote-me": "42",
			},
			promoteKeys:    nil,
			wantAbsentKeys: []string{"example.io/promote-me"},
		},
		{
			name: "configured key is promoted into labels",
			metadataAnnotations: map[string]string{
				"example.io/promote-me": "42",
			},
			promoteKeys: sets.New("example.io/promote-me"),
			wantLabel:   map[string]string{"example.io/promote-me": "42"},
		},
		{
			name: "unconfigured annotation key is not promoted",
			metadataAnnotations: map[string]string{
				"example.io/promote-me": "42",
				"example.io/ignore-me":  "99",
			},
			promoteKeys:    sets.New("example.io/promote-me"),
			wantLabel:      map[string]string{"example.io/promote-me": "42"},
			wantAbsentKeys: []string{"example.io/ignore-me"},
		},
		{
			name:                "absent annotation is a no-op",
			metadataAnnotations: nil,
			promoteKeys:         sets.New("example.io/promote-me"),
			wantAbsentKeys:      []string{"example.io/promote-me"},
		},
		{
			name: "promoted value coexists with existing labels",
			metadataLabels: map[string]string{
				"app": "foo",
			},
			metadataAnnotations: map[string]string{
				"example.io/promote-me": "42",
			},
			promoteKeys: sets.New("example.io/promote-me"),
			wantLabel: map[string]string{
				"app":                   "foo",
				"example.io/promote-me": "42",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workload := selectedWorkloadFromEntry(
				"we", "ns",
				tt.metadataLabels,
				tt.metadataAnnotations,
				tt.promoteKeys,
				&networking.WorkloadEntry{Address: "1.2.3.4"},
				nil,
			)

			for k, want := range tt.wantLabel {
				assert.Equal(t, want, workload.AugmentedLabels[k], "label %q", k)
			}
			for _, k := range tt.wantAbsentKeys {
				_, ok := workload.AugmentedLabels[k]
				assert.False(t, ok, "label %q should be absent", k)
			}
		})
	}
}
