package matchers

import (
	"fmt"

	"github.com/onsi/gomega/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ExpectedObject is a struct that represents the expected object.
type ExpectedObject struct {
	// Name is the object name.
	Name string

	// Namespace is the object namespace.
	Namespace string
}

// ObjectMatches returns a GomegaMatcher that checks whether an object matches
// the specified fields (currently only name and namespace).
func ObjectMatches(object ExpectedObject) types.GomegaMatcher {
	return &objectMatcher{expectedObject: object}
}

type objectMatcher struct {
	expectedObject ExpectedObject
}

func (m *objectMatcher) Match(actual interface{}) (bool, error) {
	object, ok := actual.(client.Object)
	if !ok {
		return false, fmt.Errorf("expected a client.Object, got %T", actual)
	}

	return object.GetName() == m.expectedObject.Name &&
		object.GetNamespace() == m.expectedObject.Namespace, nil
}

func (m *objectMatcher) FailureMessage(actual interface{}) string {
	object := actual.(client.Object)

	return fmt.Sprintf("expected: %s.%s\nto match: %s.%s",
		m.expectedObject.Namespace, m.expectedObject.Name,
		object.GetNamespace(), object.GetName())
}

func (m *objectMatcher) NegatedFailureMessage(actual interface{}) string {
	object := actual.(client.Object)

	return fmt.Sprintf("expected: %s.%s\nnot to match: %s.%s",
		m.expectedObject.Namespace, m.expectedObject.Name,
		object.GetNamespace(), object.GetName())
}
