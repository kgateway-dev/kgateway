package e2e

import (
	"context"
	"math/rand"
	"testing"

	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	"github.com/solo-io/gloo/test/kubernetes/testutils/runtime"

	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations/provider"
)

type TestSuite struct {
	// TestingFramework defines the framework that tests should rely on
	// Within the Gloo codebase, we rely extensively on Ginkgo and Gomega
	// The idea behind making this configurable, per suite, is that it ensures that
	// all of our tests can rely on the testing interface, instead of the explicit Ginkgo implementation
	TestingFramework testing.TB

	RuntimeContext runtime.Context

	// ClusterContext contains the metadata about the Kubernetes Cluster that is used for this TestSuite
	// At the moment, an entire TestSuite is run against a single Kubernetes cluster
	ClusterContext *cluster.Context
}

type TestInstallation struct {
	*TestSuite

	// Operator is responsible for executing operations against an installation
	// of Gloo Gateway, running in Kubernetes Cluster
	// This is meant to simulate the behaviors that a person could execute
	Operator *operations.Operator

	// OperationsProvider is the entity that creates operations that can be executed by the Operator
	OperationsProvider *provider.OperationProvider

	// AssertionsProvider is the entity that creates assertions that can be executed by the Operator
	AssertionsProvider *assertions.Provider
}

func NewTestInstallation(testSuite *TestSuite, glooGatewayContext *gloogateway.Context) *TestInstallation {
	return &TestInstallation{
		// Create a reference to the TestSuite, and all of it's metadata
		TestSuite: testSuite,

		// Create an operator which is responsible for executing operations against the cluster
		Operator: operations.NewGinkgoOperator(),

		// Create an operations provider, and point it to the running installation
		OperationsProvider: provider.NewOperationProvider(testSuite.TestingFramework).
			WithClusterContext(testSuite.ClusterContext).
			WithGlooGatewayContext(glooGatewayContext),

		// Create an assertions provider, and point it to the running installation
		AssertionsProvider: assertions.NewProvider(testSuite.TestingFramework).
			WithClusterContext(testSuite.ClusterContext).
			WithGlooGatewayContext(glooGatewayContext),
	}
}

// TestFn is a function that executes a test, for a given TestInstallation
type TestFn func(ctx context.Context, suite *TestInstallation)

// Test represents a single end-to-end behavior that is validated against a running installation of Gloo Gateway
// Tests are grouped by the feature they validate, and are defined in the e2e/features directory
type Test struct {
	Name        string
	Description string

	Test TestFn
}

// RunTests will execute a batch of e2e.Test against the installation
func (i *TestInstallation) RunTests(ctx context.Context, tests ...Test) {
	randomizeTests(tests...)

	for _, e2eTest := range tests {
		i.TestingFramework.Logf("TEST: %s", e2eTest.Name)
		e2eTest.Test(ctx, i)
	}
}

// randomizeTests shuffles the list of tests in-place
// This is done to insert chaos in the tests, and ensure that tests do not depend on being run in a certain order
func randomizeTests(tests ...Test) {
	rand.Shuffle(len(tests), func(i, j int) {
		tests[i], tests[j] = tests[j], tests[i]
	})
}
