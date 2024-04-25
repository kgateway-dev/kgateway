package e2e

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kubernetes/testutils/actions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/actions/provider"

	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	"github.com/solo-io/gloo/test/kubernetes/testutils/runtime"

	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
)

// TestCluster is the structure around a set of tests that run against a Kubernetes Cluster
// Within a TestCluster, we spin off multiple TestInstallation to test the behavior of a particular installation
type TestCluster struct {
	// RuntimeContext contains the set of properties that are defined at runtime by whoever is invoking tests
	RuntimeContext runtime.Context

	// ClusterContext contains the metadata about the Kubernetes Cluster that is used for this TestCluster
	ClusterContext *cluster.Context

	// activeInstallations is the set of TestInstallation that have been created for this cluster.
	// Since tests are run serially, this will only have a single entry at a time
	activeInstallations map[string]*TestInstallation
}

// PreFailHandler will execute the PreFailHandler for any of the TestInstallation that are registered
// with the given TestCluster.
// The function will be executed when a test in the TestCluster fails, but before any of the cleanup
// functions (AfterEach, AfterAll) are invoked. This allows us to capture relevant details about
// the running installation of Gloo Gateway and the Kubernetes Cluster
func (c *TestCluster) PreFailHandler() {
	for _, i := range c.activeInstallations {
		i.preFailHandler()
	}
}

func (c *TestCluster) RegisterTestInstallation(glooGatewayContext *gloogateway.Context) *TestInstallation {
	if c.activeInstallations == nil {
		c.activeInstallations = make(map[string]*TestInstallation, 2)
	}

	installation := &TestInstallation{
		// Create a reference to the TestCluster, and all of it's metadata
		TestCluster: c,

		// Maintain a reference to the Metadata used for this installation
		Metadata: glooGatewayContext,

		// ResourceClients are only available _after_ installing Gloo Gateway
		ResourceClients: nil,

		// Create an operator which is responsible for executing operations against the cluster
		Operator: operations.NewOperator().
			WithProgressWriter(ginkgo.GinkgoWriter).
			WithAssertionInterceptor(gomega.InterceptGomegaFailure),

		// Create an operations provider, and point it to the running installation
		Actions: provider.NewActionsProvider().
			WithClusterContext(c.ClusterContext).
			WithGlooGatewayContext(glooGatewayContext),

		// Create an assertions provider, and point it to the running installation
		Assertions: assertions.NewProvider().
			WithProgressWriter(ginkgo.GinkgoWriter).
			WithClusterContext(c.ClusterContext).
			WithGlooGatewayContext(glooGatewayContext),
	}
	c.activeInstallations[installation.String()] = installation

	return installation
}

func (c *TestCluster) UnregisterTestInstallation(installation *TestInstallation) {
	delete(c.activeInstallations, installation.String())
}

// TestInstallation is the structure around a set of tests that validate behavior for an installation
// of Gloo Gateway.
type TestInstallation struct {
	fmt.Stringer

	// TestCluster contains the properties of the TestCluster this TestInstallation is a part of
	TestCluster *TestCluster

	// Metadata contains the properties used to install Gloo Gateway
	Metadata *gloogateway.Context

	// ResourceClients is a set of clients that can manipulate resources owned by Gloo Gateway
	ResourceClients gloogateway.ResourceClients

	// Operator is responsible for executing operations against an installation of Gloo Gateway
	// This is meant to simulate the behaviors that a person could execute
	Operator *operations.Operator

	// Actions is the entity that creates actions that can be executed by the Operator
	Actions *provider.ActionsProvider

	// Assertions is the entity that creates assertions that can be executed by the Operator
	Assertions *assertions.Provider
}

func (i *TestInstallation) String() string {
	return i.Metadata.InstallNamespace
}

func (i *TestInstallation) InstallGlooGateway(ctx context.Context, installAction actions.ClusterAction) error {
	installOperation := &operations.BasicOperation{
		OpName:      "install-gloo-gateway",
		OpAction:    installAction,
		OpAssertion: i.Assertions.InstallationWasSuccessful(),
	}
	err := i.Operator.ExecuteOperations(ctx, installOperation)
	if err != nil {
		return err
	}

	// We can only create the ResourceClients after the CRDs exist in the Cluster
	i.ResourceClients = gloogateway.NewResourceClients(ctx, i.TestCluster.ClusterContext)
	return nil
}

func (i *TestInstallation) UninstallGlooGateway(ctx context.Context, uninstallAction actions.ClusterAction) error {
	installOperation := &operations.BasicOperation{
		OpName:      "uninstall-gloo-gateway",
		OpAction:    uninstallAction,
		OpAssertion: i.Assertions.UninstallationWasSuccessful(),
	}
	return i.Operator.ExecuteOperations(ctx, installOperation)
}

// RunTest will execute a single Test against the installation
// We intentionally do not expose a RunTests method, because then we would
// lose the ability to randomize tests through the testing framework
func (i *TestInstallation) RunTest(ctx context.Context, test Test) {
	gomega.Expect(test.Name).NotTo(gomega.BeEmpty(), "All tests must include a name")

	i.Operator.Logf("TEST: %s", test.Name)
	test.Test(ctx, i)
}

// preFailHandler is the function that is invoked if a test in the given TestInstallation fails
func (i *TestInstallation) preFailHandler() {
	exportReportOp := &operations.BasicOperation{
		OpName:   "glooctl-export-report",
		OpAction: i.Actions.Glooctl().ExportReport(),
		OpAssertion: func(ctx context.Context) {
			// This action is performed on test failure, and is not modifying the cluster
			// As a result, there is no assertion that we perform
		},
	}
	err := i.Operator.ExecuteOperations(context.Background(), exportReportOp)
	if err != nil {
		i.Operator.Logf("Failed to execute preFailHandler operation for TestInstallation (%s): %+v", i, err)
	}
}

// TestExecutor is a function that executes a test, for a given TestInstallation
type TestExecutor func(ctx context.Context, suite *TestInstallation)

// Test represents a single end-to-end behavior that is validated against a running installation of Gloo Gateway.
// Tests are grouped by the feature they validate, and are defined in the test/kubernetes/e2e/features directory
type Test struct {
	// Name is a required value that uniquely identifies a test
	Name string
	// Description is an optional value that is used to provide context to developers about a test's purpose
	Description string
	// Test is the actual function that executes the test
	Test TestExecutor
}
