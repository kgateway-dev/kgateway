package kubectl

import (
	"slices"
	"testing"
)

func TestSelectPodNames(t *testing.T) {
	// a rollout reports both the outgoing (terminating) and incoming pod
	const rollout = "gw-6b54c85cbf-x6v7s\t2026-09-11T05:23:30Z\ngw-74f7b79947-tfz48\t\n"

	tests := []struct {
		name               string
		jsonpathOutput     string
		excludeTerminating bool
		expected           []string
	}{
		{
			name:               "drops terminating pod during a rollout",
			jsonpathOutput:     rollout,
			excludeTerminating: true,
			expected:           []string{"gw-74f7b79947-tfz48"},
		},
		{
			name:               "keeps terminating pod when not excluding",
			jsonpathOutput:     rollout,
			excludeTerminating: false,
			expected:           []string{"gw-6b54c85cbf-x6v7s", "gw-74f7b79947-tfz48"},
		},
		{
			name:               "no pods matched the selector",
			jsonpathOutput:     "",
			excludeTerminating: true,
			expected:           []string{},
		},
		{
			name:               "every pod is terminating",
			jsonpathOutput:     "gw-a\t2026-09-11T05:23:30Z\ngw-b\t2026-09-11T05:23:31Z\n",
			excludeTerminating: true,
			expected:           []string{},
		},
		{
			name:               "several running pods are all returned",
			jsonpathOutput:     "gw-a\t\ngw-b\t\ngw-c\t\n",
			excludeTerminating: true,
			expected:           []string{"gw-a", "gw-b", "gw-c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectPodNames(tt.jsonpathOutput, tt.excludeTerminating)
			if !slices.Equal(got, tt.expected) {
				t.Errorf("selectPodNames() = %v, want %v", got, tt.expected)
			}
		})
	}
}
