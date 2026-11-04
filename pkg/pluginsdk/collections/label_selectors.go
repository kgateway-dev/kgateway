package collections

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ParseLabelSelectors parses a JSON-encoded list of Kubernetes LabelSelectors.
func ParseLabelSelectors(raw string) ([]labels.Selector, error) {
	if raw == "" {
		raw = "[]"
	}

	var labelSelectors []metav1.LabelSelector
	if err := json.Unmarshal([]byte(raw), &labelSelectors); err != nil {
		return nil, fmt.Errorf("error parsing label selectors %q: %w", raw, err)
	}
	return ToSelectors(labelSelectors)
}

// ParseExclusionLabelSelectors parses a JSON-encoded list of Kubernetes LabelSelectors for exclusion use cases.
// Empty selector entries are rejected because they match everything.
func ParseExclusionLabelSelectors(raw string) ([]labels.Selector, error) {
	selectors, err := ParseLabelSelectors(raw)
	if err != nil {
		return nil, err
	}
	for i, selector := range selectors {
		if selector.Empty() {
			return nil, fmt.Errorf("label selector at index %d must not be empty", i)
		}
	}
	return selectors, nil
}

// ToSelectors converts Kubernetes LabelSelectors to executable selectors.
func ToSelectors(labelSelectors []metav1.LabelSelector) ([]labels.Selector, error) {
	out := make([]labels.Selector, 0, len(labelSelectors))
	for i := range labelSelectors {
		sel, err := metav1.LabelSelectorAsSelector(&labelSelectors[i])
		if err != nil {
			return nil, fmt.Errorf("label selector at index %d: %w", i, err)
		}
		out = append(out, sel)
	}
	return out, nil
}

// MatchesAnyLabelSelector returns true if objLabels match any selector.
func MatchesAnyLabelSelector(selectors []labels.Selector, objLabels map[string]string) bool {
	for _, selector := range selectors {
		if selector.Matches(labels.Set(objLabels)) {
			return true
		}
	}
	return false
}
