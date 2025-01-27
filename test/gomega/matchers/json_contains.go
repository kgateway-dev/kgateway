package matchers

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/onsi/gomega"
)

func JSONContains(expectedJSON any) gomega.OmegaMatcher {
	matchers := []gomega.OmegaMatcher{
		gomega.Not(gomega.BeNil()),
		gomega.Not(gomega.BeEmpty()),
	}

	expectedBytes, ok := expectedJSON.([]byte)
	if !ok {
		return gomega.BeFalseBecause("expected value must be a byte slice")
	}

	var expected map[string]any
	err := json.Unmarshal(expectedBytes, &expected)
	if err != nil {
		return gomega.BeFalseBecause(err.Error())
	}

	matchers = append(matchers, ContainsDeepMapElements(expected))

	return &JSONContainsMatcher{
		expected:  expected,
		matchers:  gomega.And(matchers...),
		evaluated: false,
	}
}

type JSONContainsMatcher struct {
	expected  interface{}
	matchers  gomega.OmegaMatcher
	evaluated bool
}

func (matcher *JSONContainsMatcher) Match(actualBytes interface{}) (success bool, err error) {
	if matcher.evaluated {
		return false, errors.New("using the same matcher twice can lead to inconsistent behaviors")
	}
	matcher.evaluated = true

	actualJSON, ok := actualBytes.([]byte)
	if !ok {
		return false, nil
	}

	var actual map[string]any
	err = json.Unmarshal(actualJSON, &actual)
	if err != nil {
		return false, err
	}

	if ok, matchErr := matcher.matchers.Match(actual); !ok {
		return false, matchErr
	}

	return true, nil
}

func (matcher *JSONContainsMatcher) FailureMessage(actual any) (message string) {
	return fmt.Sprintf("%s \n%s",
		matcher.matchers.FailureMessage(actual),
		informativeComparison(matcher.expected, actual))
}

func (matcher *JSONContainsMatcher) NegatedFailureMessage(actual any) (message string) {
	return fmt.Sprintf("%s \n%s",
		matcher.matchers.NegatedFailureMessage(actual),
		informativeComparison(matcher.expected, actual))
}
