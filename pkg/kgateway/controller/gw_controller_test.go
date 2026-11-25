package controller

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		limit int
		want  string
	}{
		{
			name:  "a text within the limit is returned as it is",
			text:  "short",
			limit: 100,
			want:  "short",
		},
		{
			name:  "a text at the limit is returned as it is",
			text:  strings.Repeat("a", 100),
			limit: 100,
			want:  strings.Repeat("a", 100),
		},
		{
			name:  "a longer text keeps its start and says it was cut",
			text:  strings.Repeat("a", 101),
			limit: 100,
			want:  strings.Repeat("a", 100-len(truncatedSuffix)) + truncatedSuffix,
		},
		{
			// The cut would fall inside the last two-byte character, so the whole
			// character is left out.
			name:  "a character is not split",
			text:  strings.Repeat("a", 99-len(truncatedSuffix)) + "éé" + strings.Repeat("a", 100),
			limit: 100,
			want:  strings.Repeat("a", 99-len(truncatedSuffix)) + truncatedSuffix,
		},
		{
			name:  "a limit shorter than the suffix leaves only the suffix",
			text:  strings.Repeat("a", 100),
			limit: 10,
			want:  truncatedSuffix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Cut the text.
			got := truncateText(tt.text, tt.limit)

			// It is the expected text, and still valid UTF-8.
			assert.Equal(t, tt.want, got)
			assert.True(t, utf8.ValidString(got), "the cut text should be valid UTF-8")
		})
	}
}
