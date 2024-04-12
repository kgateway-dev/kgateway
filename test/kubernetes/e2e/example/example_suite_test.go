package example_test

import (
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations/provider"
	"github.com/solo-io/gloo/test/testutils"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"

	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
)

func TestExampleSuite(t *testing.T) {
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()

	RunSpecs(t, "Example Suite")
}

var (
	operator           *operations.Operator
	operationsProvider *provider.OperationProvider
	assertionProvider  *assertions.Provider
)

var _ = BeforeSuite(func() {
	clusterContext := cluster.MustKindClusterContext(os.Getenv(testutils.ClusterName))

	// Create an operator which is responsible for execution Operation against the cluster
	operator = operations.NewGinkgoOperator()

	// Set the operations provider to point to the running cluster
	operationsProvider = provider.NewOperationProvider().WithClusterContext(clusterContext)

	// Set the assertion provider to point to the running cluster
	assertionProvider = assertions.NewProvider().WithClusterContext(clusterContext)
})

var _ = AfterSuite(func() {

})
