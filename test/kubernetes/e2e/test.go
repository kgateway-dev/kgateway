package e2e

import (
	"context"
	"testing"

	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations/provider"
)

// TestFn is a function that executes a test, for a given TestSuite
type TestFn func(ctx context.Context, suite *TestSuite)

// Test represents a single end-to-end behavior that is validated
type Test struct {
	Name        string
	Description string

	Test TestFn
}

// TestSuite consolidates all the properties of a Suite, and makes then accessible to individual tests.
type TestSuite struct {
	// TestingFramework defines the framework that tests should rely on
	// Within the Gloo codebase, we rely extensively on Gingko and Gomega
	// The idea behind making this configurable, per suite, is that it ensures that
	// all of our tests can rely on the testing interface, instead of the explicit Ginkgo implementation
	TestingFramework testing.TB

	// Operator is responsible for executing operations against a Kubernetes Cluster
	// This is meant to simulate the behaviors that a person could execute
	Operator *operations.Operator

	// OperationsProvider is the entity that creates operations that can be executed by the Operator
	OperationsProvider *provider.OperationProvider

	// AssertionsProvider is the entity that creates assertions that can be executed by the Operator
	AssertionsProvider *assertions.Provider
}

// RunTests will execute a batch of e2e.Test for a given suite
func (s *TestSuite) RunTests(ctx context.Context, tests ...Test) {
	for _, e2eTest := range tests {
		s.TestingFramework.Logf("%s", e2eTest.Name)
		e2eTest.Test(ctx, s)
	}

}
