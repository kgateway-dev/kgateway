package collections

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseLabelSelectors(t *testing.T) {
	selectors, err := ParseLabelSelectors(`[
		{"matchLabels":{"app":"infra"}},
		{"matchExpressions":[{"key":"example.io/managed-by","operator":"Exists"}]}
	]`)
	require.NoError(t, err)
	require.Len(t, selectors, 2)

	require.True(t, MatchesAnyLabelSelector(selectors, map[string]string{"app": "infra"}))
	require.True(t, MatchesAnyLabelSelector(selectors, map[string]string{"example.io/managed-by": "controller"}))
	require.False(t, MatchesAnyLabelSelector(selectors, map[string]string{"app": "other"}))
}

func TestParseLabelSelectorsDefaultsEmpty(t *testing.T) {
	selectors, err := ParseLabelSelectors("")
	require.NoError(t, err)
	require.Empty(t, selectors)
}

func TestParseLabelSelectorsRejectsInvalidOperator(t *testing.T) {
	_, err := ParseLabelSelectors(`[{"matchExpressions":[{"key":"app","operator":"Nope"}]}]`)
	require.Error(t, err)
}

func TestParseExclusionLabelSelectorsAllowsEmptyList(t *testing.T) {
	selectors, err := ParseExclusionLabelSelectors(`[]`)
	require.NoError(t, err)
	require.Empty(t, selectors)
}

func TestParseExclusionLabelSelectorsRejectsEmptySelector(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "empty object",
			raw:  `[{}]`,
		},
		{
			name: "empty matchLabels",
			raw:  `[{"matchLabels":{}}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseExclusionLabelSelectors(tt.raw)
			require.ErrorContains(t, err, "label selector at index 0 must not be empty")
		})
	}
}
