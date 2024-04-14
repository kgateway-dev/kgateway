package e2e

import (
	"context"
	"math/rand"
	"testing"

	"github.com/solo-io/gloo/test/kubernetes/testutils/actions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/actions/provider"

	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	"github.com/solo-io/gloo/test/kubernetes/testutils/runtime"

	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
)

// TestSuite is the structure around a set of tests that run against a Kubernetes Cluster
// Within a TestSuite, we spin off multiple TestInstallation to test the behavior of a particular installation
type TestSuite struct {
	// TestingFramework defines the framework that tests should rely on
	// Within the Gloo codebase, we rely extensively on Ginkgo and Gomega
	// The idea behind making this configurable, per suite, is that it ensures that
	// all of our tests can rely on the testing interface, instead of the explicit Ginkgo implementation
	TestingFramework testing.TB

	// RuntimeContext contains the set of properties that are defined at runtime by whoever is invoking tests
	RuntimeContext runtime.Context

	// ClusterContext contains the metadata about the Kubernetes Cluster that is used for this TestSuite
	ClusterContext *cluster.Context

	// activeInstallations is the set of TestInstallation that have been created for this Suite.
	// Since tests are run serially, this will only have a single entry at a time
	activeInstallations map[string]*TestInstallation
}

// PreFailHandler will execute the PreFailHandler for any of the TestInstallation that are registered
// with the given TestSuite.
// The function will be executed when a test in the TestSuite fails, but before any of the cleanup
// functions (AfterEach, AfterAll) are invoked. This allows us to capture relevant details about
// the running installation of Gloo Gateway and the Kubernetes Cluster
func (s *TestSuite) PreFailHandler() {
	for _, i := range s.activeInstallations {
		i.preFailHandler()
	}
}

func (s *TestSuite) RegisterTestInstallation(name string, glooGatewayContext *gloogateway.Context) *TestInstallation {
	if s.activeInstallations == nil {
		s.activeInstallations = make(map[string]*TestInstallation, 2)
	}

	installation := &TestInstallation{
		// Create a reference to the TestSuite, and all of it's metadata
		TestSuite: s,

		// Name is a unique identifier for this TestInstallation
		Name: name,

		// Create an operator which is responsible for executing operations against the cluster
		Operator: operations.NewGinkgoOperator(),

		// Create an operations provider, and point it to the running installation
		Actions: provider.NewActionsProvider(s.TestingFramework).
			WithClusterContext(s.ClusterContext).
			WithGlooGatewayContext(glooGatewayContext),

		// Create an assertions provider, and point it to the running installation
		Assertions: assertions.NewProvider(s.TestingFramework).
			WithClusterContext(s.ClusterContext).
			WithGlooGatewayContext(glooGatewayContext),
	}
	s.activeInstallations[name] = installation

	return installation
}

func (s *TestSuite) UnregisterTestInstallation(installation *TestInstallation) {
	delete(s.activeInstallations, installation.Name)
}

// TestInstallation is the structure around a set of tests that validate behavior for an installation
// of Gloo Gateway.
type TestInstallation struct {
	*TestSuite

	// Name is a unique identifier for this TestInstallation
	Name string

	// Operator is responsible for executing operations against an installation of Gloo Gateway
	// This is meant to simulate the behaviors that a person could execute
	Operator *operations.Operator

	// Actions is the entity that creates actions that can be executed by the Operator
	Actions *provider.ActionsProvider

	// Assertions is the entity that creates assertions that can be executed by the Operator
	Assertions *assertions.Provider
}

func (i *TestInstallation) InstallGlooGateway(ctx context.Context, installAction actions.ClusterAction) error {
	installOperation := &operations.BasicOperation{
		OpName:      "install-gloo-gateway",
		OpAction:    installAction,
		OpAssertion: i.Assertions.InstallationWasSuccessful(),
	}
	return i.Operator.ExecuteOperations(ctx, installOperation)
}

func (i *TestInstallation) UninstallGlooGateway(ctx context.Context, uninstallAction actions.ClusterAction) error {
	installOperation := &operations.BasicOperation{
		OpName:      "uninstall-gloo-gateway",
		OpAction:    uninstallAction,
		OpAssertion: i.Assertions.UninstallationWasSuccessful(),
	}
	return i.Operator.ExecuteOperations(ctx, installOperation)
}

// RunTests will execute a batch of e2e.Test against the installation
func (i *TestInstallation) RunTests(ctx context.Context, tests ...Test) {
	randomizeTests(tests...)

	for _, e2eTest := range tests {
		if e2eTest.Name == "" {
			i.TestingFramework.Fatal("All tests must include a name")
		}

		i.TestingFramework.Logf("TEST: %s", e2eTest.Name)
		e2eTest.Test(ctx, i)
	}
}

// preFailHandler is the function that is invoked if a test in the given TestInstallation fails
func (i *TestInstallation) preFailHandler() {
	exportReportOp := &operations.BasicOperation{
		OpName:   "glooctl-export-report",
		OpAction: i.Actions.GlooCtl().ExportReport(),
		OpAssertion: func(ctx context.Context) {
			// This action is performed on test failure, and is not modifying the cluster
			// As a result, there is no assertion that we perform
		},
	}
	err := i.Operator.ExecuteOperations(context.Background(), exportReportOp)
	if err != nil {
		i.TestingFramework.Errorf("Failed to executed preFailHandler operation for TestInstallation (%s): %+v", i.Name, err)
	}
}

// randomizeTests shuffles the list of tests in-place
// This is done to insert chaos in the tests, and ensure that tests do not depend on being run in a certain order
func randomizeTests(tests ...Test) {
	rand.Shuffle(len(tests), func(i, j int) {
		tests[i], tests[j] = tests[j], tests[i]
	})
}

// TestFn is a function that executes a test, for a given TestInstallation
type TestFn func(ctx context.Context, suite *TestInstallation)

// Test represents a single end-to-end behavior that is validated against a running installation of Gloo Gateway.
// Tests are grouped by the feature they validate, and are defined in the test/kubernetes/e2e/features directory
type Test struct {
	// Name is a required value that uniquely identifies a test
	Name string
	// Description is an optional value that is used to provide context to developers about a test's purpose
	Description string
	// Test is the actual function that executes the test
	Test TestFn
}
