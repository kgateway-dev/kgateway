package serviceentry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestParseExclusionLabels(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want sets.Set[string]
	}{
		{
			name: "empty string returns nil",
			raw:  "",
			want: nil,
		},
		{
			name: "single key",
			raw:  "example.io/managed-by",
			want: sets.New("example.io/managed-by"),
		},
		{
			name: "multiple keys",
			raw:  "example.io/managed-by,example.io/other-key",
			want: sets.New("example.io/managed-by", "example.io/other-key"),
		},
		{
			name: "whitespace around keys is trimmed",
			raw:  " example.io/managed-by , example.io/other-key ",
			want: sets.New("example.io/managed-by", "example.io/other-key"),
		},
		{
			name: "empty tokens from extra commas are dropped",
			raw:  "example.io/managed-by,,example.io/other-key",
			want: sets.New("example.io/managed-by", "example.io/other-key"),
		},
		{
			name: "only whitespace/commas returns nil",
			raw:  " , , ",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseExclusionLabels(tt.raw)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWorkloadEntryIsExcluded(t *testing.T) {
	tests := []struct {
		name          string
		weLabels      map[string]string
		exclusionKeys sets.Set[string]
		want          bool
	}{
		{
			name:          "nil exclusion set never excludes",
			weLabels:      map[string]string{"example.io/managed-by": "some-controller"},
			exclusionKeys: nil,
			want:          false,
		},
		{
			name:          "WE without any exclusion key is not excluded",
			weLabels:      map[string]string{"app": "foo"},
			exclusionKeys: sets.New("example.io/managed-by"),
			want:          false,
		},
		{
			name:          "WE with matching exclusion key is excluded regardless of value",
			weLabels:      map[string]string{"example.io/managed-by": "some-controller"},
			exclusionKeys: sets.New("example.io/managed-by"),
			want:          true,
		},
		{
			name:          "WE matching the second key in the exclusion set is excluded",
			weLabels:      map[string]string{"example.io/other-key": "some-value"},
			exclusionKeys: sets.New("example.io/managed-by", "example.io/other-key"),
			want:          true,
		},
		{
			name:          "WE with empty labels is not excluded",
			weLabels:      map[string]string{},
			exclusionKeys: sets.New("example.io/managed-by"),
			want:          false,
		},
		{
			name:          "nil WE labels is not excluded",
			weLabels:      nil,
			exclusionKeys: sets.New("example.io/managed-by"),
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := workloadEntryIsExcluded(tt.weLabels, tt.exclusionKeys)
			assert.Equal(t, tt.want, got)
		})
	}
}
