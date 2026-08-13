package trafficpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildStringListMatcher(t *testing.T) {
	tests := []struct {
		name      string
		headers   []string
		wantNil   bool
		wantExact []string
	}{
		{
			name:    "nil input returns nil",
			headers: nil,
			wantNil: true,
		},
		{
			name:    "empty slice returns nil",
			headers: []string{},
			wantNil: true,
		},
		{
			name:      "single header",
			headers:   []string{"location"},
			wantExact: []string{"location"},
		},
		{
			name:      "multiple headers produce exact matchers in order",
			headers:   []string{"location", "set-cookie", "www-authenticate"},
			wantExact: []string{"location", "set-cookie", "www-authenticate"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildStringListMatcher(tt.headers)
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			require.Len(t, result.Patterns, len(tt.wantExact))
			for i, want := range tt.wantExact {
				assert.Equal(t, want, result.Patterns[i].GetExact())
			}
		})
	}
}
