package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/gloo/test/kubernetes/testutils/actions"

	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	"github.com/solo-io/gloo/test/kubernetes/testutils/runtime"

	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
)

// MustTestHelper returns the SoloTestHelper used for e2e tests
// The SoloTestHelper is a wrapper around `glooctl` and we should eventually phase it out
// in favor of using the exact tool that users rely on
func MustTestHelper(ctx context.Context, installation *TestInstallation) *helper.SoloTestHelper {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	rootDir := filepath.Join(cwd, "../../../../")
	testHelper, err := kube2e.GetTestHelperForRootDir(ctx, rootDir, installation.Metadata.InstallNamespace)
	if err != nil {
		panic(err)
	}

	return testHelper
}

func MustTestCluster() *TestCluster {
	runtimeContext := runtime.NewContext()
	clusterContext := cluster.MustKindContext(runtimeContext.ClusterName)

	return &TestCluster{
		RuntimeContext: runtimeContext,
		ClusterContext: clusterContext,
	}
}

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

func (c *TestCluster) RegisterTestInstallation(t *testing.T, glooGatewayContext *gloogateway.Context) *TestInstallation {
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

		// Create an operations provider, and point it to the running installation
		Actions: actions.NewActionsProvider().
			WithClusterContext(c.ClusterContext).
			WithGlooGatewayContext(glooGatewayContext),

		// Create an assertions provider, and point it to the running installation
		Assertions: assertions.NewProvider(t).
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

	// Actions is the entity that creates actions that can be executed by the Operator
	Actions *actions.Provider

	// Assertions is the entity that creates assertions that can be executed by the Operator
	Assertions *assertions.Provider
}

func (i *TestInstallation) String() string {
	return i.Metadata.InstallNamespace
}

func (i *TestInstallation) InstallGlooGateway(ctx context.Context, installFn func(ctx context.Context) error) {
	if !testutils.ShouldSkipInstall() {
		err := installFn(ctx)
		i.Assertions.Require.NoError(err)
		i.Assertions.EventuallyInstallationSucceeded(ctx)
	}

	// We can only create the ResourceClients after the CRDs exist in the Cluster
	clients, err := gloogateway.NewResourceClients(ctx, i.TestCluster.ClusterContext)
	i.Assertions.Require.NoError(err)
	i.ResourceClients = clients
}

func (i *TestInstallation) UninstallGlooGateway(ctx context.Context, uninstallFn func(ctx context.Context) error) {
	if testutils.ShouldSkipInstall() {
		return
	}
	err := uninstallFn(ctx)
	i.Assertions.Require.NoError(err)
	i.Assertions.EventuallyUninstallationSucceeded(ctx)
}

// PreFailHandler is the function that is invoked if a test in the given TestInstallation fails
func (i *TestInstallation) PreFailHandler(ctx context.Context) {
	logsCmd := i.Actions.Kubectl().Command(ctx, "logs", "-n", i.Metadata.InstallNamespace, "deployments/gloo")
	logsCmd.Run()
}
